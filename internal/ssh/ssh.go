package ssh

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/getkaze/keel/internal/config"
)

// ExpandHome expands a leading ~/ to the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// BuildArgs returns SSH connection flags for the given target, including
// key, jump-host proxy, and user@host. The caller appends the remote
// command or additional flags (e.g. -nNT for tunnels).
func BuildArgs(t *config.TargetConfig) []string {
	args := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"-o", "IdentityAgent=none",
		"-o", "LogLevel=ERROR",
	}
	if t.SSHKey != "" {
		args = append(args, "-i", ExpandHome(t.SSHKey))
	}
	if t.SSHJump != "" {
		proxyCmd := "ssh -o StrictHostKeyChecking=accept-new -o BatchMode=yes -o LogLevel=ERROR"
		if t.SSHKey != "" {
			proxyCmd += " -i " + ExpandHome(t.SSHKey)
		}
		proxyCmd += " -W %h:%p " + t.SSHJump
		args = append(args, "-o", "ProxyCommand="+proxyCmd)
	}
	args = append(args, t.SSHUser+"@"+t.Host)
	return args
}
