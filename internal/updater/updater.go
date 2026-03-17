package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const ReleasesBase = "https://github.com/getkaze/keel/releases"

// CheckResult holds the result of a version check.
type CheckResult struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	UpdateURL string `json:"update_url"`
	Available bool   `json:"available"`
}

// Check fetches the latest version and compares with current.
func Check(current string) (*CheckResult, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(ReleasesBase + "/latest")
	if err != nil {
		return nil, fmt.Errorf("fetch version: %w", err)
	}
	resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return nil, fmt.Errorf("fetch version: no redirect from /latest")
	}

	parts := strings.Split(loc, "/")
	latest := parts[len(parts)-1]

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

// Replace replaces the current binary with the downloaded one.
func Replace(tmpPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	if err := os.Remove(exe); err != nil {
		return fmt.Errorf("remove old binary: %w", err)
	}

	src, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("open update: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(exe, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create binary: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("write binary: %w", err)
	}

	if err := dst.Close(); err != nil {
		return fmt.Errorf("close binary: %w", err)
	}

	os.Remove(tmpPath)
	return nil
}

// FetchReleaseNotes returns the release body (markdown) for a given version tag.
func FetchReleaseNotes(version string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/getkaze/keel/releases/tags/%s", version)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch release notes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch release notes: status %d", resp.StatusCode)
	}

	var release struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse release notes: %w", err)
	}

	return stripBoilerplate(release.Body), nil
}

// stripBoilerplate removes Installation and Manual download sections,
// keeping only the "what's new" content.
func stripBoilerplate(body string) string {
	lines := strings.Split(body, "\n")
	var result []string

	for i := 0; i < len(lines); i++ {
		lower := strings.ToLower(strings.TrimSpace(lines[i]))

		// Stop when we hit boilerplate sections — everything after is skipped
		if strings.HasPrefix(lower, "## installation") ||
			strings.HasPrefix(lower, "### installation") ||
			strings.HasPrefix(lower, "## manual download") ||
			strings.HasPrefix(lower, "### manual download") {
			break
		}

		// Skip a horizontal rule if it immediately precedes a boilerplate heading
		if lower == "---" || lower == "***" || lower == "___" {
			next := ""
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" {
					next = strings.ToLower(strings.TrimSpace(lines[j]))
					break
				}
			}
			if strings.HasPrefix(next, "## installation") ||
				strings.HasPrefix(next, "### installation") ||
				strings.HasPrefix(next, "## manual download") ||
				strings.HasPrefix(next, "### manual download") {
				break
			}
		}

		result = append(result, lines[i])
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

func downloadURL(version string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s/download/%s/keel-%s-%s", ReleasesBase, version, osName, arch)
}
