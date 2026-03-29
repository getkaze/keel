package docker

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
)

// mockRunner implements CmdRunner for testing without Docker.
type mockRunner struct {
	portBind string
	cmds     [][]string // recorded docker args
	// cmdFunc optionally overrides DockerCmd behavior.
	cmdFunc func(ctx context.Context, args ...string) *exec.Cmd
}

func (m *mockRunner) DockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	m.cmds = append(m.cmds, args)
	if m.cmdFunc != nil {
		return m.cmdFunc(ctx, args...)
	}
	return exec.CommandContext(ctx, "echo", "mock")
}

func (m *mockRunner) SyncFiles(_ context.Context, _ model.Service, _ string) error { return nil }
func (m *mockRunner) GHCRLogin(_ context.Context, _ string) error                  { return nil }
func (m *mockRunner) PortBind() string                                              { return m.portBind }

func (m *mockRunner) lastArgs() []string {
	if len(m.cmds) == 0 {
		return nil
	}
	return m.cmds[len(m.cmds)-1]
}

// --- Dispatch tests ---

func TestDispatch_UnknownCommand(t *testing.T) {
	e := newTestExecutor(t, nil)
	out := make(chan string, 64)
	err := e.dispatch(context.Background(), out, "explode")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestDispatch_StartRequiresArg(t *testing.T) {
	e := newTestExecutor(t, nil)
	out := make(chan string, 64)
	err := e.dispatch(context.Background(), out, "start")
	if err == nil || !strings.Contains(err.Error(), "requires a service name") {
		t.Fatalf("expected missing arg error, got %v", err)
	}
}

func TestDispatch_StopRequiresArg(t *testing.T) {
	e := newTestExecutor(t, nil)
	out := make(chan string, 64)
	err := e.dispatch(context.Background(), out, "stop")
	if err == nil || !strings.Contains(err.Error(), "requires a service name") {
		t.Fatalf("expected missing arg error, got %v", err)
	}
}

func TestDispatch_RestartRequiresArg(t *testing.T) {
	e := newTestExecutor(t, nil)
	out := make(chan string, 64)
	err := e.dispatch(context.Background(), out, "restart")
	if err == nil || !strings.Contains(err.Error(), "requires a service name") {
		t.Fatalf("expected missing arg error, got %v", err)
	}
}

func TestDispatch_UpdateRequiresArg(t *testing.T) {
	e := newTestExecutor(t, nil)
	out := make(chan string, 64)
	err := e.dispatch(context.Background(), out, "update")
	if err == nil || !strings.Contains(err.Error(), "requires a service name") {
		t.Fatalf("expected missing arg error, got %v", err)
	}
}

func TestDispatch_StartUnknownService(t *testing.T) {
	e := newTestExecutor(t, nil)
	out := make(chan string, 64)
	err := e.dispatch(context.Background(), out, "start", "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("expected unknown service error, got %v", err)
	}
}

// --- Boot arg assembly tests ---

func TestBoot_DefaultNetwork(t *testing.T) {
	svc := model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	found := false
	for i, a := range args {
		if a == "--network" && i+1 < len(args) && args[i+1] == "keel-net" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --network keel-net in args: %v", args)
	}
}

func TestBoot_CustomNetwork(t *testing.T) {
	svc := model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8",
		Network:  "custom-net",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	found := false
	for i, a := range args {
		if a == "--network" && i+1 < len(args) && args[i+1] == "custom-net" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --network custom-net in args: %v", args)
	}
}

func TestBoot_PortBinding(t *testing.T) {
	svc := model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8",
		Ports:    model.PortConfig{External: 3306, Internal: 3306},
	}
	mr := &mockRunner{portBind: "0.0.0.0"}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	found := false
	for i, a := range args {
		if a == "-p" && i+1 < len(args) && args[i+1] == "0.0.0.0:3306:3306" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -p 0.0.0.0:3306:3306 in args: %v", args)
	}
}

func TestBoot_DefaultPortBind(t *testing.T) {
	svc := model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8",
		Ports:    model.PortConfig{External: 3306, Internal: 3306},
	}
	mr := &mockRunner{portBind: ""}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	found := false
	for i, a := range args {
		if a == "-p" && i+1 < len(args) && strings.HasPrefix(args[i+1], "127.0.0.1:") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -p 127.0.0.1:... in args: %v", args)
	}
}

func TestBoot_KeelLabels(t *testing.T) {
	svc := model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	hasManaged := false
	hasService := false
	for i, a := range args {
		if a == "--label" && i+1 < len(args) {
			if args[i+1] == "keel.managed=true" {
				hasManaged = true
			}
			if args[i+1] == "keel.service=mysql" {
				hasService = true
			}
		}
	}
	if !hasManaged {
		t.Error("missing --label keel.managed=true")
	}
	if !hasService {
		t.Error("missing --label keel.service=mysql")
	}
}

