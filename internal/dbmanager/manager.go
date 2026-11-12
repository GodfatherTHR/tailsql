package dbmanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tailsql/internal/config"
	"tailsql/internal/jdbc"
)

// Config wires the manager to the state store and filesystem locations.
type Config struct {
	Store         *config.Store
	DataDir       string
	AssetsDir     string
	ExecutableDir string
	OnChange      func([]config.Instance)
	OnError       func(instanceID, message string)
}

// CreateInput is validated input for a new database.
type CreateInput struct {
	Name     string
	Username string
	Password string
	Port     int
}

type runtimeState struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	done      chan struct{}
	adopted   bool
	startedAt time.Time
}

// Manager owns H2 child processes and the lifecycle of their metadata.
type Manager struct {
	mu        sync.Mutex
	store     *config.Store
	dataDir   string
	dbDir     string
	logDir    string
	assetsDir string
	exeDir    string
	bridge    *jdbc.Bridge
	java      JavaInfo
	h2        H2Info
	runtimes  map[string]*runtimeState
	onChange  func([]config.Instance)
	onError   func(string, string)
}

// New performs initial detection but does not fail when Java or H2 is absent;
// those states are surfaced through Health instead.
func New(cfg Config) (*Manager, error) {
	if cfg.Store == nil {
		return nil, errors.New("state store is required")
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	manager := &Manager{
		store:     cfg.Store,
		dataDir:   dataDir,
		dbDir:     filepath.Join(dataDir, "dbs"),
		logDir:    filepath.Join(dataDir, "logs"),
		assetsDir: cfg.AssetsDir,
		exeDir:    cfg.ExecutableDir,
		runtimes:  map[string]*runtimeState{},
		onChange:  cfg.OnChange,
		onError:   cfg.OnError,
	}
	if err := os.MkdirAll(manager.dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	if err := os.MkdirAll(manager.logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	classDir, err := jdbc.EnsureClass(filepath.Join(dataDir, "bridge"))
	if err != nil {
		return nil, err
	}
	manager.bridge = &jdbc.Bridge{ClassDir: classDir}
	manager.refreshExecutablesLocked()
	return manager, nil
}

// DataDir exposes the root of TailSQL's application data.
func (m *Manager) DataDir() string { return m.dataDir }

// RefreshHealth re-runs Java and H2 discovery.
func (m *Manager) RefreshHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshExecutablesLocked()
}

func (m *Manager) refreshExecutablesLocked() {
	settings := m.store.Settings()
	m.java = DetectJava(settings.JavaPath)
	m.h2 = ResolveH2Jar(settings.H2JarPath, m.h2Candidates())
	m.bridge.JavaPath = m.java.Path
	m.bridge.JarPath = m.h2.Path
}

func (m *Manager) h2Candidates() []string {
	var candidates []string
	add := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		candidates = append(candidates, path)
	}
	if m.assetsDir != "" {
		add(filepath.Join(m.assetsDir, H2FileName))
	}
	if m.exeDir != "" {
		add(filepath.Join(m.exeDir, "assets", H2FileName))
	}
	add(filepath.Join(m.dataDir, "assets", H2FileName))
	if workingDir, err := os.Getwd(); err == nil {
		add(filepath.Join(workingDir, "assets", H2FileName))
	}
	seen := map[string]bool{}
	unique := candidates[:0]
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err == nil {
			candidate = absolute
		}
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}
	return unique
}

// Java returns a copy of the last Java detection result.
func (m *Manager) Java() JavaInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.java
}

// H2 returns a copy of the last H2 discovery result.
func (m *Manager) H2() H2Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.h2
}

// Bridge returns a stateless JDBC bridge configured with the current Java and
// H2 paths.
func (m *Manager) Bridge() *jdbc.Bridge {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &jdbc.Bridge{
		JavaPath: m.java.Path,
		JarPath:  m.h2.Path,
		ClassDir: m.bridge.ClassDir,
	}
}

