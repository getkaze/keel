package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/getkaze/keel/internal/model"
)

// LocalRunner implements CmdRunner for local Docker targets.
type LocalRunner struct{}

// NewLocalRunner creates a runner that executes Docker commands locally.
func NewLocalRunner() *LocalRunner {
	return &LocalRunner{}
}

// DockerCmd returns an exec.Cmd that runs "docker <args...>" locally.
func (r *LocalRunner) DockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}

// SyncFiles is a no-op for local targets — files are already on the host.
func (r *LocalRunner) SyncFiles(_ context.Context, _ model.Service, _ string) error {
	return nil
}

// GHCRLogin authenticates with GHCR using credentials from the data directory.
func (r *LocalRunner) GHCRLogin(ctx context.Context, keelDir string) error {
	userPath := keelDir + "/state/ghcr-user"
	patPath := keelDir + "/state/ghcr-pat"

	user, err := os.ReadFile(userPath)
	if err != nil {
		return nil // no credentials configured
	}
	pat, err := os.ReadFile(patPath)
	if err != nil {
		return nil
	}

	ghcrUser := strings.TrimSpace(string(user))
	ghcrPat := strings.TrimSpace(string(pat))
	if ghcrUser == "" || ghcrPat == "" {
		return nil
	}

	cmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io", "-u", ghcrUser, "--password-stdin")
	cmd.Stdin = strings.NewReader(ghcrPat)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ghcr login: %s: %w", string(out), err)
	}
	return nil
}

// PortBind returns "127.0.0.1" for local targets.
func (r *LocalRunner) PortBind() string {
	return "127.0.0.1"
}
