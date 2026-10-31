package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPlayerDataDirectoryIsExplicitOrUserScoped(t *testing.T) {
	base := t.TempDir()
	// XDG_CONFIG_HOME governs the Linux default; macOS/Windows deliberately
	// use os.UserConfigDir's native platform convention instead.
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CONFIG_HOME", base)
		got, err := playerDataDirectory("")
		if err != nil || got != filepath.Join(base, "aftertouch-player") {
			t.Fatalf("default=%q %v", got, err)
		}
	}
	explicit := filepath.Join(base, "separate-instance")
	got, err := playerDataDirectory(explicit)
	if err != nil || got != explicit {
		t.Fatalf("explicit=%q %v", got, err)
	}
}
