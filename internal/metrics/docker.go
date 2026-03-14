package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
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

// StatsPoller provides cached Docker stats with a configurable TTL.
type StatsPoller struct {
	mu     sync.RWMutex
	cache  []model.ContainerStats
	expiry time.Time
	ttl    time.Duration
}

// NewStatsPoller creates a poller with a 5-second cache TTL.
func NewStatsPoller() *StatsPoller {
	return &StatsPoller{
		ttl: 5 * time.Second,
	}
}

// ReadStats returns cached docker stats, refreshing if expired.
func (p *StatsPoller) ReadStats(ctx context.Context) ([]model.ContainerStats, error) {
	p.mu.RLock()
	if time.Now().Before(p.expiry) {
		cached := p.cache
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if time.Now().Before(p.expiry) {
		return p.cache, nil
	}

	stats, err := fetchDockerStats(ctx)
	if err != nil {
		return nil, err
	}

	p.cache = stats
	p.expiry = time.Now().Add(p.ttl)
	return stats, nil
}

// Invalidate forces the next call to fetch fresh data.
func (p *StatsPoller) Invalidate() {
	p.mu.Lock()
	p.expiry = time.Time{}
	p.mu.Unlock()
}

func fetchDockerStats(ctx context.Context) ([]model.ContainerStats, error) {
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
