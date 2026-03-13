package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/getkaze/keel/internal/model"
)

// SeederStore provides thread-safe read access to per-seeder JSON files
// stored in {keelDir}/data/seeders/.
type SeederStore struct {
	mu         sync.RWMutex
	seedersDir string
}

// NewSeederStore creates a store backed by {keelDir}/data/seeders/.
func NewSeederStore(keelDir string) *SeederStore {
	return &SeederStore{
		seedersDir: filepath.Join(keelDir, "data", "seeders"),
	}
}

// List returns all seeder definitions from the seeders directory.
// Only JSON files are returned; non-JSON files (scripts) are ignored.
// Returns an empty slice if the directory does not exist.
func (s *SeederStore) List() ([]model.Seeder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.seedersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read seeders dir: %w", err)
	}

	var seeders []model.Seeder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sd, err := s.readFile(filepath.Join(s.seedersDir, e.Name()))
		if err != nil {
			continue // skip malformed files
		}
		seeders = append(seeders, *sd)
	}
	sort.Slice(seeders, func(i, j int) bool {
		return seeders[i].Order < seeders[j].Order
	})
	return seeders, nil
}

// Get returns a seeder by name, or nil if not found.
func (s *SeederStore) Get(name string) (*model.Seeder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readFile(s.filePath(name))
}

// GetRaw returns the raw JSON bytes for a named seeder.
func (s *SeederStore) GetRaw(name string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.filePath(name))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("seeder not found: %s", name)
	}
	return data, err
}

// Dir returns the seeders directory path.
func (s *SeederStore) Dir() string {
	return s.seedersDir
}

func (s *SeederStore) filePath(name string) string {
	return filepath.Join(s.seedersDir, name+".json")
}

func (s *SeederStore) readFile(path string) (*model.Seeder, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sd model.Seeder
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}
