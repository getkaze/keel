package cli

import (
	"strings"
	"testing"

	"github.com/getkaze/keel/internal/model"
)

// --- buildRunArgs tests ---

func TestBuildRunArgs_Basic(t *testing.T) {
	svc := model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8",
		Ports:    model.PortConfig{External: 3306, Internal: 3306},
	}
	args := buildRunArgs(svc, "/opt/keel", "127.0.0.1")

	assertContains(t, args, "run")
	assertContains(t, args, "-d")
	assertContainsPair(t, args, "--name", "keel-mysql")
	assertContainsPair(t, args, "--hostname", "keel-mysql")
	assertContainsPair(t, args, "--network", "keel-net")
	assertContainsPair(t, args, "-p", "127.0.0.1:3306:3306")
}

func TestBuildRunArgs_CustomNetwork(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "app:latest",
		Network:  "my-net",
	}
	args := buildRunArgs(svc, "/opt/keel", "")
	assertContainsPair(t, args, "--network", "my-net")
}

func TestBuildRunArgs_DefaultPortBind(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "app:latest",
		Ports:    model.PortConfig{External: 8080, Internal: 80},
	}
	args := buildRunArgs(svc, "/opt/keel", "")
	assertContainsPair(t, args, "-p", "127.0.0.1:8080:80")
}

func TestBuildRunArgs_NoPorts(t *testing.T) {
	svc := model.Service{
		Name:     "worker",
		Hostname: "keel-worker",
		Image:    "worker:latest",
	}
	args := buildRunArgs(svc, "/opt/keel", "")
	for _, a := range args {
		if a == "-p" {
			t.Error("should not have -p flag when no ports")
		}
	}
}

func TestBuildRunArgs_CommandWithSpaces(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "app:latest",
		Command:  "npm run start",
	}
	args := buildRunArgs(svc, "/opt/keel", "")
	n := len(args)
	// Command is split into fields — no sh -c wrapper
	if n < 3 || args[n-3] != "npm" || args[n-2] != "run" || args[n-1] != "start" {
		t.Errorf("expected 'npm run start' split as args at end, got: %v", args[n-3:])
	}
	for _, a := range args {
		if a == "sh" {
			t.Error("command with spaces must not use sh -c wrapper")
		}
	}
}

func TestBuildRunArgs_SimpleCommand(t *testing.T) {
	svc := model.Service{
		Name:     "app",
		Hostname: "keel-app",
		Image:    "app:latest",
		Command:  "server",
	}
	args := buildRunArgs(svc, "/opt/keel", "")
	if args[len(args)-1] != "server" {
		t.Errorf("expected 'server' as last arg")
	}
}

func TestBuildRunArgs_Environment(t *testing.T) {
	svc := model.Service{
		Name:        "app",
		Hostname:    "keel-app",
		Image:       "app:latest",
		Environment: map[string]string{"DB_HOST": "localhost"},
	}
	args := buildRunArgs(svc, "/opt/keel", "")
	assertContainsPair(t, args, "-e", "DB_HOST=localhost")
}

// --- resolveVolume tests ---

func TestResolveVolume_NamedVolume(t *testing.T) {
	got := resolveVolume("mydata:/var/lib/data", "/opt/keel")
	if got != "mydata:/var/lib/data" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestResolveVolume_AbsolutePath(t *testing.T) {
	got := resolveVolume("/host/path:/container/path", "/opt/keel")
	if got != "/host/path:/container/path" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestResolveVolume_RelativePath(t *testing.T) {
	got := resolveVolume("./data:/var/lib/data", "/opt/keel")
	if got != "/opt/keel/data:/var/lib/data" {
		t.Errorf("expected /opt/keel/data:/var/lib/data, got %q", got)
	}
}

func TestResolveVolume_RelativeNoDot(t *testing.T) {
	got := resolveVolume("configs/my.cnf:/etc/my.cnf", "/opt/keel")
	if got != "/opt/keel/configs/my.cnf:/etc/my.cnf" {
		t.Errorf("expected /opt/keel/configs/my.cnf:/etc/my.cnf, got %q", got)
	}
}

// --- shellQuote tests ---

func TestShellQuote_Simple(t *testing.T) {
	got := shellQuote("hello")
	if got != "'hello'" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestShellQuote_WithSingleQuotes(t *testing.T) {
	got := shellQuote("it's")
	if !strings.Contains(got, `'\''`) {
		t.Errorf("expected escaped single quote, got %q", got)
	}
}

func TestShellQuote_Empty(t *testing.T) {
	got := shellQuote("")
	if got != "''" {
		t.Errorf("expected empty quotes, got %q", got)
	}
}

// --- shellJoin tests ---

func TestShellJoin_SimpleArgs(t *testing.T) {
	got := shellJoin([]string{"docker", "ps", "-a"})
	if got != "docker ps -a" {
		t.Errorf("expected 'docker ps -a', got %q", got)
	}
}

func TestShellJoin_ArgsWithSpaces(t *testing.T) {
	got := shellJoin([]string{"echo", "hello world"})
	if !strings.Contains(got, "'hello world'") {
		t.Errorf("expected quoted 'hello world', got %q", got)
	}
}

func TestShellJoin_ArgsWithSpecialChars(t *testing.T) {
	got := shellJoin([]string{"docker", "run", "-e", "FOO=$BAR"})
	// FOO=$BAR should be quoted because of $
	if !strings.Contains(got, "'FOO=$BAR'") {
		t.Errorf("expected quoted 'FOO=$BAR', got %q", got)
	}
}

// --- workdirFromDockerfile tests ---

func TestWorkdirFromDockerfile_Found(t *testing.T) {
	lines := []string{
		"FROM node:18",
		"WORKDIR /usr/src/app",
		"COPY . .",
	}
	got := workdirFromDockerfile(lines)
	if got != "/usr/src/app" {
		t.Errorf("expected /usr/src/app, got %q", got)
	}
}

func TestWorkdirFromDockerfile_LastWins(t *testing.T) {
	lines := []string{
		"FROM node:18",
		"WORKDIR /first",
		"WORKDIR /second",
	}
	got := workdirFromDockerfile(lines)
	if got != "/second" {
		t.Errorf("expected /second, got %q", got)
	}
}

func TestWorkdirFromDockerfile_DefaultApp(t *testing.T) {
	lines := []string{"FROM node:18", "COPY . ."}
	got := workdirFromDockerfile(lines)
	if got != "/app" {
		t.Errorf("expected default /app, got %q", got)
	}
}

func TestWorkdirFromDockerfile_CaseInsensitive(t *testing.T) {
	lines := []string{"FROM node:18", "workdir /myapp"}
	got := workdirFromDockerfile(lines)
	if got != "/myapp" {
		t.Errorf("expected /myapp, got %q", got)
	}
}

// --- helpers ---

func assertContains(t *testing.T, args []string, val string) {
	t.Helper()
	for _, a := range args {
		if a == val {
			return
		}
	}
	t.Errorf("expected %q in args: %v", val, args)
}

func assertContainsPair(t *testing.T, args []string, key, val string) {
	t.Helper()
	for i, a := range args {
		if a == key && i+1 < len(args) && args[i+1] == val {
			return
		}
	}
	t.Errorf("expected %s %s in args: %v", key, val, args)
}
