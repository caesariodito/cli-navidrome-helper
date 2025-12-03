package app

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"cli-navidrome-helper/internal/config"
)

func TestResolvePixeldrain(t *testing.T) {
	tests := []struct {
		input   string
		wantID  string
		wantURL string
		ok      bool
	}{
		{"abc123", "abc123", "https://pixeldrain.com/api/file/abc123?download", true},
		{"https://pixeldrain.com/u/xyz", "xyz", "https://pixeldrain.com/api/file/xyz?download", true},
		{"doubledouble.top/xyz", "xyz", "https://pixeldrain.com/api/file/xyz?download", true},
		{"", "", "", false},
		{"https://example.com/file.zip", "", "", false},
	}

	for _, tt := range tests {
		id, url, err := resolvePixeldrain(tt.input)
		if tt.ok && err != nil {
			t.Fatalf("resolvePixeldrain(%q) returned error: %v", tt.input, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("resolvePixeldrain(%q) expected error, got nil", tt.input)
		}
		if !tt.ok {
			continue
		}
		if id != tt.wantID || url != tt.wantURL {
			t.Fatalf("resolvePixeldrain(%q) = (%s, %s), want (%s, %s)", tt.input, id, url, tt.wantID, tt.wantURL)
		}
	}
}

func TestSanitizeArtist(t *testing.T) {
	valid := map[string]string{
		"Artist Name":           "Artist Name",
		"Artist/Name":           "Artist_Name",
		"\\Artist\\Name\\":      "_Artist_Name_",
		"  spaced artist  ":     "spaced artist",
		"Artist-123_underscore": "Artist-123_underscore",
	}
	for input, want := range valid {
		got, err := sanitizeArtist(input)
		if err != nil {
			t.Fatalf("sanitizeArtist(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("sanitizeArtist(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{"", "/", "../bad", "/abs/path", "..", "."}
	for _, input := range invalid {
		if _, err := sanitizeArtist(input); err == nil {
			t.Fatalf("sanitizeArtist(%q) expected error, got nil", input)
		}
	}
}

func TestPruneExtracted(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep.mp3")
	removeTxt := filepath.Join(root, "notes.txt")
	removeDir := filepath.Join(root, "Samples", "kick.wav")

	if err := os.WriteFile(keep, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(removeTxt, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(removeDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(removeDir, []byte("wav"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &runner{
		cfg: config.Config{
			UnneededPatterns: []string{"*.txt", "Samples/**"},
		},
		opts: Options{},
		log:  log.New(io.Discard, "", 0),
	}

	if err := r.pruneExtracted(root); err != nil {
		t.Fatalf("pruneExtracted returned error: %v", err)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("keep file missing after prune: %v", err)
	}
	if _, err := os.Stat(removeTxt); !os.IsNotExist(err) {
		t.Fatalf("notes.txt should be removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Dir(removeDir)); err == nil {
		t.Fatalf("Samples directory should be removed")
	}
}

func TestPruneExtractedProtectsAllFiles(t *testing.T) {
	root := t.TempDir()
	only := filepath.Join(root, "only.txt")
	if err := os.WriteFile(only, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &runner{
		cfg: config.Config{
			UnneededPatterns: []string{"**"},
		},
		opts: Options{},
		log:  log.New(io.Discard, "", 0),
	}

	if err := r.pruneExtracted(root); err == nil {
		t.Fatalf("expected pruneExtracted to abort when removing all files")
	}
}

func TestMoveIntoLibraryCollision(t *testing.T) {
	src := t.TempDir()
	dest := filepath.Join(t.TempDir(), "library")

	if err := os.WriteFile(filepath.Join(src, "song.mp3"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "song.mp3"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &runner{
		cfg:  config.Config{},
		opts: Options{},
		log:  log.New(io.Discard, "", 0),
	}

	if err := r.moveIntoLibrary(src, dest); err == nil {
		t.Fatalf("expected collision error, got nil")
	}
}

func TestMoveIntoLibrarySuccess(t *testing.T) {
	src := t.TempDir()
	dest := filepath.Join(t.TempDir(), "library")

	audio := filepath.Join(src, "Album", "song.mp3")
	if err := os.MkdirAll(filepath.Dir(audio), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audio, []byte("music"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &runner{
		cfg:  config.Config{},
		opts: Options{},
		log:  log.New(io.Discard, "", 0),
	}

	if err := r.moveIntoLibrary(src, dest); err != nil {
		t.Fatalf("moveIntoLibrary returned error: %v", err)
	}

	target := filepath.Join(dest, "Album", "song.mp3")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected moved file, got err: %v", err)
	}
	if string(data) != "music" {
		t.Fatalf("moved file contents mismatch: %q", string(data))
	}
	if r.stats.movedFiles != 1 {
		t.Fatalf("expected movedFiles=1, got %d", r.stats.movedFiles)
	}
}
