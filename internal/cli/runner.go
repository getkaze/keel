package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
	keelssh "github.com/getkaze/keel/internal/ssh"
)

// Runner executes docker commands on a target (local or remote via SSH).
type Runner struct {
	target      *config.TargetConfig
	keelDir string
}

// NewRunner creates a Runner for the given target.
func NewRunner(target *config.TargetConfig, keelDir string) *Runner {
	return &Runner{target: target, keelDir: keelDir}
}

// Exec runs a docker command and streams output to stdout/stderr.
func (r *Runner) Exec(ctx context.Context, dockerArgs ...string) error {
	cmd := r.buildCmd(ctx, dockerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Output runs a docker command and returns stdout as a trimmed string.
func (r *Runner) Output(ctx context.Context, dockerArgs ...string) (string, error) {
	cmd := r.buildCmd(ctx, dockerArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRunning returns true if the container is in running state.
func (r *Runner) IsRunning(ctx context.Context, hostname string) bool {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := r.Output(tctx,
		"ps",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == hostname
}

// ContainerExists returns true if the container exists (running or stopped).
func (r *Runner) ContainerExists(ctx context.Context, hostname string) bool {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := r.Output(tctx,
		"ps", "-a",
		"--filter", "name=^/"+hostname+"$",
		"--format", "{{.Names}}",
	)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == hostname
}

// StartOne starts a container: docker start if it exists, boot (docker run) if not.
func (r *Runner) StartOne(ctx context.Context, svc model.Service, keelDir string) error {
	if r.IsRunning(ctx, svc.Hostname) {
		fmt.Printf("[%s] already running\n", svc.Name)
		return nil
	}

	network := svc.Network
	if network == "" {
		network = "keel-net"
	}
	if err := r.ensureNetwork(ctx, network); err != nil {
		return fmt.Errorf("network: %w", err)
	}

	if r.ContainerExists(ctx, svc.Hostname) {
		fmt.Printf("[%s] starting\n", svc.Name)
		return r.Exec(ctx, "start", svc.Hostname)
	}
	fmt.Printf("[%s] booting\n", svc.Name)
	return r.Boot(ctx, svc, keelDir)
}

// StopOne stops a running container.
func (r *Runner) StopOne(ctx context.Context, svc model.Service) error {
	if !r.IsRunning(ctx, svc.Hostname) {
		fmt.Printf("[%s] not running\n", svc.Name)
		return nil
	}
	fmt.Printf("[%s] stopping\n", svc.Name)
	return r.Exec(ctx, "stop", svc.Hostname)
}

// Boot creates and runs a new container from a service definition.
func (r *Runner) Boot(ctx context.Context, svc model.Service, keelDir string) error {
	network := svc.Network
	if network == "" {
		network = "keel-net"
	}
	if err := r.ensureNetwork(ctx, network); err != nil {
		return fmt.Errorf("network: %w", err)
	}

	if svc.Registry == "ghcr" {
		if err := r.GHCRLogin(ctx, keelDir); err != nil {
			return fmt.Errorf("ghcr: %w", err)
		}
	}

	// For remote targets, sync files to the remote host before boot
	// so that volume mounts can find them.
	if r.target.Mode != "local" && len(svc.Files) > 0 {
		if err := r.SyncFiles(ctx, svc, keelDir); err != nil {
			return fmt.Errorf("sync files: %w", err)
		}
	}

	args := buildRunArgs(svc, keelDir, r.target.PortBind)
	return r.Exec(ctx, args...)
}

// SyncFiles copies service files to the remote host via scp so that
// Docker volume mounts work. Each file entry has the format
// "relative/path:/container/path"; we copy the local file to the same
// absolute path (keelDir + relative) on the remote host.
func (r *Runner) SyncFiles(ctx context.Context, svc model.Service, keelDir string) error {
	for _, f := range svc.Files {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			continue
		}

		// Validate the relative path does not escape keelDir via traversal.
		cleaned := filepath.Clean(parts[0])
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			return fmt.Errorf("sync files: path traversal rejected: %s", parts[0])
		}

		localSrc := filepath.Join(keelDir, cleaned)
		remoteDst := filepath.Join(keelDir, cleaned)

		// Double-check resolved paths are within keelDir.
		if !strings.HasPrefix(localSrc, filepath.Clean(keelDir)+string(filepath.Separator)) {
			return fmt.Errorf("sync files: resolved path outside keel dir: %s", localSrc)
		}

		// Ensure parent directory exists on remote host and remove any
		// stale directory at the destination path (Docker creates a
		// directory when a bind-mount source is missing).
		mkdirArgs := r.buildSSHArgs()
		parentDir := filepath.Dir(remoteDst)
		mkdirArgs = append(mkdirArgs, fmt.Sprintf(
			"sudo rm -rf %s && sudo mkdir -p %s && sudo chown $USER %s",
			shellQuote(remoteDst),
			shellQuote(parentDir),
			shellQuote(parentDir),
		))
		mkdirCmd := exec.CommandContext(ctx, "ssh", mkdirArgs...)
		mkdirCmd.Stderr = os.Stderr
		if err := mkdirCmd.Run(); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(remoteDst), err)
		}

		// Build scp args with the same SSH options (key, jump host).
		scpArgs := []string{"-r", "-o", "StrictHostKeyChecking=accept-new", "-o", "BatchMode=yes", "-o", "LogLevel=ERROR"}
		if r.target.SSHKey != "" {
			scpArgs = append(scpArgs, "-i", keelssh.ExpandHome(r.target.SSHKey))
		}
		if r.target.SSHJump != "" {
			proxyCmd := "ssh -o StrictHostKeyChecking=accept-new -o BatchMode=yes -o LogLevel=ERROR"
			if r.target.SSHKey != "" {
				proxyCmd += " -i " + keelssh.ExpandHome(r.target.SSHKey)
			}
			proxyCmd += " -W %h:%p " + r.target.SSHJump
			scpArgs = append(scpArgs, "-o", "ProxyCommand="+proxyCmd)
		}
		scpArgs = append(scpArgs, localSrc, r.target.SSHUser+"@"+r.target.Host+":"+remoteDst)

		fmt.Printf("[%s] syncing %s\n", svc.Name, parts[0])
		scpCmd := exec.CommandContext(ctx, "scp", scpArgs...)
		scpCmd.Stderr = os.Stderr
		if err := scpCmd.Run(); err != nil {
			return fmt.Errorf("scp %s: %w", localSrc, err)
		}
	}
	return nil
}

// DevOne runs a service in development mode: builds a dev image from the
// dockerfile lines in the service JSON, mounts the local source path into
// the container, and streams output to the terminal (foreground, not detached).
// Only supported on local targets.
func (r *Runner) DevOne(ctx context.Context, svc model.Service, localPath string) error {
	if r.target.Mode != "local" {
		return fmt.Errorf("dev mode is only supported on local targets (current: %s)", r.target.Name)
	}
	if svc.Dev == nil || len(svc.Dev.Dockerfile) == 0 {
		return fmt.Errorf("service %q has no dev.dockerfile defined in its JSON", svc.Name)
	}

	devTag := "keel-dev/" + svc.Name + ":dev"
	if err := buildDevImage(ctx, svc, localPath, devTag); err != nil {
		return fmt.Errorf("build dev image: %w", err)
	}

	// Stop and remove existing container so the new one can use the same name.
	if r.ContainerExists(ctx, svc.Hostname) {
		fmt.Printf("[%s] stopping existing container\n", svc.Name)
		_ = r.Exec(ctx, "stop", svc.Hostname)
		_ = r.Exec(ctx, "rm", "-f", svc.Hostname)
	}

	args := buildDevRunArgs(svc, localPath, devTag, r.target.PortBind)
	fmt.Printf("[%s] dev mode — %s\n", svc.Name, devTag)
	fmt.Printf("[%s] mounting: %s → %s\n", svc.Name, localPath, workdirFromDockerfile(svc.Dev.Dockerfile))
	fmt.Printf("[%s] press Ctrl+C to stop\n\n", svc.Name)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// buildDevImage builds the dev Docker image by piping the dockerfile lines
// from the service JSON into `docker build`. The local path is used as build
// context so COPY instructions work (e.g. COPY go.mod go.sum ./).
func buildDevImage(ctx context.Context, svc model.Service, localPath, tag string) error {
	dockerfileContent := strings.Join(svc.Dev.Dockerfile, "\n") + "\n"
	fmt.Printf("[%s] building dev image %s...\n", svc.Name, tag)

	cmd := exec.CommandContext(ctx, "docker", "build", "-f", "-", "-t", tag, localPath)
	cmd.Stdin = strings.NewReader(dockerfileContent)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// workdirFromDockerfile extracts the last WORKDIR value from the dockerfile lines.
// Falls back to /app if none is found.
func workdirFromDockerfile(lines []string) string {
	workdir := "/app"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "WORKDIR ") {
			workdir = strings.TrimSpace(trimmed[len("WORKDIR "):])
		}
	}
	return workdir
}

// buildDevRunArgs assembles the `docker run` arguments for dev mode.
// Key differences from buildRunArgs: no -d, no --restart, adds volume mount,
// uses dev image, and may override the command.
func buildDevRunArgs(svc model.Service, localPath, image, portBind string) []string {
	if portBind == "" {
		portBind = "127.0.0.1"
	}
	network := svc.Network
	if network == "" {
		network = "keel-net"
	}

	args := []string{
		"run", "--rm",
		"--name", svc.Hostname,
		"--hostname", svc.Hostname,
		"--network", network,
	}

	if svc.Ports.External > 0 && svc.Ports.Internal > 0 {
		args = append(args, "-p",
			fmt.Sprintf("%s:%d:%d", portBind, svc.Ports.External, svc.Ports.Internal))
	}

	for _, cap := range svc.Dev.CapAdd {
		args = append(args, "--cap-add", cap)
	}

	for k, v := range svc.Environment {
		args = append(args, "-e", k+"="+v)
	}

	// Existing volumes from the service definition (e.g. data mounts).
	for _, vol := range svc.Volumes {
		args = append(args, "-v", vol)
	}

	// Source code mount: local path → WORKDIR inside the container.
	args = append(args, "-v", localPath+":"+workdirFromDockerfile(svc.Dev.Dockerfile))

	args = append(args, image)

	// Command from dev.command (passed as argument list to avoid shell-quoting issues).
	if len(svc.Dev.Command) > 0 {
		args = append(args, svc.Dev.Command...)
	}

	return args
}

// ensureNetwork creates the Docker network if it doesn't exist.
// If the configured subnet conflicts, it retries without a fixed subnet
// so Docker auto-assigns a free one.
func (r *Runner) ensureNetwork(ctx context.Context, network string) error {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := r.Output(tctx, "network", "inspect", network); err == nil {
		return nil
	}
	subnet := r.networkSubnet()
	if subnet != "" {
		err := r.Exec(tctx, "network", "create", "--driver", "bridge", "--subnet", subnet, network)
		if err == nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "network: subnet %s conflict, retrying without fixed subnet\n", subnet)
	}
	return r.Exec(tctx, "network", "create", "--driver", "bridge", network)
}