func TestBoot_NoPorts(t *testing.T) {
	svc := model.Service{
		Name:     "redis",
		Hostname: "keel-redis",
		Image:    "redis:7",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	for _, a := range args {
		if a == "-p" {
			t.Error("should not have -p flag when no ports configured")
		}
	}
}

func TestBoot_CommandWithSpaces(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "myapp:latest",
		Command:  "npm run dev",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	// Should end with: "npm", "run", "dev" (split by fields)
	n := len(args)
	if n < 3 || args[n-3] != "npm" || args[n-2] != "run" || args[n-1] != "dev" {
		t.Errorf("expected 'npm run dev' split as args at end: %v", args)
	}
}

func TestBoot_SimpleCommand(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "myapp:latest",
		Command:  "server",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	if args[len(args)-1] != "server" {
		t.Errorf("expected 'server' as last arg, got: %v", args)
	}
	for _, a := range args {
		if a == "sh" {
			t.Error("simple command should not use sh -c")
		}
	}
}

// --- Volume resolution tests ---

func TestResolveVolume_NamedVolume(t *testing.T) {
	e := &Executor{KeelDir: "/opt/keel"}
	result := e.resolveVolume("mydata:/var/lib/data")
	if result != "mydata:/var/lib/data" {
		t.Errorf("named volume should be unchanged, got %q", result)
	}
}

func TestResolveVolume_AbsolutePath(t *testing.T) {
	e := &Executor{KeelDir: "/opt/keel"}
	result := e.resolveVolume("/host/path:/container/path")
	if result != "/host/path:/container/path" {
		t.Errorf("absolute path should be unchanged, got %q", result)
	}
}

func TestResolveVolume_RelativePath(t *testing.T) {
	e := &Executor{KeelDir: "/opt/keel"}
	result := e.resolveVolume("./data:/var/lib/data")
	if result != "/opt/keel/data:/var/lib/data" {
		t.Errorf("expected /opt/keel/data:/var/lib/data, got %q", result)
	}
}

// --- Health check tests ---

func TestBoot_HealthCheckHTTP(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "myapp:latest",
		HealthCheck: &model.HealthCheck{
			Type:        "http",
			URL:         "http://127.0.0.1:8080/health",
			Interval:    15,
			Retries:     3,
			StartPeriod: 10,
		},
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	checks := map[string]string{
		"--health-cmd":          "wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || curl -sf http://127.0.0.1:8080/health >/dev/null 2>&1",
		"--health-interval":     "15s",
		"--health-retries":      "3",
		"--health-start-period": "10s",
	}
	for flag, want := range checks {
		found := false
		for i, a := range args {
			if a == flag && i+1 < len(args) && args[i+1] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s %s in args: %v", flag, want, args)
		}
	}
}

func TestBoot_HealthCheckCommand(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "myapp:latest",
		HealthCheck: &model.HealthCheck{
			Type:    "command",
			Command: "pg_isready -U postgres",
			Retries: 5,
		},
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	args := mr.lastArgs()
	found := false
	for i, a := range args {
		if a == "--health-cmd" && i+1 < len(args) && args[i+1] == "pg_isready -U postgres" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --health-cmd with command string in args: %v", args)
	}
}

func TestBoot_NoHealthCheck(t *testing.T) {
	svc := model.Service{
		Name:     "redis",
		Hostname: "keel-redis",
		Image:    "redis:7",
	}
	mr := &mockRunner{}
	e := &Executor{Runner: mr, KeelDir: "/tmp/keel"}
	out := make(chan string, 64)
	_ = e.boot(context.Background(), out, svc)

	for _, a := range mr.lastArgs() {
		if strings.HasPrefix(a, "--health-") {
			t.Errorf("unexpected health flag %q when HealthCheck is nil", a)
		}
	}
}

// --- Local registry tests ---

func TestUpdate_LocalRegistrySkipsPull(t *testing.T) {
	dir := t.TempDir()
	store := config.NewServiceStore(dir)
	_ = store.Save(model.Service{
		Name:     "mole",
		Hostname: "mole",
		Image:    "mole:local",
		Registry: "local",
		Network:  "keel-net",
	})

	mr := &mockRunner{}
	e := NewExecutor(dir, store, mr)
	out := make(chan string, 64)
	_ = e.updateService(context.Background(), out, "mole")

	for _, cmd := range mr.cmds {
		if len(cmd) > 0 && cmd[0] == "pull" {
			t.Errorf("pull should be skipped for local registry, got cmd: %v", cmd)
		}
	}
}

func TestUpdate_RemoteRegistryPulls(t *testing.T) {
	dir := t.TempDir()
	store := config.NewServiceStore(dir)
	_ = store.Save(model.Service{
		Name:     "nginx",
		Hostname: "keel-nginx",
		Image:    "nginx:latest",
		Network:  "keel-net",
	})

	mr := &mockRunner{}
	e := NewExecutor(dir, store, mr)
	out := make(chan string, 64)
	_ = e.updateService(context.Background(), out, "nginx")

	pulled := false
	for _, cmd := range mr.cmds {
		if len(cmd) > 0 && cmd[0] == "pull" {
			pulled = true
			break
		}
	}
	if !pulled {
		t.Error("expected pull to be called for remote registry")
	}
}

// --- emit test ---

func TestEmit_FullChannel(t *testing.T) {
	ch := make(chan string) // unbuffered
	// Should not block or panic
	emit(ch, "test message")
}

// --- helper ---

func newTestExecutor(t *testing.T, runner CmdRunner) *Executor {
	t.Helper()
	dir := t.TempDir()
	store := config.NewServiceStore(dir)
	if runner == nil {
		runner = &mockRunner{}
	}
	return NewExecutor(dir, store, runner)
}
