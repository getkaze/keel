package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/updater"
)

// VersionHandler responds with the current and latest version info.
type VersionHandler struct {
	Version string
}

func (h *VersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result, err := updater.Check(h.Version)
	if err != nil {
		// If check fails (no internet), return current version with no update
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"current":   h.Version,
			"latest":    h.Version,
			"available": false,
		})
		return
	}

	resp := map[string]any{
		"current":    result.Current,
		"latest":     result.Latest,
		"update_url": result.UpdateURL,
		"available":  result.Available,
	}

	// Fetch release notes for the latest version
	if result.Available {
		if notes, err := updater.FetchReleaseNotes(result.Latest); err == nil {
			resp["changelog"] = notes
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// UpdateHandler performs a self-update via SSE, then restarts the process.
type UpdateHandler struct {
	Version string
}

func (h *UpdateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(msg string) {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}

	// 1. Check latest version
	send("Checking for updates...")
	result, err := updater.Check(h.Version)
	if err != nil {
		fmt.Fprintf(w, "event: app-error\ndata: Failed to check version: %s\n\n", err)
		flusher.Flush()
		return
	}

	if !result.Available {
		fmt.Fprintf(w, "event: app-error\ndata: Already on latest version %s\n\n", h.Version)
		flusher.Flush()
		return
	}

	send(fmt.Sprintf("New version available: %s → %s", result.Current, result.Latest))

	// 2. Download
	send(fmt.Sprintf("Downloading %s...", result.Latest))
	tmpPath, err := updater.Download(result.Latest)
	if err != nil {
		msg := fmt.Sprintf("Download failed: %s", err)
		if strings.Contains(err.Error(), "permission denied") {
			msg = "Permission denied — restart keel with sudo to update"
		}
		fmt.Fprintf(w, "event: app-error\ndata: %s\n\n", msg)
		flusher.Flush()
		return
	}

	// 3. Replace binary
	send("Replacing binary...")
	if err := updater.Replace(tmpPath); err != nil {
		msg := fmt.Sprintf("Replace failed: %s", err)
		if strings.Contains(err.Error(), "permission denied") {
			msg = "Permission denied — restart keel with sudo to update"
		}
		fmt.Fprintf(w, "event: app-error\ndata: %s\n\n", msg)
		flusher.Flush()
		return
	}

	send(fmt.Sprintf("Updated to %s successfully!", result.Latest))
	fmt.Fprintf(w, "event: done\ndata: Updated to %s — restarting...\n\n", result.Latest)
	flusher.Flush()

	// 4. Restart: re-exec the new binary after a short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		exe, err := os.Executable()
		if err != nil {
			log.Printf("update: cannot find executable for restart: %v", err)
			os.Exit(0)
		}

		if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
			cmd := exec.Command(exe, os.Args[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if err := cmd.Start(); err != nil {
				log.Printf("update: restart failed: %v", err)
			}
		}

		os.Exit(0)
	}()
}
