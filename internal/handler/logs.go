package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/model"
)

const (
	defaultLogLines  = 100
	maxLogLines      = 10000
	logStreamTimeout = 30 * time.Minute
)

// LogHandler handles GET /api/logs/{name} — streams container logs via SSE.
type LogHandler struct {
	Services *config.ServiceStore
	Docker   *docker.StatusPoller
}

func (h *LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := h.Services.FindService(name)
	if err != nil {
		log.Printf("logs: lookup service %q: %v", name, err)
		http.Error(w, "failed to look up service", http.StatusInternalServerError)
		return
	}
	if svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	lines := parseLines(r)
	source := r.URL.Query().Get("source")
	filePath := r.URL.Query().Get("file")

	// SSE setup
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx, cancel := context.WithTimeout(r.Context(), logStreamTimeout)
	defer cancel()

	// If the source has a host_path, read directly from the host — no container needed.
	if source != "" {
		logSource := findLogSource(svc.Logs, source)
		if logSource == nil {
			fmt.Fprintf(w, "event: error\ndata: unknown log source: %s\n\n", source)
			flusher.Flush()
			return
		}
		if logSource.HostPath != "" {
			target := logSource.HostPath
			// Allow selecting a specific file within a host directory.
			if filePath != "" && isWithinDir(filePath, logSource.HostPath) {
				target = filePath
			}
			cmd := exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(lines), "-F", target)
			streamCmd(ctx, w, flusher, cmd)
			return
		}
	}

	// All other paths require the container to be running.
	containers, _ := h.Docker.ListContainers(r.Context())
	ci := docker.MatchServiceToContainer(name, svc.Hostname, containers)
	if ci == nil {
		http.Error(w, "container not found (service may not be running)", http.StatusNotFound)
		return
	}
	containerName := ci.Names

	// Build docker command based on source type.
	var cmdArgs []string
	if filePath != "" {
		cmdArgs = []string{"exec", containerName, "tail", "-n", strconv.Itoa(lines), "-f", filePath}
	} else if source != "" {
		logSource := findLogSource(svc.Logs, source)
		if logSource == nil {
			fmt.Fprintf(w, "event: error\ndata: unknown log source: %s\n\n", source)
			flusher.Flush()
			return
		}
		if logSource.Type != "file" || logSource.Path == "" {
			fmt.Fprintf(w, "event: error\ndata: log source has no file path\n\n")
			flusher.Flush()
			return
		}
		cmdArgs = []string{"exec", containerName, "tail", "-n", strconv.Itoa(lines), "-f", logSource.Path}
	} else {
		cmdArgs = []string{"logs", "--follow", "--tail", strconv.Itoa(lines), containerName}
	}

	streamCmd(ctx, w, flusher, exec.CommandContext(ctx, "docker", cmdArgs...))
}

func streamCmd(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, cmd *exec.Cmd) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: failed to create pipe: %s\n\n", err)
		flusher.Flush()
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		stdout.Close()
		fmt.Fprintf(w, "event: error\ndata: failed to start: %s\n\n", err)
		flusher.Flush()
		return
	}
	// Ensure the process is reaped even if we return early (e.g. ctx timeout).
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			fmt.Fprintf(w, "event: error\ndata: stream timeout\n\n")
			flusher.Flush()
			return
		default:
			fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
			flusher.Flush()
		}
	}
}

// LogSourceFile represents a resolved log file.
type LogSourceFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// LogSourceInfo represents a log source with resolved files.
type LogSourceInfo struct {
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Path      string          `json:"path,omitempty"`
	HostPath  string          `json:"host_path,omitempty"`
	Available bool            `json:"available"` // true when readable regardless of container state
	Files     []LogSourceFile `json:"files,omitempty"`
}

// ServeLogSources handles GET /api/logs/{name}/sources — lists available log sources
// and resolves directory paths to actual files.
func (h *LogHandler) ServeLogSources(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := h.Services.FindService(name)
	if err != nil {
		http.Error(w, "failed to look up service", http.StatusInternalServerError)
		return
	}
	if svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Always include docker logs as first source
	sources := []LogSourceInfo{
		{Name: "container", Type: "docker"},
	}

	// Resolve file-based sources
	containers, _ := h.Docker.ListContainers(r.Context())
	ci := docker.MatchServiceToContainer(name, svc.Hostname, containers)

	for _, ls := range svc.Logs {
		if ls.Type != "file" {
			continue
		}

		info := LogSourceInfo{
			Name:      ls.Name,
			Type:      ls.Type,
			Path:      ls.Path,
			HostPath:  ls.HostPath,
			Available: ls.HostPath != "",
		}

		// List files from host directory (always, no container needed).
		if ls.HostPath != "" {
			if hf := listHostLogFiles(ls.HostPath); len(hf) > 0 {
				info.Files = hf
			}
		}

		// If container is running, also try to resolve via docker exec.
		if ci != nil {
			info.Available = true
			if ls.Path != "" && len(info.Files) == 0 {
				if cf := listLogFiles(r.Context(), ci.Names, ls.Path); len(cf) > 0 {
					info.Files = cf
				}
			}
		}

		sources = append(sources, info)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}

// listLogFiles lists log files inside a path (file or directory) in a container.
func listLogFiles(ctx context.Context, containerName, logPath string) []LogSourceFile {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Use find to list files — works for both files and directories
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"find", logPath, "-type", "f", "-name", "*.log", "-o", "-name", "*.txt", "-o", "-name", "*.out")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Path might be a single file, not a directory
		return nil
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil
	}

	var files []LogSourceFile
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, LogSourceFile{
			Name: path.Base(line),
			Path: line,
		})
	}
	return files
}

// listHostLogFiles lists log files in a host directory path recursively.
func listHostLogFiles(hostPath string) []LogSourceFile {
	info, err := os.Stat(hostPath)
	if err != nil || !info.IsDir() {
		return nil
	}
	var files []LogSourceFile
	filepath.Walk(hostPath, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, hostPath)
		rel = strings.TrimPrefix(rel, "/")
		files = append(files, LogSourceFile{Name: rel, Path: p})
		return nil
	})
	return files
}

// isWithinDir returns true if p is inside (or equal to) dir.
func isWithinDir(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && !strings.HasPrefix(rel, "..")
}

func parseLines(r *http.Request) int {
	lines := defaultLogLines
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			lines = n
		}
	}
	if lines > maxLogLines {
		lines = maxLogLines
	}
	return lines
}

func findLogSource(sources []model.LogSource, name string) *model.LogSource {
	for i, s := range sources {
		if s.Name == name {
			return &sources[i]
		}
	}
	return nil
}
