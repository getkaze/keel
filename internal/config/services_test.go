package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkaze/keel/internal/model"
)

func makeTestStore(t *testing.T) (*ServiceStore, string) {
	t.Helper()
	dir := t.TempDir()
	servicesDir := filepath.Join(dir, "data", "services")
	if err := os.MkdirAll(servicesDir, 0755); err != nil {
		t.Fatal(err)
	}
	store := &ServiceStore{
		servicesDir: servicesDir,
		configPath:  filepath.Join(dir, "data", "config.json"),
	}
	return store, dir
}

func writeService(t *testing.T, dir string, svc model.Service) {
	t.Helper()
	data, _ := json.MarshalIndent(svc, "", "  ")
	path := filepath.Join(dir, "data", "services", svc.Name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestServiceStore_List_Empty(t *testing.T) {
	store, _ := makeTestStore(t)
	svcs, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svcs) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcs))
	}
}

func TestServiceStore_List_MissingDir(t *testing.T) {
	store := &ServiceStore{servicesDir: "/tmp/keel-nonexistent-services-dir-xyz"}
	svcs, err := store.List()
	if err != nil {
		t.Fatalf("expected no error for missing dir, got: %v", err)
	}
	if svcs != nil {
		t.Errorf("expected nil slice for missing dir, got %v", svcs)
	}
}

func TestServiceStore_Get_Found(t *testing.T) {
	store, dir := makeTestStore(t)
	writeService(t, dir, model.Service{
		Name:     "mysql",
		Hostname: "keel-mysql",
		Image:    "mysql:8.4",
		Network:  "keel-net",
	})

	svc, err := store.Get("mysql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.Name != "mysql" {
		t.Errorf("expected name 'mysql', got %q", svc.Name)
	}
}

func TestServiceStore_Get_NotFound(t *testing.T) {
	store, _ := makeTestStore(t)
	svc, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc != nil {
		t.Errorf("expected nil, got %+v", svc)
	}
}

func TestServiceStore_Save_And_Get(t *testing.T) {
	store, _ := makeTestStore(t)
	svc := model.Service{
		Name:    "redis",
		Image:   "redis:7",
		Network: "keel-net",
	}

	if err := store.Save(svc); err != nil {
		t.Fatalf("save error: %v", err)
	}

	got, err := store.Get("redis")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got == nil || got.Name != "redis" {
		t.Errorf("expected saved service, got %+v", got)
	}
	// Hostname auto-filled
	if got.Hostname != "keel-redis" {
		t.Errorf("expected hostname 'keel-redis', got %q", got.Hostname)
	}
}

func TestServiceStore_Save_RequiredFields(t *testing.T) {
	store, _ := makeTestStore(t)

	if err := store.Save(model.Service{Name: "noimage"}); err == nil {
		t.Fatal("expected error for missing image")
	}
	if err := store.Save(model.Service{Image: "redis:7"}); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestServiceStore_Delete(t *testing.T) {
	store, dir := makeTestStore(t)
	writeService(t, dir, model.Service{Name: "todelete", Image: "nginx", Network: "net"})

	if err := store.Delete("todelete"); err != nil {
		t.Fatalf("delete error: %v", err)
	}
	svc, _ := store.Get("todelete")
	if svc != nil {
		t.Error("expected nil after delete")
	}
}

func TestServiceStore_Delete_NotFound(t *testing.T) {
	store, _ := makeTestStore(t)
	// Should not error on missing file
	if err := store.Delete("nonexistent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceStore_List_Multiple(t *testing.T) {
	store, dir := makeTestStore(t)
	writeService(t, dir, model.Service{Name: "svc1", Image: "img1", Network: "net"})
	writeService(t, dir, model.Service{Name: "svc2", Image: "img2", Network: "net"})

	svcs, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svcs) != 2 {
		t.Errorf("expected 2 services, got %d", len(svcs))
	}
}

func TestServiceStore_SaveRaw_Valid(t *testing.T) {
	store, _ := makeTestStore(t)
	raw := `{"name":"nginx","hostname":"keel-nginx","image":"nginx:latest","network":"keel-net","ports":{"internal":80,"external":80}}`

	if err := store.SaveRaw("nginx", []byte(raw)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc, _ := store.Get("nginx")
	if svc == nil || svc.Image != "nginx:latest" {
		t.Errorf("expected saved service, got %+v", svc)
	}
}

func TestServiceStore_SaveRaw_NameMismatch(t *testing.T) {
	store, _ := makeTestStore(t)
	raw := `{"name":"other","image":"nginx:latest","network":"keel-net"}`
	if err := store.SaveRaw("nginx", []byte(raw)); err == nil {
		t.Fatal("expected error for name mismatch")
	}
}

func TestServiceStore_GlobalConfig_Missing(t *testing.T) {
	store := &ServiceStore{configPath: "/tmp/keel-nonexistent-config.json"}
	cfg, err := store.GlobalConfig()
	if err != nil {
		t.Fatalf("expected no error for missing config, got: %v", err)
	}
	if cfg.Network != "keel-net" {
		t.Errorf("expected default network, got %q", cfg.Network)
	}
}
