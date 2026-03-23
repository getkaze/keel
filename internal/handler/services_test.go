package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/model"
)

func makeServiceTestDeps(t *testing.T) (*ServiceDeps, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data", "services"), 0755)
	store := config.NewServiceStore(dir)
	return &ServiceDeps{
		Services: store,
	}, dir
}

func TestSaveServiceConfig_EmptyBody(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	req := httptest.NewRequest("PUT", "/api/services/test/config", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	deps.saveServiceConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSaveServiceConfig_InvalidJSON(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	req := httptest.NewRequest("PUT", "/api/services/test/config", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	deps.saveServiceConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestSaveServiceConfig_ValidJSON(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	body := `{"name":"test","hostname":"keel-test","image":"nginx:latest","network":"keel-net","ports":{"internal":80,"external":80}}`
	req := httptest.NewRequest("PUT", "/api/services/test/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	deps.saveServiceConfig(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSaveServiceConfig_NameMismatch(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	body := `{"name":"other","hostname":"keel-other","image":"nginx:latest","network":"keel-net"}`
	req := httptest.NewRequest("PUT", "/api/services/test/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	deps.saveServiceConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for name mismatch, got %d", w.Code)
	}
}

func TestSaveServiceConfig_FormData(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	body := `config={"name":"test","hostname":"keel-test","image":"nginx:latest","network":"keel-net","ports":{"internal":80,"external":80}}`
	req := httptest.NewRequest("PUT", "/api/services/test/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	deps.saveServiceConfig(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSaveServiceConfig_BodySizeLimit(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	// Create a body larger than 1 MB
	bigBody := strings.Repeat("x", 2<<20)
	req := httptest.NewRequest("PUT", "/api/services/test/config", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	deps.saveServiceConfig(w, req)
	if w.Code == http.StatusOK {
		t.Error("expected rejection for oversized body")
	}
}

func TestValidateServiceName_Unknown(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	err := deps.validateServiceName("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
	if !strings.Contains(err.Error(), "unknown service") {
		t.Errorf("expected 'unknown service' in error, got %q", err.Error())
	}
}

func TestValidateServiceName_Found(t *testing.T) {
	deps, _ := makeServiceTestDeps(t)
	deps.Services.Save(model.Service{Name: "mysql", Image: "mysql:8", Network: "keel-net"})
	err := deps.validateServiceName("mysql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