// InstallH2 downloads the pinned engine into the application data directory.
func (m *Manager) InstallH2(ctx context.Context) (H2Info, error) {
	downloadDir := filepath.Join(m.dataDir, "assets")
	if _, err := DownloadH2(ctx, downloadDir); err != nil {
		return m.H2(), err
	}
	m.mu.Lock()
	m.refreshExecutablesLocked()
	h2 := m.h2
	m.mu.Unlock()
	if !h2.Available {
		return h2, fmt.Errorf("H2 engine was downloaded but could not be validated: %s", h2.Error)
	}
	return h2, nil
}

// Create validates and persists a new stopped instance.
func (m *Manager) Create(input CreateInput) (config.Instance, error) {
	name := NormalizeName(input.Name)
	if err := ValidateName(name); err != nil {
		return config.Instance{}, err
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = "sa"
	}
	if err := ValidateUsername(username); err != nil {
		return config.Instance{}, err
	}

	password := input.Password
	if password == "" {
		generated, err := GeneratePassword()
		if err != nil {
			return config.Instance{}, err
		}
		password = generated
	}
	if err := ValidatePassword(password); err != nil {
		return config.Instance{}, err
	}

	port := input.Port
	if port == 0 {
		allocated, err := FindAvailablePort("127.0.0.1", FirstLocalPort, PortScanAttempts)
		if err != nil {
			return config.Instance{}, err
		}
		port = allocated
	} else {
		if err := ValidatePort(port); err != nil {
			return config.Instance{}, err
		}
		if !PortAvailable("127.0.0.1", port) {
			return config.Instance{}, fmt.Errorf("port %d is already in use on 127.0.0.1", port)
		}
	}

	tailscalePort, err := FindAvailablePort("127.0.0.1", FirstTailscalePort, PortScanAttempts)
	if err != nil {
		return config.Instance{}, fmt.Errorf("allocate Tailscale proxy port: %w", err)
	}
	if tailscalePort == port {
		tailscalePort, err = FindAvailablePort("127.0.0.1", tailscalePort+1, PortScanAttempts)
		if err != nil {
			return config.Instance{}, fmt.Errorf("allocate Tailscale proxy port: %w", err)
		}
	}

	id, err := NewID()
	if err != nil {
		return config.Instance{}, err
	}
	serverPassword, err := GenerateServerPassword()
	if err != nil {
		return config.Instance{}, err
	}

	now := time.Now().UTC()
	instance := config.Instance{
		ID:             id,
		Name:           name,
		Username:       username,
		Password:       password,
		ServerPassword: serverPassword,
		Port:           port,
		Status:         config.StatusStopped,
		TailscalePort:  tailscalePort,
		DatabaseFile:   name + ".mv.db",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	m.mu.Lock()
	err = m.store.Create(instance)
	m.mu.Unlock()
	if err != nil {
		return config.Instance{}, err
	}
	m.emit()
	return m.enrich(instance), nil
}

// List returns all instances enriched with current file sizes.
func (m *Manager) List() []config.Instance {
	instances := m.store.List()
	for i := range instances {
		instances[i] = m.enrich(instances[i])
	}
	return instances
}

// Get returns one instance enriched with its current file size.
func (m *Manager) Get(id string) (config.Instance, bool) {
	instance, ok := m.store.Get(id)
	if !ok {
		return config.Instance{}, false
	}
	return m.enrich(instance), true
}

// ConnectionInfo returns the loopback JDBC connection details for a running instance.
func (m *Manager) ConnectionInfo(id string) (string, string, string, error) {
	instance, ok := m.store.Get(id)
	if !ok {
		return "", "", "", fmt.Errorf("database instance %q was not found", id)
	}
	if instance.Status != config.StatusRunning {
		return "", "", "", fmt.Errorf("database %s is %s; start it before running SQL", instance.Name, instance.Status)
	}
	return LocalJDBCURL(instance.Name, instance.Port), instance.Username, instance.Password, nil
}

// DatabasePath returns the absolute .mv.db path for an instance.
func (m *Manager) DatabasePath(id string) (string, error) {
	instance, ok := m.getStored(id)
	if !ok {
		return "", fmt.Errorf("database instance %q was not found", id)
	}
	return filepath.Join(m.dbDir, instance.DatabaseFile), nil
}

func (m *Manager) getStored(id string) (config.Instance, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Get(id)
}

// SetTailscaleState records the outcome of enabling/disabling the proxy.
func (m *Manager) SetTailscaleState(id string, enabled bool, ip string, port int) (config.Instance, error) {
	m.mu.Lock()
	instance, ok := m.store.Get(id)
	if !ok {
		m.mu.Unlock()
		return config.Instance{}, fmt.Errorf("database instance %q was not found", id)
	}
	if enabled && port <= 0 {
		m.mu.Unlock()
		return config.Instance{}, errors.New("a valid Tailscale proxy port is required")
	}
	instance.TailscaleEnabled = enabled
	if enabled {
		instance.TailscaleIP = ip
		instance.TailscalePort = port
	}
	instance.UpdatedAt = time.Now().UTC()
	err := m.store.Update(instance)
	m.mu.Unlock()
	if err != nil {
		return config.Instance{}, err
	}
	m.emit()
	return m.enrich(instance), nil
}

// SetFunnelState records whether public Tailscale Funnel hosting is active.
func (m *Manager) SetFunnelState(id string, enabled bool, port int, host string) (config.Instance, error) {
	m.mu.Lock()
	instance, ok := m.store.Get(id)
	if !ok {
		m.mu.Unlock()
		return config.Instance{}, fmt.Errorf("database instance %q was not found", id)
	}
	if enabled && port <= 0 {
		m.mu.Unlock()
		return config.Instance{}, errors.New("a valid Funnel port is required")
	}
	instance.FunnelEnabled = enabled
	if enabled {
		instance.FunnelPort = port
		instance.FunnelHost = host
	} else {
		instance.FunnelPort = 0
		instance.FunnelHost = ""
	}
	instance.UpdatedAt = time.Now().UTC()
	err := m.store.Update(instance)
	m.mu.Unlock()
	if err != nil {
		return config.Instance{}, err
	}
	m.emit()
	return m.enrich(instance), nil
}

// LogPath is the H2 server output log for an instance.
func (m *Manager) LogPath(id string) string {
	return filepath.Join(m.logDir, id+".log")
}

// ReadLog returns the tail of an instance's H2 server log.
func (m *Manager) ReadLog(id string, lines int) (string, error) {
	if _, ok := m.store.Get(id); !ok {
		return "", fmt.Errorf("database instance %q was not found", id)
	}
	if lines <= 0 {
		lines = 200
	}
	if lines > 2000 {
		lines = 2000
	}
	content, err := os.ReadFile(m.LogPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read server log: %w", err)
	}
	all := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), nil
}

func (m *Manager) enrich(instance config.Instance) config.Instance {
	info, err := os.Stat(filepath.Join(m.dbDir, instance.DatabaseFile))
	if err == nil {
		instance.FileSizeBytes = info.Size()
	}
	return instance
}

func (m *Manager) emit() {
	callback := m.onChange
	if callback == nil {
		return
	}
	callback(m.List())
}

func (m *Manager) notifyError(instanceID, message string) {
	if m.onError != nil {
		m.onError(instanceID, message)
	}
}

func isJavaImage(image string) bool {
	base := strings.ToLower(strings.TrimSpace(image))
	base = strings.TrimSuffix(base, ".exe")
	return base == "java"
}

func looksLikeH2CommandLine(commandLine string, port int) bool {
	if strings.TrimSpace(commandLine) == "" {
		return false
	}
	lower := strings.ToLower(commandLine)
	return strings.Contains(lower, "org.h2.tools.server") &&
		strings.Contains(lower, "-tcpport") &&
		strings.Contains(lower, strconv.Itoa(port))
}

func truncateOutput(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
