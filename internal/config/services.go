package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/getkaze/keel/internal/model"
)

const defaultNetwork = "keel-net"

// ServiceStore provides thread-safe CRUD access to per-service JSON files
// stored in {keelDir}/data/services/.
type ServiceStore struct {
	mu          sync.RWMutex
	servicesDir string
	configPath  string
}

// NewServiceStore creates a store backed by {keelDir}/data/services/.
func NewServiceStore(keelDir string) *ServiceStore {
	return &ServiceStore{
		servicesDir: filepath.Join(keelDir, "data", "services"),
		configPath:  filepath.Join(keelDir, "data", "config.json"),
	}
}

// List returns all service definitions from the services directory.
// Returns an empty slice if the directory does not exist.
func (s *ServiceStore) List() ([]model.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.servicesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read services dir: %w", err)
	}

	var services []model.Service
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		svc, err := s.readFile(filepath.Join(s.servicesDir, e.Name()))
		if err != nil {
			continue // skip malformed files
		}
		services = append(services, *svc)
	}
	sort.SliceStable(services, func(i, j int) bool {
		oi, oj := services[i].StartOrder, services[j].StartOrder
		if oi == 0 {
			return false
		}
		if oj == 0 {
			return true
		}
		return oi < oj
	})
	return services, nil
}

// Get returns a service by name, or nil if not found.
func (s *ServiceStore) Get(name string) (*model.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readFile(s.filePath(name))
}

// Save writes (or overwrites) a service definition atomically.
func (s *ServiceStore) Save(svc model.Service) error {
	if svc.Name == "" {
		return fmt.Errorf("service name is required")
	}
	if svc.Image == "" {
		return fmt.Errorf("service image is required")
	}
	if svc.Hostname == "" {
		svc.Hostname = "keel-" + svc.Name
	}
	if svc.Network == "" {
		svc.Network = defaultNetwork
	}

	data, err := json.MarshalIndent(svc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal service: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.servicesDir, 0755); err != nil {
		return fmt.Errorf("create services dir: %w", err)
	}
	return atomicWrite(s.filePath(svc.Name), data)
}

// GetRaw returns the raw JSON bytes for a named service.
func (s *ServiceStore) GetRaw(name string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.filePath(name))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("service not found: %s", name)
	}
	return data, err
}

// SaveRaw validates and atomically writes raw JSON for a named service.
func (s *ServiceStore) SaveRaw(name string, data []byte) error {
	var svc model.Service
	if err := json.Unmarshal(data, &svc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if svc.Name == "" || svc.Image == "" {
		return fmt.Errorf("missing required fields: name, image")
	}
	if svc.Name != name {
		return fmt.Errorf("service name in JSON (%q) must match path (%q)", svc.Name, name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWrite(s.filePath(name), data)
}

// Delete removes a service JSON file.
func (s *ServiceStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.filePath(name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GlobalConfig reads the global environment config (network settings).
// Returns defaults if the config file does not exist.
func (s *ServiceStore) GlobalConfig() (*model.GlobalConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.GlobalConfig{Network: defaultNetwork}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg model.GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// ListByGroup returns services filtered by group name.
func (s *ServiceStore) ListByGroup(group string) ([]model.Service, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var result []model.Service
	for _, svc := range all {
		if svc.Group == group {
			result = append(result, svc)
		}
	}
	return result, nil
}

// Groups returns the set of distinct group names across all services.
func (s *ServiceStore) Groups() ([]string, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var groups []string
	for _, svc := range all {
		if svc.Group != "" && !seen[svc.Group] {
			seen[svc.Group] = true
			groups = append(groups, svc.Group)
		}
	}
	return groups, nil
}

// AllServices is an alias for List(), kept for handler compatibility.
func (s *ServiceStore) AllServices() ([]model.Service, error) {
	return s.List()
}

// FindService is an alias for Get(), kept for handler compatibility.
func (s *ServiceStore) FindService(name string) (*model.Service, error) {
	return s.Get(name)
}

func (s *ServiceStore) filePath(name string) string {
	return filepath.Join(s.servicesDir, name+".json")
}

func (s *ServiceStore) readFile(path string) (*model.Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var svc model.Service
	if err := json.Unmarshal(data, &svc); err != nil {
		return nil, err
	}
	return &svc, nil
}

func atomicWrite(path string, data []byte) error {
	var origMode os.FileMode = 0644
	var origUID, origGID int = -1, -1
	if info, err := os.Stat(path); err == nil {
		origMode = info.Mode()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			origUID = int(stat.Uid)
			origGID = int(stat.Gid)
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".svc-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}

	os.Chmod(tmpPath, origMode)
	if origUID >= 0 {
		os.Chown(tmpPath, origUID, origGID)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
