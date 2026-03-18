package terminal

import (
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Session holds a PTY-attached bash process.
type Session struct {
	PTY       *os.File
	Cmd       *exec.Cmd
	closeOnce sync.Once
}

// NewSession spawns /bin/bash with a PTY.
func NewSession() (*Session, error) {
	cmd := exec.Command("/bin/bash")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	return &Session{PTY: ptmx, Cmd: cmd}, nil
}

// NewExecSession spawns docker exec -it <container> <shell> with a PTY.
// It tries /bin/bash first, falling back to /bin/sh for minimal images (e.g. Alpine/Redis).
func NewExecSession(container string) (*Session, error) {
	shell := detectShell(container)
	cmd := exec.Command("docker", "exec", "-it", "-e", "TERM=xterm-256color", container, shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	return &Session{PTY: ptmx, Cmd: cmd}, nil
}

// detectShell checks if /bin/bash exists inside the container, otherwise falls back to /bin/sh.
func detectShell(container string) string {
	check := exec.Command("docker", "exec", container, "test", "-x", "/bin/bash")
	if err := check.Run(); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

// Resize changes the terminal dimensions.
func (s *Session) Resize(rows, cols uint16) error {
	return pty.Setsize(s.PTY, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// Close terminates the session and cleans up. Safe to call multiple times.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		if s.PTY != nil {
			s.PTY.Close()
		}
		if s.Cmd != nil && s.Cmd.Process != nil {
			s.Cmd.Process.Kill()
			s.Cmd.Wait()
		}
	})
}
