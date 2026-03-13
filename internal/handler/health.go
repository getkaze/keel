package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/model"
)

// HealthResult holds the health check result for a single service.
type HealthResult struct {
	Name    string                `json:"name"`
	Status  model.ContainerStatus `json:"container_status"`
	Healthy *bool                 `json:"healthy,omitempty"`
	Output  string                `json:"output,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// HealthHandler handles GET /api/health.
type HealthHandler struct {
	Services *config.ServiceStore
	Docker   *docker.StatusPoller
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	services, err := h.Services.List()
	if err != nil {
		log.Printf("health: failed to list services: %v (returning empty)", err)
		services = nil
	}

	containers, _ := h.Docker.ListContainers(r.Context())

	var results []HealthResult
	for _, svc := range services {
		ci := docker.MatchServiceToContainer(svc.Name, svc.Hostname, containers)
		status := docker.ContainerToStatus(ci)

		result := HealthResult{
			Name:   svc.Name,
			Status: status,
		}

		if svc.HealthCheck != nil && status == model.StatusRunning {
			healthy, output, checkErr := runHealthCheck(r.Context(), svc, ci)
			result.Healthy = &healthy
			result.Output = output
			if checkErr != nil {
				result.Error = checkErr.Error()
			}
		}

		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func runHealthCheck(ctx context.Context, svc model.Service, ci *docker.ContainerInfo) (bool, string, error) {
	if svc.HealthCheck == nil {
		return false, "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch svc.HealthCheck.Type {
	case "command":
		return runCommandCheck(ctx, ci, svc.HealthCheck.Command)
	case "http":
		return runHTTPCheck(ctx, svc.HealthCheck.URL)
	default:
		return false, "", fmt.Errorf("unknown health check type: %s", svc.HealthCheck.Type)
	}
}

func runCommandCheck(ctx context.Context, ci *docker.ContainerInfo, command string) (bool, string, error) {
	if ci == nil {
		return false, "", fmt.Errorf("container not found")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", ci.Names, "sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := stdout.String()
	if output == "" {
		output = stderr.String()
	}
	if err != nil {
		return false, output, nil
	}
	return true, output, nil
}

func runHTTPCheck(ctx context.Context, url string) (bool, string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("health http check %s: %v", url, err)
		return false, "", nil
	}
	defer resp.Body.Close()
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	return healthy, fmt.Sprintf("HTTP %d", resp.StatusCode), nil
}
