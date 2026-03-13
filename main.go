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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"runtime"

	"github.com/getkaze/keel/internal/cli"
	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/server"
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
	if target != nil && target.Mode == "remote" {
		cleanup := startDockerTunnel(target)
		defer cleanup()
	}

	srv := server.New(server.Config{
		Port:     port,
		Bind:     bind,
		Dev:      dev,
		KeelDir:  keelDir,
		StaticFS: embeddedFS,
		Version:  version,
		Ctx:      ctx,
		Target:   target,
	})

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

// startDockerTunnel opens an SSH tunnel that forwards the remote Docker socket
// to a local Unix socket. It sets DOCKER_HOST so all docker commands use it.
// Returns a cleanup function that kills the tunnel and removes the socket.
func startDockerTunnel(target *config.TargetConfig) func() {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("keel-docker-%s.sock", target.Name))
	_ = os.Remove(sockPath) // remove stale socket

	sshArgs := []string{
		"-nNT",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "LogLevel=ERROR",
		"-L", sockPath + ":/var/run/docker.sock",
	}
	if target.SSHKey != "" {
		keyPath := target.SSHKey
		if strings.HasPrefix(keyPath, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				keyPath = filepath.Join(home, keyPath[2:])
			}
		}
		sshArgs = append(sshArgs, "-i", keyPath)
	}
	if target.SSHJump != "" {
		keyPath := target.SSHKey
		if strings.HasPrefix(keyPath, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				keyPath = filepath.Join(home, keyPath[2:])
			}
		}
		proxyCmd := "ssh -o StrictHostKeyChecking=no -o BatchMode=yes -o LogLevel=ERROR"
		if keyPath != "" {
			proxyCmd += " -i " + keyPath
		}
		proxyCmd += " -W %h:%p " + target.SSHJump
		sshArgs = append(sshArgs, "-o", "ProxyCommand="+proxyCmd)
	}
	sshArgs = append(sshArgs, target.SSHUser+"@"+target.Host)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("docker tunnel: failed to start SSH: %v", err)
	}

	// Wait for the socket to appear (up to 5s).
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	os.Setenv("DOCKER_HOST", "unix://"+sockPath)
	log.Printf("docker tunnel: %s → %s@%s (socket: %s)", target.Name, target.SSHUser, target.Host, sockPath)

	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait() // Reap the process to avoid zombies
		}
		_ = os.Remove(sockPath)
	}
}

func openBrowser(url string) {
	var cmds []string
	if runtime.GOOS == "darwin" {
		cmds = []string{"open"}
	} else {
		cmds = []string{"xdg-open", "sensible-browser", "x-www-browser", "firefox", "chromium", "google-chrome"}
	}
	for _, cmd := range cmds {
		if path, err := exec.LookPath(cmd); err == nil {
			if p, err := os.StartProcess(path, []string{path, url}, &os.ProcAttr{
				Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			}); err == nil {
				_ = p.Release()
				return
			}
		}
	}
	log.Printf("Could not open browser: install xdg-utils or open %s manually", url)
}
