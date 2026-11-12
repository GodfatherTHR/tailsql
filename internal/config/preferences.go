package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Default MCP and REST ports are loopback-only unless the user opts in.
const (
	DefaultMCPPort     = 9099
	DefaultRESTPort    = 9199
	DefaultFunnelPort  = 443
	DefaultSnapshotMax = 30
)

// Preference providers mirror internal/aihelper provider identifiers.
const (
	ProviderOllama       = "ollama"
	ProviderOpenRouter   = "openrouter"
	ProviderOpenAICompat = "openai"
)

// Preferences are the v0.3 feature settings persisted in ~/.tailsql/config.json.
// They are intentionally separate from instances.json so v0.2.1 state stays
// byte-compatible.
type Preferences struct {
	MCPEnabled             bool   `json:"mcpEnabled"`
	MCPPort                int    `json:"mcpPort"`
	MCPTailscaleEnabled    bool   `json:"mcpTailscaleEnabled"`
	MCPToken               string `json:"mcpToken"`
	RESTEnabled            bool   `json:"restEnabled"`
	RESTPort               int    `json:"restPort"`
	RESTTailscaleEnabled   bool   `json:"restTailscaleEnabled"`
	RESTFunnelEnabled      bool   `json:"restFunnelEnabled"`
	RESTFunnelPort         int    `json:"restFunnelPort"`
	RESTAPIKey             string `json:"restApiKey"`
	RESTAllowPublicRead    bool   `json:"restAllowPublicRead"`
	AgentReadOnly          bool   `json:"agentReadOnly"`
	AutoSnapshotOnAgentDDL bool   `json:"autoSnapshotOnAgentDdl"`
	AIProvider             string `json:"aiProvider"`
	AIBaseURL              string `json:"aiBaseUrl"`
	AIModel                string `json:"aiModel"`
	AIAPIKey               string `json:"aiApiKey"`
	SnapshotRetention      int    `json:"snapshotRetention"`
}

// DefaultPreferences returns safe loopback-only defaults with fresh tokens.
func DefaultPreferences() Preferences {
	return Preferences{
		MCPPort:                DefaultMCPPort,
		RESTPort:               DefaultRESTPort,
		RESTFunnelPort:         DefaultFunnelPort,
		AutoSnapshotOnAgentDDL: true,
		AIProvider:             ProviderOllama,
		AIBaseURL:              "http://127.0.0.1:11434",
		AIModel:                "qwen2.5-coder:7b",
		SnapshotRetention:      DefaultSnapshotMax,
	}
}

// PreferencesStore owns the feature settings file with atomic persistence.
type PreferencesStore struct {
	mu    sync.RWMutex
	path  string
	prefs Preferences
}

// OpenPreferences loads or creates ~/.tailsql/config.json. Missing tokens are
// generated and persisted so the first UI load can show them immediately.
func OpenPreferences(path string) (*PreferencesStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("preferences path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create preferences directory: %w", err)
	}
	store := &PreferencesStore{path: path, prefs: DefaultPreferences()}

	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		store.ensureTokens()
		if saveErr := store.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
		return store, nil
	case err != nil:
		return nil, fmt.Errorf("read preferences: %w", err)
	case len(strings.TrimSpace(string(data))) == 0:
		store.ensureTokens()
		if saveErr := store.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
		return store, nil
	}

	if err := json.Unmarshal(data, &store.prefs); err != nil {
		backup := path + ".corrupt"
		_ = os.Rename(path, backup)
		store.prefs = DefaultPreferences()
		store.ensureTokens()
		if saveErr := store.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
		return store, nil
	}
	store.normalize()
	if store.ensureTokens() {
		if saveErr := store.saveLocked(); saveErr != nil {
			return nil, saveErr
		}
	}
	return store, nil
}

// Get returns a copy of the current preferences.
func (s *PreferencesStore) Get() Preferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prefs
}

