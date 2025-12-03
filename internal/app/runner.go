package app

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"cli-navidrome-helper/internal/config"

	"github.com/bmatcuk/doublestar/v4"
)

type runner struct {
	cfg       config.Config
	opts      Options
	log       *log.Logger
	artistDir string
	stats     runStats
}

type runStats struct {
	downloadBytes    int64
	extractedEntries int
	pruned           int
	movedFiles       int
}

func newRunner(cfg config.Config, opts Options) *runner {
	return &runner{
		cfg:  cfg,
		opts: opts,
		log:  log.New(os.Stdout, "nd-import: ", log.LstdFlags),
	}
}

func (r *runner) Execute() error {
	r.log.Printf("Importing Pixeldrain archive for artist %q", r.opts.Artist)

	if err := r.validateInputs(); err != nil {
		return err
	}
	artistDir, err := sanitizeArtist(r.opts.Artist)
	if err != nil {
		return err
	}
	r.artistDir = artistDir

	fileID, downloadURL, err := resolvePixeldrain(r.opts.URL)
	if err != nil {
		return err
	}
	r.log.Printf("Resolved Pixeldrain ID: %s", fileID)

	archivePath, err := r.downloadArchive(downloadURL, fileID)
	if err != nil {
		return err
	}
	defer r.cleanupPath(archivePath)

	extractDir, err := r.extractArchive(archivePath)
	if err != nil {
		return err
	}
	defer r.cleanupPath(extractDir)

	if err := r.pruneExtracted(extractDir); err != nil {
		return err
	}

	dest := r.destinationPath()
	if err := r.moveIntoLibrary(extractDir, dest); err != nil {
		return err
	}

	r.log.Printf("Import complete -> %s (downloaded %s, extracted %d entries, pruned %d, moved %d files)", dest, humanBytes(r.stats.downloadBytes), r.stats.extractedEntries, r.stats.pruned, r.stats.movedFiles)
	return nil
}

func (r *runner) validateInputs() error {
	if strings.TrimSpace(r.opts.Artist) == "" {
		return fmt.Errorf("artist is required")
	}
	if strings.TrimSpace(r.opts.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if r.opts.TmpDir != "" {
		if !filepath.IsAbs(r.opts.TmpDir) {
			return fmt.Errorf("tmp-dir must be absolute: %q", r.opts.TmpDir)
		}
		info, err := os.Stat(r.opts.TmpDir)
		if err != nil {
			return fmt.Errorf("tmp-dir %q not accessible: %w", r.opts.TmpDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("tmp-dir %q is not a directory", r.opts.TmpDir)
		}
	}
	return nil
}

func (r *runner) downloadArchive(downloadURL, fileID string) (string, error) {
	if downloadURL == "" {
		return "", errors.New("download URL is empty")
	}
	tmpDir, err := os.MkdirTemp(r.tmpBase(), "nd-import-download-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("User-Agent", "nd-import/0.1")
	req.Header.Set("Accept", "application/zip")
	if r.cfg.PixeldrainToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.cfg.PixeldrainToken)
	}

	client := &http.Client{Timeout: 0}
	r.log.Printf("Downloading Pixeldrain file %s ...", fileID)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("download failed: status %d %s: %s", resp.StatusCode, resp.Status, strings.TrimSpace(string(body)))
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "zip") && !strings.Contains(contentType, "octet-stream") {
		return "", fmt.Errorf("unexpected content-type %q (expected zip) from Pixeldrain", contentType)
	}

	outFile, err := os.CreateTemp(tmpDir, "pixeldrain-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("write download: %w", err)
	}
	if written == 0 {
		return "", fmt.Errorf("downloaded file is empty")
	}
	r.stats.downloadBytes = written
	r.log.Printf("Downloaded %s to %s", humanBytes(written), outFile.Name())

	return outFile.Name(), nil
}

func (r *runner) extractArchive(archivePath string) (string, error) {
	if archivePath == "" {
		return "", fmt.Errorf("archive path is empty")
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return "", fmt.Errorf("archive %s is empty", archivePath)
	}

	destDir, err := os.MkdirTemp(r.tmpBase(), "nd-import-extract-")
	if err != nil {
		return "", fmt.Errorf("create extract dir: %w", err)
	}

	for _, f := range reader.File {
		rel := filepath.Clean(f.Name)
		if rel == "." {
			continue
		}
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("zip entry %q uses unsupported path", f.Name)
		}

		targetPath := filepath.Join(destDir, rel)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return "", fmt.Errorf("create directory %q: %w", targetPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return "", fmt.Errorf("create parent for %q: %w", targetPath, err)
		}

		src, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}

		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}
		dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			src.Close()
			return "", fmt.Errorf("create file %q: %w", targetPath, err)
		}

		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			src.Close()
			return "", fmt.Errorf("copy entry %q: %w", f.Name, err)
		}

		dst.Close()
		src.Close()
	}

	r.stats.extractedEntries = len(reader.File)
	r.log.Printf("Extracted %d entries into %s", len(reader.File), destDir)
	return destDir, nil
}

