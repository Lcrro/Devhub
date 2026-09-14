package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Lcrro/devhub/internal/model"
)

type Store struct {
	mu        sync.RWMutex
	state     model.State
	dataDir   string
	statePath string
	coversDir string
}

func DefaultDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "devhub"), nil
}

func New(dataDir string) (*Store, error) {
	if dataDir == "" {
		var err error
		dataDir, err = DefaultDataDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	coversDir := filepath.Join(dataDir, "covers")
	if err := os.MkdirAll(coversDir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		dataDir:   dataDir,
		statePath: filepath.Join(dataDir, "state.json"),
		coversDir: coversDir,
		state: model.State{
			Version:  1,
			Settings: model.Settings{AutoRemember: true, AutoCapture: true, ScanIntervalSeconds: 5},
			Projects: map[string]*model.Project{},
			Ignored:  map[string]bool{},
		},
	}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func (s *Store) DataDir() string   { return s.dataDir }
func (s *Store) CoversDir() string { return s.coversDir }

func (s *Store) load() error {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		return err
	}
	var state model.State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Projects == nil {
		state.Projects = map[string]*model.Project{}
	}
	if state.Ignored == nil {
		state.Ignored = map[string]bool{}
	}
	if state.Settings.ScanIntervalSeconds <= 0 {
		state.Settings.ScanIntervalSeconds = 5
	}
	s.state = state
	return nil
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath)
}

func cloneProject(p *model.Project) *model.Project {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

func (s *Store) Snapshot() model.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := model.State{Version: s.state.Version, Settings: s.state.Settings, Projects: map[string]*model.Project{}, Ignored: map[string]bool{}}
	for id, p := range s.state.Projects {
		out.Projects[id] = cloneProject(p)
	}
	for id, ignored := range s.state.Ignored {
		out.Ignored[id] = ignored
	}
	return out
}

func (s *Store) Get(id string) (*model.Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.state.Projects[id]
	return cloneProject(p), ok
}

func (s *Store) Upsert(p model.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if existing, ok := s.state.Projects[p.ID]; ok {
		if p.CreatedAt.IsZero() {
			p.CreatedAt = existing.CreatedAt
		}
		if p.CoverFile == "" {
			p.CoverFile = existing.CoverFile
		}
		if p.LastCaptured.IsZero() {
			p.LastCaptured = existing.LastCaptured
		}
		if p.LastStarted.IsZero() {
			p.LastStarted = existing.LastStarted
		}
		if existing.Managed {
			p.Managed = true
		}
		if p.StartCommand == "" {
			p.StartCommand = existing.StartCommand
		}
		if p.Root == "" {
			p.Root = existing.Root
		}
		if p.WorkingDir == "" {
			p.WorkingDir = existing.WorkingDir
		}
		if p.Name == "" {
			p.Name = existing.Name
		}
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	cp := p
	s.state.Projects[p.ID] = &cp
	return s.saveLocked()
}

func (s *Store) Update(id string, fn func(*model.Project)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.state.Projects[id]
	if !ok {
		return os.ErrNotExist
	}
	fn(p)
	p.UpdatedAt = time.Now()
	return s.saveLocked()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.state.Projects[id]
	if ok && p.CoverFile != "" {
		_ = os.Remove(filepath.Join(s.coversDir, filepath.Base(p.CoverFile)))
	}
	delete(s.state.Projects, id)
	return s.saveLocked()
}

func (s *Store) IsIgnored(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Ignored[id]
}

func (s *Store) Ignore(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Ignored == nil {
		s.state.Ignored = map[string]bool{}
	}
	s.state.Ignored[id] = true
	if p, ok := s.state.Projects[id]; ok && p.CoverFile != "" {
		_ = os.Remove(filepath.Join(s.coversDir, filepath.Base(p.CoverFile)))
	}
	delete(s.state.Projects, id)
	return s.saveLocked()
}

func (s *Store) UpdateSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if settings.ScanIntervalSeconds < 2 {
		settings.ScanIntervalSeconds = 2
	}
	if settings.ScanIntervalSeconds > 60 {
		settings.ScanIntervalSeconds = 60
	}
	s.state.Settings = settings
	return s.saveLocked()
}