// Update validates, normalizes, and persists new preferences.
func (s *PreferencesStore) Update(prefs Preferences) error {
	prefs = normalizePreferences(prefs)
	if prefs.MCPToken == "" {
		prefs.MCPToken = generateToken("tsql_mcp_")
	}
	if prefs.RESTAPIKey == "" {
		prefs.RESTAPIKey = generateToken("tsql_rest_")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.prefs
	s.prefs = prefs
	if err := s.saveLocked(); err != nil {
		s.prefs = previous
		return err
	}
	return nil
}

// RotateMCPToken replaces the MCP bearer token and persists it.
func (s *PreferencesStore) RotateMCPToken() (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.prefs
	s.prefs.MCPToken = generateToken("tsql_mcp_")
	if err := s.saveLocked(); err != nil {
		s.prefs = previous
		return previous, err
	}
	return s.prefs, nil
}

// RotateRESTKey replaces the REST API key and persists it.
func (s *PreferencesStore) RotateRESTKey() (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.prefs
	s.prefs.RESTAPIKey = generateToken("tsql_rest_")
	if err := s.saveLocked(); err != nil {
		s.prefs = previous
		return previous, err
	}
	return s.prefs, nil
}

func (s *PreferencesStore) normalize() {
	s.prefs = normalizePreferences(s.prefs)
}

func (s *PreferencesStore) ensureTokens() bool {
	changed := false
	if strings.TrimSpace(s.prefs.MCPToken) == "" {
		s.prefs.MCPToken = generateToken("tsql_mcp_")
		changed = true
	}
	if strings.TrimSpace(s.prefs.RESTAPIKey) == "" {
		s.prefs.RESTAPIKey = generateToken("tsql_rest_")
		changed = true
	}
	return changed
}

func normalizePreferences(prefs Preferences) Preferences {
	if prefs.MCPPort < 1024 || prefs.MCPPort > 65535 {
		prefs.MCPPort = DefaultMCPPort
	}
	if prefs.RESTPort < 1024 || prefs.RESTPort > 65535 {
		prefs.RESTPort = DefaultRESTPort
	}
	if prefs.RESTPort == prefs.MCPPort {
		prefs.RESTPort = DefaultRESTPort
		if prefs.RESTPort == prefs.MCPPort {
			prefs.RESTPort = prefs.MCPPort + 1
		}
	}
	if prefs.RESTFunnelPort <= 0 {
		prefs.RESTFunnelPort = DefaultFunnelPort
	}
	if prefs.SnapshotRetention <= 0 {
		prefs.SnapshotRetention = DefaultSnapshotMax
	}
	switch strings.ToLower(strings.TrimSpace(prefs.AIProvider)) {
	case ProviderOpenRouter:
		prefs.AIProvider = ProviderOpenRouter
	case ProviderOpenAICompat, "openai-compatible", "custom":
		prefs.AIProvider = ProviderOpenAICompat
	default:
		prefs.AIProvider = ProviderOllama
	}
	prefs.AIBaseURL = strings.TrimRight(strings.TrimSpace(prefs.AIBaseURL), "/")
	if prefs.AIBaseURL == "" {
		if prefs.AIProvider == ProviderOllama {
			prefs.AIBaseURL = "http://127.0.0.1:11434"
		} else {
			prefs.AIBaseURL = "https://api.openai.com/v1"
		}
	}
	if strings.TrimSpace(prefs.AIModel) == "" {
		prefs.AIModel = "qwen2.5-coder:7b"
	}
	// Funnel forwards to the loopback listener, so tailnet binding is
	// meaningless while funnel is enabled.
	if prefs.RESTFunnelEnabled {
		prefs.RESTTailscaleEnabled = false
	}
	if !prefs.RESTEnabled {
		prefs.RESTFunnelEnabled = false
	}
	return prefs
}

func (s *PreferencesStore) saveLocked() error {
	data, err := json.MarshalIndent(s.prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary preferences file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, os.ErrInvalid) {
		cleanup()
		return fmt.Errorf("secure temporary preferences file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary preferences file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("flush preferences file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close preferences file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace preferences file: %w", err)
	}
	_ = os.Chmod(s.path, 0o600)
	return nil
}

func generateToken(prefix string) string {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return prefix + "fallback"
	}
	return prefix + hex.EncodeToString(buffer)
}
