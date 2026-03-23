package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- State file tests ---

func TestSeederState_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0755)

	se := &SeederExecutor{
		KeelDir:    dir,
		Cmd:        localCmdBuilder{},
		lastStatus: make(map[string]SeederStateEntry),
	}

	se.lastStatus["test-seeder"] = SeederStateEntry{
		Status: "success",
		RanAt:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	se.saveState()

	// Verify file exists
	data, err := os.ReadFile(se.stateFile())
	if err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	var state map[string]SeederStateEntry
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("invalid JSON in state file: %v", err)
	}
	if state["test-seeder"].Status != "success" {
		t.Errorf("expected status 'success', got %q", state["test-seeder"].Status)
	}

	// Re-load and verify
	se2 := &SeederExecutor{
		KeelDir:    dir,
		Cmd:        localCmdBuilder{},
		lastStatus: make(map[string]SeederStateEntry),
	}
	se2.loadState()
	if se2.lastStatus["test-seeder"].Status != "success" {
		t.Errorf("expected loaded status 'success', got %q", se2.lastStatus["test-seeder"].Status)
	}
}

func TestSeederState_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	se := &SeederExecutor{
		KeelDir:    dir,
		Cmd:        localCmdBuilder{},
		lastStatus: make(map[string]SeederStateEntry),
	}
	se.loadState() // should not error
	if len(se.lastStatus) != 0 {
		t.Errorf("expected empty status map for missing file, got %d entries", len(se.lastStatus))
	}
}

func TestSeederState_LoadCorrupted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0755)
	os.WriteFile(filepath.Join(dir, "data", "seeder-state.json"), []byte("{invalid"), 0644)

	se := &SeederExecutor{
		KeelDir:    dir,
		Cmd:        localCmdBuilder{},
		lastStatus: make(map[string]SeederStateEntry),
	}
	se.loadState()
	if len(se.lastStatus) != 0 {
		t.Errorf("expected empty status for corrupted file, got %d entries", len(se.lastStatus))
	}
}

func TestGetLastStatus(t *testing.T) {
	se := &SeederExecutor{
		lastStatus: map[string]SeederStateEntry{
			"seed-a": {Status: "success"},
			"seed-b": {Status: "error"},
		},
	}

	if got := se.GetLastStatus("seed-a"); got != "success" {
		t.Errorf("expected 'success', got %q", got)
	}
	if got := se.GetLastStatus("seed-b"); got != "error" {
		t.Errorf("expected 'error', got %q", got)
	}
	if got := se.GetLastStatus("unknown"); got != "" {
		t.Errorf("expected empty for unknown seeder, got %q", got)
	}
}

func TestGetLastRanAt(t *testing.T) {
	now := time.Now()
	se := &SeederExecutor{
		lastStatus: map[string]SeederStateEntry{
			"seed-a": {Status: "success", RanAt: now},
		},
	}

	got := se.GetLastRanAt("seed-a")
	if got.IsZero() {
		t.Fatal("expected non-zero time")
	}
	if got.Unix() != now.Unix() {
		t.Errorf("expected %v, got %v", now, got)
	}

	if got := se.GetLastRanAt("unknown"); !got.IsZero() {
		t.Errorf("expected zero time for unknown seeder, got %v", got)
	}
}

func TestClearAll(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0755)

	se := &SeederExecutor{
		KeelDir: dir,
		Cmd:     localCmdBuilder{},
		lastStatus: map[string]SeederStateEntry{
			"seed-a": {Status: "success"},
			"seed-b": {Status: "error"},
		},
	}
	se.ClearAll()

	if len(se.lastStatus) != 0 {
		t.Errorf("expected empty after ClearAll, got %d entries", len(se.lastStatus))
	}

	// State file should exist and be empty map
	data, err := os.ReadFile(se.stateFile())
	if err != nil {
		t.Fatalf("state file not found: %v", err)
	}
	var state map[string]SeederStateEntry
	json.Unmarshal(data, &state)
	if len(state) != 0 {
		t.Errorf("state file should be empty map, got %d entries", len(state))
	}
}

// --- formatDuration tests ---

func TestFormatDuration_Milliseconds(t *testing.T) {
	got := formatDuration(150 * time.Millisecond)
	if got != "150ms" {
		t.Errorf("expected '150ms', got %q", got)
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	got := formatDuration(2500 * time.Millisecond)
	if got != "2.5s" {
		t.Errorf("expected '2.5s', got %q", got)
	}
}

func TestFormatDuration_ZeroMs(t *testing.T) {
	got := formatDuration(0)
	if got != "0ms" {
		t.Errorf("expected '0ms', got %q", got)
	}
}

func TestStateFile_Path(t *testing.T) {
	se := &SeederExecutor{KeelDir: "/opt/keel"}
	expected := "/opt/keel/data/seeder-state.json"
	if se.stateFile() != expected {
		t.Errorf("expected %q, got %q", expected, se.stateFile())
	}
}
