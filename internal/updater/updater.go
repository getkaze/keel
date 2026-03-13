package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
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
		Available: latest != current && current != "dev",
	}, nil
}

// Download fetches the latest binary to a temp file and returns its path.
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

	tmp, err := os.CreateTemp("", "keel-update-*")
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
