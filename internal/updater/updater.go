package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const ReleasesBase = "https://releases.getkaze.dev"

// CheckResult holds the result of a version check.
type CheckResult struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	UpdateURL string `json:"update_url"`
	Available bool   `json:"available"`
}

// Check fetches the latest version and compares with current.
func Check(current string) (*CheckResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(ReleasesBase + "/version")
	if err != nil {
		return nil, fmt.Errorf("fetch version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch version: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}

	latest := strings.TrimSpace(string(body))

	return &CheckResult{
		Current:   current,
		Latest:    latest,
		UpdateURL: downloadURL(latest),
		Available: current != "dev" && IsNewer(latest, current),
	}, nil
}

// IsNewer reports whether version a is semantically newer than version b.
// Versions are expected as "major.minor.patch" (leading "v" is stripped).
// Falls back to string comparison if parsing fails.
func IsNewer(a, b string) bool {
	aParts, aOk := parseSemver(a)
	bParts, bOk := parseSemver(b)
	if !aOk || !bOk {
		return a != b
	}
	for i := 0; i < 3; i++ {
		if aParts[i] != bParts[i] {
			return aParts[i] > bParts[i]
		}
	}
	return false
}

// parseSemver splits "major.minor.patch" into [3]int.
func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		result[i] = n
	}
	return result, true
}

// Download fetches the latest binary to a temp file and returns its path.
// The temp file is created in the same directory as the running binary
// to avoid cross-device rename errors when /tmp is on a different filesystem.
func Download(version string) (string, error) {
	url := downloadURL(version)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: status %d", resp.StatusCode)
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	targetDir := filepath.Dir(exe)

	tmp, err := os.CreateTemp(targetDir, "keel-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("chmod: %w", err)
	}

	return tmp.Name(), nil
}

// Replace atomically replaces the current binary with the downloaded one.
func Replace(tmpPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}

func downloadURL(version string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s/%s/keel-%s-%s", ReleasesBase, version, osName, arch)
}