// networkSubnet reads the configured subnet from global config, or returns "".
func (r *Runner) networkSubnet() string {
	store := config.NewServiceStore(r.keelDir)
	cfg, err := store.GlobalConfig()
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.NetworkSubnet
}

// GHCRLogin logs in to ghcr.io using credentials stored in keelDir/state/.
// Only supported on local targets; silently skipped for remote targets.
func (r *Runner) GHCRLogin(ctx context.Context, keelDir string) error {
	pat, err := os.ReadFile(filepath.Join(keelDir, "state/ghcr-pat"))
	if err != nil || len(bytes.TrimSpace(pat)) == 0 {
		return fmt.Errorf("PAT not found at %s/state/ghcr-pat", keelDir)
	}
	user, err := os.ReadFile(filepath.Join(keelDir, "state/ghcr-user"))
	if err != nil || len(bytes.TrimSpace(user)) == 0 {
		return fmt.Errorf("GitHub username not found at %s/state/ghcr-user", keelDir)
	}

	ghcrUser := strings.TrimSpace(string(user))
	ghcrPat := bytes.TrimSpace(pat)

	fmt.Println("logging in to ghcr.io")

	if r.target.Mode == "local" {
		cmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io",
			"-u", ghcrUser, "--password-stdin")
		cmd.Stdin = bytes.NewReader(ghcrPat)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Remote: pipe PAT via SSH stdin so it never appears in process args.
	sshArgs := r.buildSSHArgs()
	sshArgs = append(sshArgs, fmt.Sprintf("docker login ghcr.io -u %s --password-stdin",
		shellQuote(ghcrUser)))
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Stdin = bytes.NewReader(append(ghcrPat, '\n'))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PortBind returns the address to bind container ports to for this target.
func (r *Runner) PortBind() string {
	return r.target.PortBind
}

// DockerCmd implements docker.CmdRunner, routing via SSH for remote targets.
func (r *Runner) DockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	return r.buildCmd(ctx, args...)
}

