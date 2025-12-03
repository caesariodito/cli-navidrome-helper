package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// Config represents environment-derived settings.
type Config struct {
	NavidromeMusicPath string
	UnneededPatterns   []string
	PixeldrainToken    string
}

// Load reads .env (if present) and validates required settings.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		NavidromeMusicPath: strings.TrimSpace(os.Getenv("NAVIDROME_MUSIC_PATH")),
		PixeldrainToken:    strings.TrimSpace(os.Getenv("PIXELDRAIN_TOKEN")),
	}

	rawPatterns := strings.TrimSpace(os.Getenv("UNNEEDED_FILES"))
	if rawPatterns != "" {
		for _, part := range strings.Split(rawPatterns, ",") {
			pattern := strings.TrimSpace(part)
			if pattern != "" {
				cfg.UnneededPatterns = append(cfg.UnneededPatterns, pattern)
			}
		}
	}

	if cfg.NavidromeMusicPath == "" {
		return cfg, errors.New("NAVIDROME_MUSIC_PATH is required (absolute path to Navidrome music root)")
	}
	if !filepath.IsAbs(cfg.NavidromeMusicPath) {
		return cfg, fmt.Errorf("NAVIDROME_MUSIC_PATH must be an absolute path: %q", cfg.NavidromeMusicPath)
	}
	info, err := os.Stat(cfg.NavidromeMusicPath)
	if err != nil {
		return cfg, fmt.Errorf("NAVIDROME_MUSIC_PATH %q is not accessible: %w", cfg.NavidromeMusicPath, err)
	}
	if !info.IsDir() {
		return cfg, fmt.Errorf("NAVIDROME_MUSIC_PATH %q is not a directory", cfg.NavidromeMusicPath)
	}

	return cfg, nil
}
