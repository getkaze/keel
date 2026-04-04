package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
)

const (
	execTimeout = 30 * time.Second
	longTimeout = 300 * time.Second
)

// Executor runs Docker operations through a CmdRunner, which may target
// local or remote (SSH) Docker hosts transparently.
type Executor struct {
	Services *config.ServiceStore
	KeelDir  string
	Runner   CmdRunner
}

// NewExecutor creates an Executor for the given keel data directory.
func NewExecutor(keelDir string, services *config.ServiceStore, runner CmdRunner) *Executor {
	return &Executor{
		Services: services,
		KeelDir:  keelDir,
		Runner:   runner,
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
	if e.isRunning(ctx, svc.Hostname) {
		emit(out, fmt.Sprintf("[%s] already running", svc.Name))
		return nil
	}

	network := svc.Network
	if network == "" {
		network = "keel-net"
	}
	if err := e.ensureNetwork(ctx, network); err != nil {
		return fmt.Errorf("network: %w", err)
	}

	if e.containerExists(ctx, svc.Hostname) {
		emit(out, fmt.Sprintf("[%s] starting", svc.Name))
		return e.dockerStream(ctx, out, "start", svc.Hostname)
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
	if !e.isRunning(ctx, svc.Hostname) {
		emit(out, fmt.Sprintf("[%s] not running", svc.Name))
		return nil
	}
	emit(out, fmt.Sprintf("[%s] stopping", svc.Name))
	return e.dockerStream(ctx, out, "stop", svc.Hostname)
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
		emit(out, "logging in to ghcr.io")
		if err := e.Runner.GHCRLogin(ctx, e.KeelDir); err != nil {
			return fmt.Errorf("ghcr: %w", err)
		}
	}

	if svc.Registry == "local" {
		emit(out, fmt.Sprintf("[%s] local image, skipping pull", svc.Name))
	} else {
		emit(out, fmt.Sprintf("[%s] pulling %s", svc.Name, svc.Image))
		if err := e.dockerStream(ctx, out, "pull", svc.Image); err != nil {
			emit(out, fmt.Sprintf("[%s] pull failed, keeping existing container: %v", svc.Name, err))
			return fmt.Errorf("pull %s: %w", svc.Image, err)
		}
	}

	emit(out, fmt.Sprintf("[%s] removing container", svc.Name))
	_ = e.dockerSilent(ctx, "rm", "-f", svc.Hostname)

	emit(out, fmt.Sprintf("[%s] booting", svc.Name))
	return e.boot(ctx, out, *svc)
}

// RemoveContainer stops and removes a container by hostname.
func (e *Executor) RemoveContainer(ctx context.Context, hostname string) error {
	return e.dockerSilent(ctx, "rm", "-f", hostname)
}

// --- boot ---

func (e *Executor) boot(ctx context.Context, out chan<- string, svc model.Service) error {
	network := svc.Network
	if network == "" {
		network = "keel-net"
	}

	if svc.Registry == "ghcr" {
		emit(out, "logging in to ghcr.io")
		if err := e.Runner.GHCRLogin(ctx, e.KeelDir); err != nil {
			return fmt.Errorf("ghcr: %w", err)
		}
	}

	// Sync files to remote host before boot so volume mounts work.
	if len(svc.Files) > 0 {
		if err := e.Runner.SyncFiles(ctx, svc, e.KeelDir); err != nil {
			return fmt.Errorf("sync files: %w", err)
		}
	}

	portBind := e.Runner.PortBind()
	if portBind == "" {
		portBind = "127.0.0.1"
	}

	args := []string{"run", "-d"}
	if svc.Platform != "" {
		args = append(args, "--platform", svc.Platform)
	}
	args = append(args,
		"--name", svc.Hostname,
		"--hostname", svc.Hostname,
		"--network", network,
		"--restart", "unless-stopped",
		"--label", "keel.managed=true",
		"--label", "keel.service="+svc.Name,
	)

	if svc.Ports.External > 0 && svc.Ports.Internal > 0 {
		args = append(args, "-p", fmt.Sprintf("%s:%d:%d", portBind, svc.Ports.External, svc.Ports.Internal))
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

	if svc.HealthCheck != nil {
		hc := svc.HealthCheck
		var healthCmd string
		switch hc.Type {
		case "http":
			healthCmd = "wget -qO- " + hc.URL + " >/dev/null 2>&1 || curl -sf " + hc.URL + " >/dev/null 2>&1"
		case "command":
			healthCmd = hc.Command
		}
		if healthCmd != "" {
			args = append(args, "--health-cmd", healthCmd)
		}
		if hc.Interval > 0 {
			args = append(args, "--health-interval", strconv.Itoa(hc.Interval)+"s")
		}
		if hc.Retries > 0 {
			args = append(args, "--health-retries", strconv.Itoa(hc.Retries))
		}
		if hc.StartPeriod > 0 {
			args = append(args, "--health-start-period", strconv.Itoa(hc.StartPeriod)+"s")
		}
	}

	args = append(args, svc.Image)
	if svc.Command != "" {
		args = append(args, strings.Fields(svc.Command)...)
	}

	return e.dockerStream(ctx, out, args...)
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

func (e *Executor) ensureNetwork(ctx context.Context, network string) error {
	subnet := e.networkSubnet()
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// network inspect exits 0 if it exists, non-zero otherwise
	if cmd := e.Runner.DockerCmd(tctx, "network", "inspect", network); cmd.Run() == nil {
		return nil
	}

	// Try with configured subnet first; if it conflicts, retry without it
	// (Docker will auto-assign a free subnet).
	if subnet != "" {
		cmd := e.Runner.DockerCmd(tctx, "network", "create", "--driver", "bridge", "--subnet", subnet, network)
		if cmd.Run() == nil {
			return nil
		}
		log.Printf("network: subnet %s conflict, retrying without fixed subnet", subnet)
	}

	cmd := e.Runner.DockerCmd(tctx, "network", "create", "--driver", "bridge", network)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker network create %s: %w: %s", network, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (e *Executor) isRunning(ctx context.Context, hostname string) bool {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := e.Runner.DockerCmd(tctx, "ps",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == hostname
}

func (e *Executor) containerExists(ctx context.Context, hostname string) bool {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := e.Runner.DockerCmd(tctx, "ps", "-a",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == hostname
}

func emit(out chan<- string, msg string) {
	select {
	case out <- msg:
	default:
		log.Printf("sse: dropped message (channel full): %s", msg)
	}
}

func (e *Executor) dockerStream(ctx context.Context, out chan<- string, dockerArgs ...string) error {
	tctx, cancel := context.WithTimeout(ctx, longTimeout)
	defer cancel()

	cmd := e.Runner.DockerCmd(tctx, dockerArgs...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start docker: %w", err)
	}

	// Idle timeout: kill the process if no output is received for idleTimeout.
	const idleTimeout = 10 * time.Minute
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

	scanDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			idle.Reset(idleTimeout)
			select {
			case <-tctx.Done():
				scanDone <- tctx.Err()
				return
			case out <- scanner.Text():
			default:
				log.Printf("sse: dropped stream line (channel full): %s", scanner.Text())
			}
		}
		scanDone <- scanner.Err()
	}()

	select {
	case err := <-scanDone:
		if err != nil {
			return err
		}
	case <-idle.C:
		log.Printf("docker stream: idle timeout (%v) — killing process", idleTimeout)
		emit(out, "timeout: no output received, aborting")
		cancel()
		<-scanDone
		return fmt.Errorf("docker stream idle timeout (%v)", idleTimeout)
	}

	if err := cmd.Wait(); err != nil {
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return err
	}
	return nil
}

func (e *Executor) dockerSilent(ctx context.Context, dockerArgs ...string) error {
	tctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	cmd := e.Runner.DockerCmd(tctx, dockerArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
