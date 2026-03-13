package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/model"
)

type dockerStatsJSON struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
}

// ReadDockerStats returns per-container resource usage via docker stats.
func ReadDockerStats(ctx context.Context) ([]model.ContainerStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker stats: %w: %s", err, stderr.String())
	}

	var results []model.ContainerStats
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var ds dockerStatsJSON
		if err := json.Unmarshal([]byte(line), &ds); err != nil {
			continue
		}
		results = append(results, model.ContainerStats{
			Name:     strings.TrimPrefix(ds.Name, "/"),
			CPUPerc:  ds.CPUPerc,
			MemUsage: ds.MemUsage,
			MemPerc:  ds.MemPerc,
			NetIO:    ds.NetIO,
			BlockIO:  ds.BlockIO,
		})
	}

	return results, nil
}