func (r *runner) pruneExtracted(extractDir string) error {
	if extractDir == "" {
		return fmt.Errorf("extract directory is empty")
	}
	if len(r.cfg.UnneededPatterns) == 0 {
		return nil
	}

	var toRemove []string
	var fileCount int

	err := filepath.WalkDir(extractDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == extractDir {
			return nil
		}

		rel, err := filepath.Rel(extractDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		if !d.IsDir() {
			fileCount++
		}

		for _, pattern := range r.cfg.UnneededPatterns {
			ok, err := doublestar.Match(pattern, relSlash)
			if err != nil {
				return fmt.Errorf("invalid pattern %q: %w", pattern, err)
			}
			if ok {
				toRemove = append(toRemove, path)
				// If a directory matches, skip evaluating deeper because WalkDir will still enter; no need to short-circuit.
				break
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Protect against deleting everything.
	unique := make(map[string]struct{})
	for _, p := range toRemove {
		unique[p] = struct{}{}
	}

	remainingFiles := 0
	err = filepath.WalkDir(extractDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == extractDir {
			return nil
		}
		if d.IsDir() {
			if _, ok := unique[path]; ok {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := unique[path]; !ok {
			remainingFiles++
		}
		return nil
	})
	if err != nil {
		return err
	}

	if fileCount > 0 && remainingFiles == 0 {
		return fmt.Errorf("prune patterns would remove all %d files; aborting", fileCount)
	}

	if len(unique) == 0 {
		return nil
	}

	var removed []string
	for path := range unique {
		removed = append(removed, path)
	}
	sort.Strings(removed)

	for _, path := range removed {
		if r.opts.DryRun {
			r.log.Printf("dry-run: would remove %s", path)
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %q: %w", path, err)
		}
	}

	r.stats.pruned = len(removed)
	r.log.Printf("Pruned %d item(s) matching UNNEEDED_FILES", len(removed))
	return nil
}

func (r *runner) moveIntoLibrary(extractDir, dest string) error {
	if extractDir == "" {
		return fmt.Errorf("extract directory is empty")
	}
	if dest == "" {
		return fmt.Errorf("destination path is empty")
	}

	if err := r.ensureNoCollisions(extractDir, dest); err != nil {
		return err
	}

	if r.opts.DryRun {
		r.log.Printf("dry-run: would merge extracted files into %s", dest)
		return nil
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create destination %q: %w", dest, err)
	}

	err := filepath.WalkDir(extractDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == extractDir {
			return nil
		}

		rel, err := filepath.Rel(extractDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		if err := copyFile(path, target, info.Mode()); err != nil {
			return err
		}
		r.stats.movedFiles++
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *runner) ensureNoCollisions(srcRoot, destRoot string) error {
	return filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcRoot {
			return nil
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destRoot, rel)

		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		if d.IsDir() && !info.IsDir() {
			return fmt.Errorf("destination conflict: %s exists as a file", target)
		}
		if !d.IsDir() && info.IsDir() {
			return fmt.Errorf("destination conflict: %s exists as a directory", target)
		}
		if !d.IsDir() && !info.IsDir() {
			return fmt.Errorf("destination conflict: %s already exists", target)
		}
		return nil
	})
}

func (r *runner) cleanupPath(path string) {
	if path == "" || r.opts.KeepTemp {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		r.log.Printf("warning: failed to clean up %s: %v", path, err)
		return
	}
}

func (r *runner) destinationPath() string {
	return filepath.Join(r.cfg.NavidromeMusicPath, r.artistDir)
}

func (r *runner) tmpBase() string {
	if r.opts.TmpDir != "" {
		return r.opts.TmpDir
	}
	return ""
}

func sanitizeArtist(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("artist is required")
	}
	cleaned := filepath.Clean(trimmed)
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("artist %q contains invalid path characters", name)
	}
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("artist %q is invalid", name)
	}
	return cleaned, nil
}

var pixeldrainIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,}$`)

func resolvePixeldrain(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("url is required")
	}

	if pixeldrainIDPattern.MatchString(raw) && !strings.Contains(raw, "/") && !strings.Contains(raw, ".") {
		id := raw
		return id, fmt.Sprintf("https://pixeldrain.com/api/file/%s?download", url.PathEscape(id)), nil
	}

	u := raw
	if !strings.Contains(raw, "://") {
		u = "https://" + raw
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}

	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", "", fmt.Errorf("invalid URL %q: missing host", raw)
	}
	if !strings.Contains(host, "pixeldrain.com") && !strings.Contains(host, "doubledouble.top") {
		return "", "", fmt.Errorf("unsupported host %q; expected Pixeldrain", host)
	}

	segments := strings.FieldsFunc(strings.Trim(parsed.Path, "/"), func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return "", "", fmt.Errorf("missing Pixeldrain id in URL %q", raw)
	}

	id := segments[len(segments)-1]
	id = strings.TrimSpace(id)
	if !pixeldrainIDPattern.MatchString(id) {
		return "", "", fmt.Errorf("invalid Pixeldrain id %q", id)
	}

	return id, fmt.Sprintf("https://pixeldrain.com/api/file/%s?download", url.PathEscape(id)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(1024), 0
	for m := n / 1024; m >= 1024 && exp < 4; m /= 1024 {
		div *= 1024
		exp++
	}
	value := float64(n) / float64(div)
	unit := []string{"KB", "MB", "GB", "TB", "PB"}[exp]
	return fmt.Sprintf("%.1f %s", value, unit)
}
