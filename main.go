package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"runtime"

	"github.com/getkaze/keel/internal/cli"
	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/handler"
	"github.com/getkaze/keel/internal/metrics"
	"github.com/getkaze/keel/internal/server"
	"github.com/getkaze/keel/internal/tunnel"
)

var version = "dev"

// cliKeelDir returns the data directory for CLI subcommands.
func cliKeelDir() string {
	return config.DefaultDataDir()
}

func main() {
	// Route CLI subcommands before flag parsing.
	// A subcommand is any first argument that does not start with '-'.
	// Special case: -h/--help should show the full usage, not Go's flag defaults.
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "-h" || arg == "--help" {
			cli.Run([]string{"help"}, cliKeelDir(), version)
			return
		}
		if !strings.HasPrefix(arg, "-") {
			cli.Run(os.Args[1:], cliKeelDir(), version)
			return
		}
	}

	var (
		port        int
		bind        string
		keelDir string
		dev bool
	)

	flag.IntVar(&port, "port", 60000, "HTTP server port")
	flag.StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	flag.StringVar(&keelDir, "keel-dir", config.DefaultDataDir(), "keel data directory")
	flag.BoolVar(&dev, "dev", false, "Development mode (serve assets from filesystem)")
	flag.Parse()

	// Application-wide context — cancelled on SIGINT/SIGTERM.
	// All exec commands, SSE streams, and WebSocket sessions derive from this.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If active target is remote, set up an SSH tunnel to the remote Docker
	// socket so that all `docker` commands from the dashboard executor
	// transparently reach the remote host.
	target, _ := config.ReadTargetConfig(keelDir)
	var tunnelMon *tunnel.Monitor
	if target != nil && target.Mode == "remote" {
		tunnelMon = tunnel.NewMonitor(target)
		if err := tunnelMon.Start(); err != nil {
			log.Fatalf("docker tunnel: %v", err)
		}
		defer tunnelMon.Stop()
	}

	// Create the appropriate CmdRunner based on the active target.
	var inner docker.CmdRunner
	if target != nil && target.Mode == "remote" {
		inner = cli.NewRunner(target, keelDir)
	} else {
		inner = docker.NewLocalRunner()
	}
	runner := docker.NewReloadableRunner(inner)

	remoteRef := &handler.RemoteRef{}
	if target != nil && target.Mode == "remote" {
		remoteRef.Store(metrics.NewRemoteCollector(target))
	}

	srv := server.New(server.Config{
		Port:      port,
		Bind:      bind,
		Dev:       dev,
		KeelDir:   keelDir,
		StaticFS:  embeddedFS,
		Version:   version,
		Ctx:       ctx,
		Target:    target,
		Runner:    runner,
		Tunnel:    tunnelMon,
		RemoteRef: remoteRef,
	})

	// Watch target config for changes and hot-reload the runner.
	go watchTargetConfig(ctx, keelDir, target, runner, &tunnelMon, remoteRef)

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received %s, shutting down...", sig)

		// 1. Cancel application context — kills exec processes, SSE streams, WS sessions
		cancel()

		// 2. Graceful HTTP shutdown with 5s deadline
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Forced shutdown: %v", err)
		}
	}()

	go openBrowser(fmt.Sprintf("http://%s:%d", bind, port))

	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}

// watchTargetConfig polls the target config files every 5 seconds and swaps
// the Runner when the active target changes.
func watchTargetConfig(ctx context.Context, keelDir string, current *config.TargetConfig, runner *docker.ReloadableRunner, tunnelMon **tunnel.Monitor, remoteRef *handler.RemoteRef) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	configHash := targetHash(current)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			newTarget, _ := config.ReadTargetConfig(keelDir)
			h := targetHash(newTarget)
			if h == configHash {
				continue
			}
			configHash = h
			log.Printf("target config changed, reloading (target=%s mode=%s)", newTarget.Name, newTarget.Mode)

			// Swap Runner.
			if newTarget.Mode == "remote" {
				runner.Swap(cli.NewRunner(newTarget, keelDir))
				remoteRef.Store(metrics.NewRemoteCollector(newTarget))
			} else {
				runner.Swap(docker.NewLocalRunner())
				remoteRef.Store(nil)
			}

			// Handle tunnel lifecycle.
			mon := *tunnelMon
			if newTarget.Mode == "remote" && mon == nil {
				newMon := tunnel.NewMonitor(newTarget)
				if err := newMon.Start(); err != nil {
					log.Printf("tunnel reload error: %v", err)
				} else {
					*tunnelMon = newMon
				}
			} else if newTarget.Mode != "remote" && mon != nil {
				mon.Stop()
				*tunnelMon = nil
			}
		}
	}
}

// targetHash returns a simple string key for detecting config changes.
func targetHash(t *config.TargetConfig) string {
	if t == nil {
		return "local"
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s", t.Name, t.Mode, t.Host, t.SSHUser, t.SSHKey, t.SSHJump)
}

func openBrowser(url string) {
	var cmds []string
	if runtime.GOOS == "darwin" {
		cmds = []string{"open"}
	} else {
		cmds = []string{"xdg-open", "sensible-browser", "x-www-browser", "firefox", "chromium", "google-chrome"}
	}
	devNull, _ := os.Open(os.DevNull)
	defer devNull.Close()
	for _, cmd := range cmds {
		if path, err := exec.LookPath(cmd); err == nil {
			if p, err := os.StartProcess(path, []string{path, url}, &os.ProcAttr{
				Files: []*os.File{devNull, devNull, devNull},
			}); err == nil {
				_ = p.Release()
				return
			}
		}
	}
	log.Printf("Could not open browser: install xdg-utils or open %s manually", url)
}
