package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
)

const (
	healthPollInterval = 2 * time.Second
	healthTimeout      = 60 * time.Second
)

// CmdBuilder builds exec.Cmd for docker commands, routing via SSH for remote targets.
type CmdBuilder interface {
	DockerCmd(ctx context.Context, args ...string) *exec.Cmd
}

// localCmdBuilder runs docker commands directly on the local machine.
type localCmdBuilder struct{}

func (localCmdBuilder) DockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}

// SeederExecutor runs seeder commands against infrastructure containers.
type SeederExecutor struct {
	Services   *config.ServiceStore
	Seeders    *config.SeederStore
	KeelDir    string
	Cmd        CmdBuilder
	statusMu   sync.RWMutex
	lastStatus map[string]string
}

// NewSeederExecutor creates a SeederExecutor.
func NewSeederExecutor(keelDir string, services *config.ServiceStore, seeders *config.SeederStore) *SeederExecutor {
	return &SeederExecutor{
		Services:   services,
		Seeders:    seeders,
		KeelDir:    keelDir,
		Cmd:        localCmdBuilder{},
		lastStatus: make(map[string]string),
	}
}

// NewSeederExecutorWithCmd creates a SeederExecutor with a custom command builder.
func NewSeederExecutorWithCmd(keelDir string, services *config.ServiceStore, seeders *config.SeederStore, cmd CmdBuilder) *SeederExecutor {
	return &SeederExecutor{
		Services:   services,
		Seeders:    seeders,
		KeelDir:    keelDir,
		Cmd:        cmd,
		lastStatus: make(map[string]string),
	}
}

// GetLastStatus returns the last known run status ("success" or "error") for a seeder.
func (se *SeederExecutor) GetLastStatus(name string) string {
	se.statusMu.RLock()
	defer se.statusMu.RUnlock()
	return se.lastStatus[name]
}

// RunAll runs all seeders in alphabetical order, stopping on first error.
func (se *SeederExecutor) RunAll(ctx context.Context, out chan<- string) error {
	seeders, err := se.Seeders.List()
	if err != nil {
		return fmt.Errorf("list seeders: %w", err)
	}
	if len(seeders) == 0 {
		return nil
	}

	for i := range seeders {
		if err := se.RunOne(ctx, out, &seeders[i]); err != nil {
			return err
		}
	}
	return nil
}

// RunOne runs a single seeder's commands sequentially.
func (se *SeederExecutor) RunOne(ctx context.Context, out chan<- string, seeder *model.Seeder) (retErr error) {
	defer func() {
		status := "success"
		if retErr != nil {
			status = "error"
		}
		se.statusMu.Lock()
		se.lastStatus[seeder.Name] = status
		se.statusMu.Unlock()
	}()
	// Resolve target hostname
	svc, err := se.Services.Get(seeder.Target)
	if err != nil {
		return fmt.Errorf("resolve target %q: %w", seeder.Target, err)
	}
	if svc == nil {
		return fmt.Errorf("target service not found: %s", seeder.Target)
	}

	hostname := svc.Hostname

	// Wait for target to be healthy
	emit(out, fmt.Sprintf("[%s] waiting for %s to be healthy", seeder.Name, seeder.Target))
	if err := se.waitHealthy(ctx, hostname, healthTimeout); err != nil {
		return fmt.Errorf("[%s] target %s not healthy: %w", seeder.Name, seeder.Target, err)
	}

	for _, cmd := range seeder.Commands {
		start := time.Now()
		emit(out, fmt.Sprintf("[%s] running: %s", seeder.Name, cmd.Name))

		var execErr error
		if cmd.Script != "" {
			execErr = se.runScript(ctx, hostname, cmd)
		} else {
			execErr = se.runInline(ctx, hostname, cmd)
		}

		duration := time.Since(start)
		if execErr != nil {
			emit(out, fmt.Sprintf("[%s] ✗ %s: %s", seeder.Name, cmd.Name, execErr.Error()))
			return fmt.Errorf("seeder %q failed at %q: %w", seeder.Name, cmd.Name, execErr)
		}
		emit(out, fmt.Sprintf("[%s] ✓ %s (%s)", seeder.Name, cmd.Name, formatDuration(duration)))
	}
	return nil
}

// HasSeeders returns true if there are any seeder JSON files.
func (se *SeederExecutor) HasSeeders() bool {
	seeders, err := se.Seeders.List()
	if err != nil {
		return false
	}
	return len(seeders) > 0
}

func (se *SeederExecutor) runInline(ctx context.Context, hostname string, cmd model.SeederCommand) error {
	tctx, cancel := context.WithTimeout(ctx, longTimeout)
	defer cancel()

	c := se.Cmd.DockerCmd(tctx, "exec", hostname, "sh", "-c", cmd.Command)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func (se *SeederExecutor) runScript(ctx context.Context, hostname string, cmd model.SeederCommand) error {
	// Validate script filename (no path traversal)
	if strings.Contains(cmd.Script, "/") || strings.Contains(cmd.Script, "..") {
		return fmt.Errorf("invalid script path: %s", cmd.Script)
	}

	scriptPath := filepath.Join(se.Seeders.Dir(), cmd.Script)
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read script %s: %w", cmd.Script, err)
	}

	tctx, cancel := context.WithTimeout(ctx, longTimeout)
	defer cancel()

	args := append([]string{"exec", "-i", hostname}, strings.Fields(cmd.Interpreter)...)
	c := se.Cmd.DockerCmd(tctx, args...)
	c.Stdin = bytes.NewReader(scriptData)
	var combined bytes.Buffer
	c.Stdout = &combined
	c.Stderr = &combined
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(combined.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// waitHealthy polls a container's health status until it becomes healthy or the timeout expires.
// If the container has no health check, it falls back to isRunning().
func (se *SeederExecutor) waitHealthy(ctx context.Context, hostname string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for %s to become healthy (%s)", hostname, timeout)
		case <-ticker.C:
			status, err := se.inspectHealth(ctx, hostname)
			if err != nil {
				// Container might not have a health check — fall back to running check
				if se.isRunning(ctx, hostname) {
					return nil
				}
				continue
			}
			if status == "healthy" {
				return nil
			}
		}
	}
}

func (se *SeederExecutor) inspectHealth(ctx context.Context, hostname string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := se.Cmd.DockerCmd(tctx, "inspect",
		"--format", "{{.State.Health.Status}}",
		hostname,
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (se *SeederExecutor) isRunning(ctx context.Context, hostname string) bool {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, _ := se.Cmd.DockerCmd(tctx, "ps",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	).Output()
	return strings.TrimSpace(string(out)) == hostname
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
