package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkaze/keel/internal/model"
)

func makeSeederTestStore(t *testing.T) (*SeederStore, string) {
	t.Helper()
	dir := t.TempDir()
	seedersDir := filepath.Join(dir, "data", "seeders")
	os.MkdirAll(seedersDir, 0755)
	return NewSeederStore(dir), dir
}

func writeSeeder(t *testing.T, dir string, sd model.Seeder) {
	t.Helper()
	data, _ := json.MarshalIndent(sd, "", "  ")
	path := filepath.Join(dir, "data", "seeders", sd.Name+".json")
	os.WriteFile(path, data, 0644)
}

func TestSeederStore_List_Empty(t *testing.T) {
	store, _ := makeSeederTestStore(t)
	seeders, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeders) != 0 {
		t.Errorf("expected 0 seeders, got %d", len(seeders))
	}
}

func TestSeederStore_List_MissingDir(t *testing.T) {
	store := &SeederStore{seedersDir: "/tmp/keel-nonexistent-seeders-dir-xyz"}
	seeders, err := store.List()
	if err != nil {
		t.Fatalf("expected no error for missing dir, got: %v", err)
	}
	if seeders != nil {
		t.Errorf("expected nil for missing dir, got %v", seeders)
	}
}

func TestSeederStore_Get_Found(t *testing.T) {
	store, dir := makeSeederTestStore(t)
	writeSeeder(t, dir, model.Seeder{
		Name:   "seed-mysql",
		Target: "mysql",
		Order:  1,
	})

	sd, err := store.Get("seed-mysql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sd == nil {
		t.Fatal("expected seeder, got nil")
	}
	if sd.Name != "seed-mysql" {
		t.Errorf("expected name 'seed-mysql', got %q", sd.Name)
	}
	if sd.Target != "mysql" {
		t.Errorf("expected target 'mysql', got %q", sd.Target)
	}
}

func TestSeederStore_Get_NotFound(t *testing.T) {
	store, _ := makeSeederTestStore(t)
	sd, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sd != nil {
		t.Errorf("expected nil, got %+v", sd)
	}
}

func TestSeederStore_List_IgnoresNonJSON(t *testing.T) {
	store, dir := makeSeederTestStore(t)
	writeSeeder(t, dir, model.Seeder{Name: "seed-a", Target: "mysql", Order: 1})
	// Write a script file (non-JSON)
	os.WriteFile(filepath.Join(dir, "data", "seeders", "init.sql"), []byte("CREATE TABLE..."), 0644)

	seeders, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeders) != 1 {
		t.Errorf("expected 1 seeder (ignoring .sql), got %d", len(seeders))
	}
}

func TestSeederStore_List_SortedByOrder(t *testing.T) {
	store, dir := makeSeederTestStore(t)
	writeSeeder(t, dir, model.Seeder{Name: "seed-b", Target: "mysql", Order: 2})
	writeSeeder(t, dir, model.Seeder{Name: "seed-a", Target: "mysql", Order: 1})

	seeders, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeders) != 2 {
		t.Fatalf("expected 2 seeders, got %d", len(seeders))
	}
	if seeders[0].Name != "seed-a" {
		t.Errorf("expected first seeder 'seed-a' (order 1), got %q", seeders[0].Name)
	}
}

func TestSeederStore_GetRaw(t *testing.T) {
	store, dir := makeSeederTestStore(t)
	writeSeeder(t, dir, model.Seeder{Name: "seed-raw", Target: "mysql"})

	raw, err := store.GetRaw("seed-raw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty raw data")
	}
	// Should be valid JSON
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Errorf("raw data is not valid JSON: %v", err)
	}
}

func TestSeederStore_GetRaw_NotFound(t *testing.T) {
	store, _ := makeSeederTestStore(t)
	_, err := store.GetRaw("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing seeder")
	}
}

func TestSeederStore_Dir(t *testing.T) {
	store := NewSeederStore("/opt/keel")
	expected := "/opt/keel/data/seeders"
	if store.Dir() != expected {
		t.Errorf("expected %q, got %q", expected, store.Dir())
	}
}
