package docker

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

// ContainerInfo holds parsed docker ps JSON output.
type ContainerInfo struct {
	ID      string `json:"ID"`
	Names   string `json:"Names"`
	Image   string `json:"Image"`
	Status  string `json:"Status"`
	State   string `json:"State"`
	Ports   string `json:"Ports"`
	Created string `json:"CreatedAt"`
}

// StatusPoller provides cached Docker container status.
type StatusPoller struct {
	mu     sync.RWMutex
	cache  []ContainerInfo
	expiry time.Time
	ttl    time.Duration
}

// NewStatusPoller creates a poller with a 5-second cache TTL.
func NewStatusPoller() *StatusPoller {
	return &StatusPoller{
		ttl: 5 * time.Second,
	}
}

// ListContainers returns all containers on keel-net, using cache if fresh.
func (p *StatusPoller) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	p.mu.RLock()
	if time.Now().Before(p.expiry) {
		cached := p.cache
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Now().Before(p.expiry) {
		return p.cache, nil
	}

	containers, err := fetchContainers(ctx)
	if err != nil {
		return nil, err
	}

	p.cache = containers
	p.expiry = time.Now().Add(p.ttl)
	return containers, nil
}

func fetchContainers(ctx context.Context) ([]ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "network=keel-net",
		"--format", "json",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker ps: %w: %s", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	}

	var containers []ContainerInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ci ContainerInfo
		if err := json.Unmarshal([]byte(line), &ci); err != nil {
			continue
		}
		containers = append(containers, ci)
	}

	return containers, nil
}

// MatchServiceToContainer finds a container matching a service name or hostname.
// Priority: explicit hostname > keel-{name} > {name}.
func MatchServiceToContainer(serviceName, serviceHostname string, containers []ContainerInfo) *ContainerInfo {
	for i, c := range containers {
		name := strings.TrimPrefix(c.Names, "/")
		if (serviceHostname != "" && name == serviceHostname) ||
			name == "keel-"+serviceName ||
			name == serviceName {
			return &containers[i]
		}
	}
	return nil
}

// ContainerToStatus converts a Docker container state to a model.ContainerStatus.
func ContainerToStatus(ci *ContainerInfo) model.ContainerStatus {
	if ci == nil {
		return model.StatusMissing
	}
	switch strings.ToLower(ci.State) {
	case "running":
		if strings.Contains(strings.ToLower(ci.Status), "unhealthy") {
			return model.StatusUnhealthy
		}
		return model.StatusRunning
	case "restarting":
		return model.StatusRestarting
	default:
		return model.StatusStopped
	}
}

// Invalidate forces the next call to fetch fresh data.
func (p *StatusPoller) Invalidate() {
	p.mu.Lock()
	p.expiry = time.Time{}
	p.mu.Unlock()
}
