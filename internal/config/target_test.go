package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func makeTargetTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0755)
	os.MkdirAll(filepath.Join(dir, "state"), 0755)
	return dir
}

func writeTargetsFile(t *testing.T, dir string, content string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, "data", "targets.json"), []byte(content), 0644)
}

func TestReadTarget_MissingFile(t *testing.T) {
	dir := makeTargetTestDir(t)
	info, err := ReadTarget(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Mode != "local" || info.Host != "localhost" {
		t.Errorf("expected local/localhost default, got mode=%q host=%q", info.Mode, info.Host)
	}
}

func TestReadTarget_LocalTarget(t *testing.T) {
	dir := makeTargetTestDir(t)
	writeTargetsFile(t, dir, `{
		"targets": {
			"local": {"host": "localhost"}
		},
		"default": "local"
	}`)

	info, err := ReadTarget(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Mode != "local" {
		t.Errorf("expected mode 'local', got %q", info.Mode)
	}
	if info.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", info.Host)
	}
}

func TestReadTarget_RemoteTarget(t *testing.T) {
	dir := makeTargetTestDir(t)
	user := "deploy"
	writeTargetsFile(t, dir, `{
		"targets": {
			"prod": {"host": "10.0.0.1", "ssh_user": "`+user+`", "ssh_key": "~/.ssh/id_ed25519"}
		},
		"default": "prod"
	}`)
	os.WriteFile(filepath.Join(dir, "state", "target"), []byte("prod\n"), 0644)

	info, err := ReadTarget(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Mode != "remote" {
		t.Errorf("expected mode 'remote', got %q", info.Mode)
	}
	if info.User != "deploy" {
		t.Errorf("expected user 'deploy', got %q", info.User)
	}
}

func TestReadTargetConfig_DefaultPortBind(t *testing.T) {
	dir := makeTargetTestDir(t)
	writeTargetsFile(t, dir, `{
		"targets": {
			"local": {"host": "localhost"}
		}
	}`)
	os.WriteFile(filepath.Join(dir, "state", "target"), []byte("local\n"), 0644)

	cfg, err := ReadTargetConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PortBind != "127.0.0.1" {
		t.Errorf("expected default port bind '127.0.0.1', got %q", cfg.PortBind)
	}
}

func TestReadTargetConfig_CustomPortBind(t *testing.T) {
	dir := makeTargetTestDir(t)
	writeTargetsFile(t, dir, `{
		"targets": {
			"staging": {"host": "10.0.0.2", "port_bind": "0.0.0.0", "ssh_user": "user"}
		}
	}`)
	os.WriteFile(filepath.Join(dir, "state", "target"), []byte("staging\n"), 0644)

	cfg, err := ReadTargetConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PortBind != "0.0.0.0" {
		t.Errorf("expected port bind '0.0.0.0', got %q", cfg.PortBind)
	}
}

func TestReadTargetConfig_NotFound(t *testing.T) {
	dir := makeTargetTestDir(t)
	writeTargetsFile(t, dir, `{"targets": {"prod": {"host": "10.0.0.1"}}}`)
	os.WriteFile(filepath.Join(dir, "state", "target"), []byte("staging\n"), 0644)

	_, err := ReadTargetConfig(dir)
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestListTargets_MissingFile(t *testing.T) {
	dir := makeTargetTestDir(t)
	names, err := ListTargets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 1 || names[0] != "local" {
		t.Errorf("expected [local], got %v", names)
	}
}

func TestListTargets_Multiple(t *testing.T) {
	dir := makeTargetTestDir(t)
	writeTargetsFile(t, dir, `{
		"targets": {
			"prod": {"host": "10.0.0.1"},
			"staging": {"host": "10.0.0.2"},
			"local": {"host": "localhost"}
		}
	}`)
	names, err := ListTargets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("expected 3 targets, got %d", len(names))
	}
	// Should be sorted
	if names[0] != "local" || names[1] != "prod" || names[2] != "staging" {
		t.Errorf("expected sorted [local prod staging], got %v", names)
	}
}

func TestSetTarget_ReadBack(t *testing.T) {
	dir := makeTargetTestDir(t)
	if err := SetTarget(dir, "prod"); err != nil {
		t.Fatalf("set target error: %v", err)
	}
	got := activeTargetName(dir)
	if got != "prod" {
		t.Errorf("expected 'prod', got %q", got)
	}
}

func TestActiveTargetName_Default(t *testing.T) {
	dir := makeTargetTestDir(t)
	got := activeTargetName(dir)
	if got != "local" {
		t.Errorf("expected default 'local', got %q", got)
	}
}

func TestReadTargetConfig_InvalidJSON(t *testing.T) {
	dir := makeTargetTestDir(t)
	os.WriteFile(filepath.Join(dir, "data", "targets.json"), []byte("{invalid"), 0644)
	_, err := ReadTargetConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadTargetConfig_SSHJump(t *testing.T) {
	dir := makeTargetTestDir(t)
	user := "deploy"
	data := map[string]interface{}{
		"targets": map[string]interface{}{
			"prod": map[string]interface{}{
				"host":     "10.0.0.1",
				"ssh_user": user,
				"ssh_key":  "~/.ssh/key",
				"ssh_jump": "bastion.example.com",
			},
		},
	}
	raw, _ := json.Marshal(data)
	os.WriteFile(filepath.Join(dir, "data", "targets.json"), raw, 0644)
	os.WriteFile(filepath.Join(dir, "state", "target"), []byte("prod\n"), 0644)

	cfg, err := ReadTargetConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SSHJump != "bastion.example.com" {
		t.Errorf("expected ssh_jump 'bastion.example.com', got %q", cfg.SSHJump)
	}
	if cfg.SSHKey != "~/.ssh/key" {
		t.Errorf("expected ssh_key '~/.ssh/key', got %q", cfg.SSHKey)
	}
}