// buildCmd constructs the exec.Cmd for a docker command, routed via SSH for remote targets.
func (r *Runner) buildCmd(ctx context.Context, dockerArgs ...string) *exec.Cmd {
	if r.target.Mode == "local" {
		return exec.CommandContext(ctx, "docker", dockerArgs...)
	}

	sshArgs := r.buildSSHArgs()
	allDockerArgs := append([]string{"docker"}, dockerArgs...)
	sshArgs = append(sshArgs, shellJoin(allDockerArgs))
	return exec.CommandContext(ctx, "ssh", sshArgs...)
}

// buildSSHArgs returns the SSH flags for the current target.
func (r *Runner) buildSSHArgs() []string {
	return keelssh.BuildArgs(r.target)
}

// buildRunArgs assembles the arguments for a `docker run` command from a service definition.
func buildRunArgs(svc model.Service, keelDir, portBind string) []string {
	if portBind == "" {
		portBind = "127.0.0.1"
	}
	network := svc.Network
	if network == "" {
		network = "keel-net"
	}

	args := []string{
		"run", "-d",
		"--name", svc.Hostname,
		"--hostname", svc.Hostname,
		"--network", network,
		"--restart", "unless-stopped",
	}

	if svc.Ports.External > 0 && svc.Ports.Internal > 0 {
		args = append(args, "-p",
			fmt.Sprintf("%s:%d:%d", portBind, svc.Ports.External, svc.Ports.Internal))
	}

	for k, v := range svc.Environment {
		args = append(args, "-e", k+"="+v)
	}
	for _, vol := range svc.Volumes {
		args = append(args, "-v", resolveVolume(vol, keelDir))
	}
	for _, f := range svc.Files {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) == 2 {
			src := filepath.Join(keelDir, parts[0])
			args = append(args, "-v", src+":"+parts[1]+":ro")
		}
	}

	args = append(args, svc.Image)
	if svc.Command != "" {
		if strings.ContainsAny(svc.Command, " \t\"'") {
			args = append(args, "sh", "-c", svc.Command)
		} else {
			args = append(args, svc.Command)
		}
	}
	return args
}

// resolveVolume converts a relative bind-mount source to an absolute path.
// Named volumes (no path separator in source) are returned unchanged.
func resolveVolume(vol, keelDir string) string {
	parts := strings.SplitN(vol, ":", 2)
	src := parts[0]
	if !strings.Contains(src, "/") && !strings.HasPrefix(src, ".") {
		return vol
	}
	if filepath.IsAbs(src) {
		return vol
	}
	abs := filepath.Join(keelDir, src)
	if len(parts) == 2 {
		return abs + ":" + parts[1]
	}
	return abs
}

// shellQuote wraps a string in single quotes, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellJoin joins args into a shell-safe string for SSH execution.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\n\"'\\$`|&;<>(){}") {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}
