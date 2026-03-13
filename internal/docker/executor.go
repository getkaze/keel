package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
)

const (
	execTimeout = 30 * time.Second
	longTimeout = 300 * time.Second
)

// Executor runs Docker operations directly.
type Executor struct {
	Services    *config.ServiceStore
	KeelDir string
}

// NewExecutor creates an Executor for the given keel data directory.
func NewExecutor(keelDir string, services *config.ServiceStore) *Executor {
	return &Executor{
		Services:    services,
		KeelDir: keelDir,
	}
}

// Stream dispatches a command and streams output line by line.
func (e *Executor) Stream(ctx context.Context, command string, args ...string) (<-chan string, <-chan error) {
	lines := make(chan string, 64)
	errc := make(chan error, 1)

	go func() {
		defer close(lines)
		defer close(errc)
		errc <- e.dispatch(ctx, lines, command, args...)
	}()

	return lines, errc
}

func (e *Executor) dispatch(ctx context.Context, out chan<- string, command string, args ...string) error {
	switch command {
	case "start":
		if len(args) == 0 {
			return fmt.Errorf("start requires a service name")
		}
		return e.startService(ctx, out, args[0])
	case "stop":
		if len(args) == 0 {
			return fmt.Errorf("stop requires a service name")
		}
		return e.stopService(ctx, out, args[0])
	case "restart":
		if len(args) == 0 {
			return fmt.Errorf("restart requires a service name")
		}
		if err := e.stopService(ctx, out, args[0]); err != nil {
			return err
		}
		return e.startService(ctx, out, args[0])
	case "update":
		if len(args) == 0 {
			return fmt.Errorf("update requires a service name")
		}
		return e.updateService(ctx, out, args[0])
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// --- start ---

func (e *Executor) startService(ctx context.Context, out chan<- string, name string) error {
	svc, err := e.Services.Get(name)
	if err != nil {
		return fmt.Errorf("service lookup: %w", err)
	}
	if svc == nil {
		return fmt.Errorf("unknown service: %s", name)
	}
	return e.startOne(ctx, out, svc)
}

func (e *Executor) startOne(ctx context.Context, out chan<- string, svc *model.Service) error {
	if isRunning(ctx, svc.Hostname) {
		emit(out, fmt.Sprintf("[%s] already running", svc.Name))
		return nil
	}

	network := svc.Network
	if network == "" {
		network = "keel-net"
	}
	if err := ensureNetwork(ctx, network, e.networkSubnet()); err != nil {
		return fmt.Errorf("network: %w", err)
	}

	if containerExists(ctx, svc.Hostname) {
		emit(out, fmt.Sprintf("[%s] starting", svc.Name))
		return dockerStream(ctx, out, "docker", "start", svc.Hostname)
	}

	emit(out, fmt.Sprintf("[%s] booting", svc.Name))
	return e.boot(ctx, out, *svc)
}

// --- stop ---

func (e *Executor) stopService(ctx context.Context, out chan<- string, name string) error {
	svc, err := e.Services.Get(name)
	if err != nil {
		return fmt.Errorf("service lookup: %w", err)
	}
	if svc == nil {
		return fmt.Errorf("unknown service: %s", name)
	}
	return e.stopOne(ctx, out, svc)
}

func (e *Executor) stopOne(ctx context.Context, out chan<- string, svc *model.Service) error {
	if !isRunning(ctx, svc.Hostname) {
		emit(out, fmt.Sprintf("[%s] not running", svc.Name))
		return nil
	}
	emit(out, fmt.Sprintf("[%s] stopping", svc.Name))
	return dockerStream(ctx, out, "docker", "stop", svc.Hostname)
}

// --- update ---

func (e *Executor) updateService(ctx context.Context, out chan<- string, name string) error {
	svc, err := e.Services.Get(name)
	if err != nil {
		return err
	}
	if svc == nil {
		return fmt.Errorf("unknown service: %s", name)
	}

	if svc.Registry == "ghcr" {
		if err := e.ghcrLogin(ctx, out); err != nil {
			return fmt.Errorf("ghcr: %w", err)
		}
	}

	emit(out, fmt.Sprintf("[%s] pulling %s", svc.Name, svc.Image))
	_ = dockerStream(ctx, out, "docker", "pull", svc.Image)

	emit(out, fmt.Sprintf("[%s] removing container", svc.Name))
	_ = dockerSilent(ctx, "docker", "rm", "-f", svc.Hostname)

	emit(out, fmt.Sprintf("[%s] booting", svc.Name))
	return e.boot(ctx, out, *svc)
}

// RemoveContainer stops and removes a container by hostname.
func (e *Executor) RemoveContainer(ctx context.Context, hostname string) error {
	return dockerSilent(ctx, "docker", "rm", "-f", hostname)
}

// --- boot ---

func (e *Executor) boot(ctx context.Context, out chan<- string, svc model.Service) error {
	network := svc.Network
	if network == "" {
		network = "keel-net"
	}

	if svc.Registry == "ghcr" {
		if err := e.ghcrLogin(ctx, out); err != nil {
			return fmt.Errorf("ghcr: %w", err)
		}
	}

	args := []string{
		"run", "-d",
		"--name", svc.Hostname,
		"--hostname", svc.Hostname,
		"--network", network,
		"--restart", "unless-stopped",
	}

	if svc.Ports.External > 0 && svc.Ports.Internal > 0 {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", svc.Ports.External, svc.Ports.Internal))
	}

	for k, v := range svc.Environment {
		args = append(args, "-e", k+"="+v)
	}
	for _, vol := range svc.Volumes {
		args = append(args, "-v", e.resolveVolume(vol))
	}
	for _, f := range svc.Files {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) == 2 {
			src := filepath.Join(e.KeelDir, parts[0])
			args = append(args, "-v", src+":"+parts[1]+":ro")
		}
	}

	args = append(args, svc.Image)
	if svc.Command != "" {
		args = append(args, strings.Fields(svc.Command)...)
	}

	return dockerStream(ctx, out, append([]string{"docker"}, args...)...)
}

// resolveVolume converts a relative bind-mount source to an absolute path.
// Named volumes (no path separator in source) are returned unchanged.
func (e *Executor) resolveVolume(vol string) string {
	parts := strings.SplitN(vol, ":", 2)
	src := parts[0]
	// Named volume: no path separator — Docker manages it, leave as-is.
	if !strings.Contains(src, "/") && !strings.HasPrefix(src, ".") {
		return vol
	}
	// Already absolute.
	if filepath.IsAbs(src) {
		return vol
	}
	abs := filepath.Join(e.KeelDir, src)
	if len(parts) == 2 {
		return abs + ":" + parts[1]
	}
	return abs
}

// --- helpers ---

func (e *Executor) networkSubnet() string {
	cfg, err := e.Services.GlobalConfig()
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.NetworkSubnet
}

func ensureNetwork(ctx context.Context, network, subnet string) error {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// network inspect exits 0 if it exists, non-zero otherwise
	if err := exec.CommandContext(tctx, "docker", "network", "inspect", network).Run(); err == nil {
		return nil
	}

	// Try with configured subnet first; if it conflicts, retry without it
	// (Docker will auto-assign a free subnet).
	if subnet != "" {
		cmd := exec.CommandContext(tctx, "docker", "network", "create", "--driver", "bridge", "--subnet", subnet, network)
		if cmd.Run() == nil {
			return nil
		}
		log.Printf("network: subnet %s conflict, retrying without fixed subnet", subnet)
	}

	cmd := exec.CommandContext(tctx, "docker", "network", "create", "--driver", "bridge", network)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker network create %s: %w: %s", network, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func isRunning(ctx context.Context, hostname string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "docker", "ps",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	).Output()
	return strings.TrimSpace(string(out)) == hostname
}

func containerExists(ctx context.Context, hostname string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	).Output()
	return strings.TrimSpace(string(out)) == hostname
}

func emit(out chan<- string, msg string) {
	select {
	case out <- msg:
	default:
	}
}

func dockerStream(ctx context.Context, out chan<- string, args ...string) error {
	tctx, cancel := context.WithTimeout(ctx, longTimeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, args[0], args[1:]...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", args[0], err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- scanner.Text():
		}
	}

	if err := cmd.Wait(); err != nil {
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return err
	}
	return nil
}

func dockerSilent(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (e *Executor) ghcrLogin(ctx context.Context, out chan<- string) error {
	patFile := filepath.Join(e.KeelDir, "state", "ghcr-pat")
	userFile := filepath.Join(e.KeelDir, "state", "ghcr-user")
	pat, err := os.ReadFile(patFile)
	if err != nil || len(bytes.TrimSpace(pat)) == 0 {
		return fmt.Errorf("PAT not found at %s", patFile)
	}
	user, err := os.ReadFile(userFile)
	if err != nil || len(bytes.TrimSpace(user)) == 0 {
		return fmt.Errorf("GitHub username not found at %s", userFile)
	}
	emit(out, "logging in to ghcr.io")
	cmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io",
		"-u", strings.TrimSpace(string(user)),
		"--password-stdin")
	cmd.Stdin = bytes.NewReader(pat)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ghcr login: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
