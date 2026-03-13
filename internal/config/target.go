package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TargetInfo holds the current target display information (used by HTTP handlers).
type TargetInfo struct {
	Name string `json:"name"`
	Mode string `json:"mode"` // "local" or "remote"
	Host string `json:"host"`
	User string `json:"user,omitempty"`
}

// TargetConfig is the complete target definition, used by the CLI runner.
type TargetConfig struct {
	Name       string
	Mode       string // "local" or "remote"
	Host       string
	ExternalIP string
	SSHUser    string
	SSHKey     string
	SSHJump    string
	PortBind   string
}

type targetsFile struct {
	Targets map[string]targetDef `json:"targets"`
	Default string               `json:"default"`
}

type targetDef struct {
	Host       string  `json:"host"`
	ExternalIP string  `json:"external_ip"`
	SSHUser    *string `json:"ssh_user"`
	SSHKey     string  `json:"ssh_key"`
	SSHJump    string  `json:"ssh_jump"`
	PortBind   string  `json:"port_bind"`
	Desc       string  `json:"description"`
}

// stateFilePath returns the path to the active target state file.
func stateFilePath(keelDir string) string {
	return filepath.Join(keelDir, "state", "target")
}

// ReadTarget reads the current target state and definition.
func ReadTarget(keelDir string) (*TargetInfo, error) {
	name := activeTargetName(keelDir)

	targetsPath := keelDir + "/data/targets.json"
	data, err := os.ReadFile(targetsPath)
	if err != nil {
		return &TargetInfo{Name: name, Mode: "local", Host: "localhost"}, nil
	}

	var tf targetsFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse targets.json: %w", err)
	}

	def, ok := tf.Targets[name]
	if !ok {
		return &TargetInfo{Name: name, Mode: "local", Host: "localhost"}, nil
	}

	info := &TargetInfo{
		Name: name,
		Host: def.Host,
	}

	if def.SSHUser != nil && *def.SSHUser != "" {
		info.Mode = "remote"
		info.User = *def.SSHUser
	} else {
		info.Mode = "local"
	}

	return info, nil
}

// ReadTargetConfig reads the active target and returns its full configuration for the CLI runner.
func ReadTargetConfig(keelDir string) (*TargetConfig, error) {
	name := activeTargetName(keelDir)

	targetsPath := keelDir + "/data/targets.json"
	data, err := os.ReadFile(targetsPath)
	if err != nil {
		return &TargetConfig{Name: name, Mode: "local", Host: "localhost", PortBind: "127.0.0.1"}, nil
	}

	var tf targetsFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse targets.json: %w", err)
	}

	def, ok := tf.Targets[name]
	if !ok {
		return nil, fmt.Errorf("target %q not found in targets.json", name)
	}

	cfg := &TargetConfig{
		Name:       name,
		Host:       def.Host,
		ExternalIP: def.ExternalIP,
		SSHKey:     def.SSHKey,
		SSHJump:    def.SSHJump,
		PortBind:   def.PortBind,
	}
	if cfg.PortBind == "" {
		cfg.PortBind = "127.0.0.1"
	}

	if def.SSHUser != nil && *def.SSHUser != "" {
		cfg.Mode = "remote"
		cfg.SSHUser = *def.SSHUser
	} else {
		cfg.Mode = "local"
	}

	return cfg, nil
}

// ListTargets returns all available target names sorted alphabetically.
func ListTargets(keelDir string) ([]string, error) {
	targetsPath := keelDir + "/data/targets.json"
	data, err := os.ReadFile(targetsPath)
	if err != nil {
		return []string{"local"}, nil
	}

	var tf targetsFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse targets.json: %w", err)
	}

	names := make([]string, 0, len(tf.Targets))
	for name := range tf.Targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// SetTarget writes the target name to the state file.
func SetTarget(keelDir, name string) error {
	return os.WriteFile(stateFilePath(keelDir), []byte(name+"\n"), 0644)
}

// activeTargetName reads the current target name from the state file.
func activeTargetName(keelDir string) string {
	if data, err := os.ReadFile(stateFilePath(keelDir)); err == nil {
		if n := strings.TrimSpace(string(data)); n != "" {
			return n
		}
	}
	return "local"
}
