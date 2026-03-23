package tunnel

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/getkaze/keel/internal/config"
	keelssh "github.com/getkaze/keel/internal/ssh"
)

// Status represents the current state of the SSH tunnel.
type Status string

const (
	StatusConnected    Status = "connected"
	StatusReconnecting Status = "reconnecting"
	StatusFailed       Status = "failed"
	StatusDisconnected Status = "disconnected"
)

const (
	healthInterval = 30 * time.Second
	healthTimeout  = 5 * time.Second
	socketTimeout  = 15 * time.Second
	maxRetries     = 10
	maxBackoff     = 30 * time.Second
)

// Monitor manages an SSH tunnel to a remote Docker socket with automatic
// reconnection and health checking.
type Monitor struct {
	target   *config.TargetConfig
	sockPath string

	mu        sync.RWMutex
	status    Status
	listeners []chan Status

	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
}

// NewMonitor creates a tunnel monitor for the given remote target.
func NewMonitor(target *config.TargetConfig) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("keel-docker-%s.sock", target.Name))

	return &Monitor{
		target:   target,
		sockPath: sockPath,
		status:   StatusDisconnected,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start establishes the initial tunnel connection and begins monitoring.
func (m *Monitor) Start() error {
	if err := m.connect(); err != nil {
		m.setStatus(StatusFailed)
		return err
	}
	m.setStatus(StatusConnected)
	os.Setenv("DOCKER_HOST", "unix://"+m.sockPath)
	log.Printf("docker tunnel: %s → %s@%s (socket: %s)", m.target.Name, m.target.SSHUser, m.target.Host, m.sockPath)

	go m.monitor()
	return nil
}

// Stop shuts down the tunnel and stops monitoring.
func (m *Monitor) Stop() {
	m.cancel()
	m.killCmd()
	_ = os.Remove(m.sockPath)
	m.setStatus(StatusDisconnected)
}

// Status returns the current tunnel status.
func (m *Monitor) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Subscribe returns a channel that receives status changes.
func (m *Monitor) Subscribe() <-chan Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan Status, 8)
	m.listeners = append(m.listeners, ch)
	return ch
}

// Unsubscribe removes a previously subscribed channel.
func (m *Monitor) Unsubscribe(ch <-chan Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, l := range m.listeners {
		if l == ch {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			close(l)
			return
		}
	}
}

func (m *Monitor) setStatus(s Status) {
	m.mu.Lock()
	prev := m.status
	m.status = s
	listeners := make([]chan Status, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.Unlock()

	if s != prev {
		log.Printf("docker tunnel: status %s → %s", prev, s)
		for _, ch := range listeners {
			select {
			case ch <- s:
			default:
			}
		}
	}
}

func (m *Monitor) connect() error {
	_ = os.Remove(m.sockPath) // remove stale socket

	baseArgs := keelssh.BuildArgs(m.target)
	sshArgs := []string{"-nNT", "-o", "ExitOnForwardFailure=yes", "-L", m.sockPath + ":/var/run/docker.sock"}
	sshArgs = append(sshArgs, baseArgs...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ssh tunnel start: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()

	// Wait for socket to appear
	deadline := time.Now().Add(socketTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(m.sockPath); err == nil {
			return nil
		}
		select {
		case <-m.ctx.Done():
			m.killCmd()
			return m.ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	m.killCmd()
	return fmt.Errorf("tunnel socket did not appear within %s", socketTimeout)
}

func (m *Monitor) monitor() {
	// Watch for SSH process exit.
	exitCh := make(chan error, 1)
	go func() {
		m.mu.RLock()
		cmd := m.cmd
		m.mu.RUnlock()
		if cmd != nil {
			exitCh <- cmd.Wait()
		}
	}()

	healthTicker := time.NewTicker(healthInterval)
	defer healthTicker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return

		case err := <-exitCh:
			if m.ctx.Err() != nil {
				return // shutting down
			}
			log.Printf("docker tunnel: SSH process exited: %v", err)
			m.reconnect()
			// Restart exit watcher for new process.
			go func() {
				m.mu.RLock()
				cmd := m.cmd
				m.mu.RUnlock()
				if cmd != nil {
					exitCh <- cmd.Wait()
				}
			}()

		case <-healthTicker.C:
			if !m.healthCheck() {
				log.Printf("docker tunnel: health check failed")
				m.killCmd()
				m.reconnect()
				// Restart exit watcher for new process.
				go func() {
					m.mu.RLock()
					cmd := m.cmd
					m.mu.RUnlock()
					if cmd != nil {
						exitCh <- cmd.Wait()
					}
				}()
			}
		}
	}
}

func (m *Monitor) healthCheck() bool {
	ctx, cancel := context.WithTimeout(m.ctx, healthTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	return cmd.Run() == nil
}

func (m *Monitor) reconnect() {
	m.setStatus(StatusReconnecting)
	backoff := time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if m.ctx.Err() != nil {
			return
		}

		log.Printf("docker tunnel: reconnecting (attempt %d/%d, backoff %s)", attempt, maxRetries, backoff)

		select {
		case <-m.ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err := m.connect(); err != nil {
			log.Printf("docker tunnel: reconnect failed: %v", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		m.setStatus(StatusConnected)
		log.Printf("docker tunnel: reconnected")
		return
	}

	m.setStatus(StatusFailed)
	log.Printf("docker tunnel: failed after %d attempts", maxRetries)
}

func (m *Monitor) killCmd() {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	_ = os.Remove(m.sockPath)
}
