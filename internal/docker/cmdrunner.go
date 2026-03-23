package docker

import (
	"context"
	"os/exec"
	"sync"

	"github.com/getkaze/keel/internal/model"
)

// CmdRunner abstracts Docker CLI execution so the Executor works
// transparently with both local and remote (SSH) targets.
type CmdRunner interface {
	// DockerCmd returns an *exec.Cmd that will execute "docker <args...>"
	// on the appropriate target (local or remote via SSH).
	DockerCmd(ctx context.Context, args ...string) *exec.Cmd

	// SyncFiles copies service file mounts to the target host.
	// For local targets this is a no-op; for remote targets it uses scp.
	SyncFiles(ctx context.Context, svc model.Service, keelDir string) error

	// GHCRLogin authenticates with GitHub Container Registry on the target.
	GHCRLogin(ctx context.Context, keelDir string) error

	// PortBind returns the address to bind container ports to (e.g. "127.0.0.1").
	PortBind() string
}

// ReloadableRunner wraps a CmdRunner and allows swapping the underlying
// implementation at runtime (e.g., when the target config changes).
type ReloadableRunner struct {
	mu    sync.RWMutex
	inner CmdRunner
}

// NewReloadableRunner creates a ReloadableRunner with the given initial runner.
func NewReloadableRunner(r CmdRunner) *ReloadableRunner {
	return &ReloadableRunner{inner: r}
}

func (r *ReloadableRunner) DockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inner.DockerCmd(ctx, args...)
}

func (r *ReloadableRunner) SyncFiles(ctx context.Context, svc model.Service, keelDir string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inner.SyncFiles(ctx, svc, keelDir)
}

func (r *ReloadableRunner) GHCRLogin(ctx context.Context, keelDir string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inner.GHCRLogin(ctx, keelDir)
}

func (r *ReloadableRunner) PortBind() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inner.PortBind()
}

// Swap replaces the underlying CmdRunner. Safe for concurrent use.
func (r *ReloadableRunner) Swap(newRunner CmdRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inner = newRunner
}
