package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultDataDir returns the default keel data directory for the current OS.
//   - Linux:  /var/lib/keel
//   - macOS:  ~/.keel
func DefaultDataDir() string {
	if d := os.Getenv("KEEL_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "/var/lib/keel"
		}
		return filepath.Join(home, ".keel")
	}
	return "/var/lib/keel"
}
