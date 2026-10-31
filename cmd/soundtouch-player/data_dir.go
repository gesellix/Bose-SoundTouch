package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func playerDataDirectory(configured string) (string, error) {
	if configured != "" {
		return filepath.Abs(configured)
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve player data directory; set --data-dir: %w", err)
	}

	return filepath.Join(base, "aftertouch-player"), nil
}
