package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 1

// Store owns the persisted application state. All reads and mutations are
// serialized; every mutation is written atomically before it becomes visible
// to callers.
type Store struct {
	mu       sync.RWMutex
	path     string
	state    State
	warnings []string
}

// Open loads (or initializes) the state file. A corrupt file is moved aside
// rather than preventing the application from starting.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("state path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	s := &Store{path: path}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.state = State{Version: schemaVersion, Instances: []Instance{}}
		if saveErr := s.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("read state file: %w", err)
	}

	if len(data) == 0 {
		s.warnings = append(s.warnings, "State file was empty; starting with a fresh configuration.")
		s.state = State{Version: schemaVersion, Instances: []Instance{}}
		if saveErr := s.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
		return s, nil
	}

	if err := json.Unmarshal(data, &s.state); err != nil {
		backup := path + ".corrupt-" + time.Now().UTC().Format("20060102T150405Z")
		if renameErr := os.Rename(path, backup); renameErr != nil {
			return nil, fmt.Errorf("state file is corrupt and could not be moved aside: %w", err)
		}
		s.warnings = append(s.warnings, "State file was corrupt; the previous file was preserved at "+backup+".")
		s.state = State{Version: schemaVersion, Instances: []Instance{}}
		if saveErr := s.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
		return s, nil
	}

	if s.state.Version == 0 {
		s.state.Version = schemaVersion
	}
	if s.state.Instances == nil {
		s.state.Instances = []Instance{}
	}
	return s, nil
}

// Path returns the absolute path of the state file.
func (s *Store) Path() string { return s.path }

// Warnings returns initialization warnings for the diagnostics panel.
func (s *Store) Warnings() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.warnings...)
}

// List returns a sorted deep copy of all instances.
func (s *Store) List() []Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Instance, len(s.state.Instances))
	for i, instance := range s.state.Instances {
		out[i] = instance.Clone()
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Get returns a copy of one instance.
func (s *Store) Get(id string) (Instance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, instance := range s.state.Instances {
		if instance.ID == id {
			return instance.Clone(), true
		}
	}
	return Instance{}, false
}

// Create persists a new instance. IDs and names must be unique.
func (s *Store) Create(instance Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.state.Instances {
		if existing.ID == instance.ID {
			return fmt.Errorf("instance id %q already exists", instance.ID)
		}
		if strings.EqualFold(existing.Name, instance.Name) {
			return fmt.Errorf("a database named %q already exists", instance.Name)
		}
	}

	previous := s.state.Instances
	s.state.Instances = append(append([]Instance(nil), previous...), instance.Clone())
	if err := s.saveLocked(); err != nil {
		s.state.Instances = previous
		return err
	}
	return nil
}

// Update replaces an existing instance by ID.
func (s *Store) Update(instance Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.state.Instances {
		if existing.ID != instance.ID {
			continue
		}
		previous := existing
		s.state.Instances[i] = instance.Clone()
		if err := s.saveLocked(); err != nil {
			s.state.Instances[i] = previous
			return err
		}
		return nil
	}
	return fmt.Errorf("instance %q not found", instance.ID)
}

// Delete removes an instance from the store.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.state.Instances {
		if existing.ID != id {
			continue
		}
		previous := s.state.Instances
		next := make([]Instance, 0, len(previous)-1)
		next = append(next, previous[:i]...)
		next = append(next, previous[i+1:]...)
		s.state.Instances = next
		if err := s.saveLocked(); err != nil {
			s.state.Instances = previous
			return err
		}
		return nil
	}
	return fmt.Errorf("instance %q not found", id)
}

// Settings returns the current settings copy.
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Settings
}

// SetSettings persists new settings.
func (s *Store) SetSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Settings
	s.state.Settings = settings
	if err := s.saveLocked(); err != nil {
		s.state.Settings = previous
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	if s.state.Instances == nil {
		s.state.Instances = []Instance{}
	}
	s.state.Version = schemaVersion

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".instances-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, os.ErrInvalid) {
		cleanup()
		return fmt.Errorf("secure temporary state file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary state file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("flush state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close state file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace state file: %w", err)
	}
	_ = os.Chmod(s.path, 0o600)
	return nil
}
