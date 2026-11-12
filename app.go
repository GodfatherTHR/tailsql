package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tailsql/internal/config"
	"tailsql/internal/dbmanager"
	"tailsql/internal/mcp"
	"tailsql/internal/restapi"
	"tailsql/internal/snapshots"
	"tailsql/internal/tailscale"
)

const (
	appVersion     = "0.3.0"
	tailscaleCheck = 8 * time.Second
)

// App is the Wails-bound backend facade.
type App struct {
	version string

	mu             sync.RWMutex
	ctx            context.Context
	store          *config.Store
	prefs          *config.PreferencesStore
	manager        *dbmanager.Manager
	hosts          *tailscale.HostManager
	snapshots      *snapshots.Store
	mcp            *mcp.Server
	rest           *restapi.Server
	health         SystemHealth
	initErr        error
	shuttingDown   atomic.Bool
	realtimeCancel context.CancelFunc
	realtimeStamps map[string]databaseStamp
	realtimeEcho   map[string]time.Time

	restFunnelActive bool
	restFunnelPort   int
	restFunnelError  string
}

// NewApp builds the application facade; initialization happens in startup.
func NewApp(version string) *App {
	return &App{version: version}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		a.recordInitError(fmt.Errorf("resolve home directory: %w", err))
		return
	}
	dataDir := filepath.Join(home, ".tailsql")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		a.recordInitError(fmt.Errorf("create application data directory %s: %w", dataDir, err))
		return
	}

	store, err := config.Open(filepath.Join(dataDir, "instances.json"))
	if err != nil {
		a.recordInitError(err)
		return
	}
	a.mu.Lock()
	a.store = store
	a.mu.Unlock()

	executableDir := ""
	if executable, err := os.Executable(); err == nil {
		executableDir = filepath.Dir(executable)
	}
	workingDir, _ := os.Getwd()
	assetsDir := ""
	if workingDir != "" {
		assetsDir = filepath.Join(workingDir, "assets")
	}

	manager, err := dbmanager.New(dbmanager.Config{
		Store:         store,
		DataDir:       dataDir,
		AssetsDir:     assetsDir,
		ExecutableDir: executableDir,
		OnChange: func(instances []config.Instance) {
			a.emit("instances:changed", a.instanceViews(instances))
		},
		OnError: func(instanceID, message string) {
			if instanceID != "" {
				a.mu.RLock()
				hosts := a.hosts
				a.mu.RUnlock()
				if hosts != nil {
					hosts.Stop(instanceID)
				}
				go a.cleanupFunnelForInstanceID(instanceID)
			}
			a.emit("app:error", AppErrorEvent{InstanceID: instanceID, Message: message})
		},
	})
	if err != nil {
		a.recordInitError(err)
		return
	}
	a.mu.Lock()
	a.manager = manager
	a.mu.Unlock()

	a.mu.Lock()
	a.hosts = tailscale.NewHostManager(func(snapshot tailscale.Snapshot) {
		a.emit("proxy:snapshot", snapshot)
	})
	a.mu.Unlock()

	detectionContext, cancelDetection := context.WithTimeout(a.baseContext(), tailscaleCheck)
	tailscaleStatus := tailscale.Detect(detectionContext, store.Settings().TailscalePath)
	cancelDetection()
	a.setHealth(SystemHealth{
		AppVersion:  a.version,
		DataDir:     dataDir,
		Java:        manager.Java(),
		H2:          manager.H2(),
		Tailscale:   tailscaleStatus,
		Warnings:    store.Warnings(),
		Initialized: true,
	})

	reconcileContext, cancelReconcile := context.WithTimeout(a.baseContext(), 20*time.Second)
	manager.Reconcile(reconcileContext)
	cancelReconcile()

	a.startRealtimeWatcher()

	a.mu.Lock()
	a.health.Java = manager.Java()
	a.health.H2 = manager.H2()
	a.health.Warnings = store.Warnings()
	a.mu.Unlock()

	go a.reconcileFunnels()

	prefs, prefsErr := config.OpenPreferences(filepath.Join(dataDir, "config.json"))
	if prefsErr != nil {
		a.recordInitError(prefsErr)
		return
	}
	snapshotStore, snapshotErr := snapshots.New(filepath.Join(dataDir, "snapshots"))
	if snapshotErr != nil {
		a.recordInitError(snapshotErr)
		return
	}
	a.mu.Lock()
	a.prefs = prefs
	a.snapshots = snapshotStore
	a.mcp = mcp.New(mcp.Config{Version: a.version}, newMCPBackend(a))
	a.rest = restapi.New(restapi.Config{Version: a.version}, newRESTBackend(a))
	a.mu.Unlock()
	a.applyAgentServers()

	a.emit("instances:changed", a.instanceViews(manager.List()))
	a.emit("tailscale:status", tailscaleStatus)
	a.emitAgentState()
}

func (a *App) shutdown(_ context.Context) {
	a.shuttingDown.Store(true)
	a.stopRealtimeWatcher()
	a.stopAgentServers()
	a.mu.RLock()
	hosts := a.hosts
	manager := a.manager
	a.mu.RUnlock()

	if hosts != nil {
		hosts.StopAll()
	}
	if manager != nil {
		a.disableAllFunnels(manager.List())
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		manager.StopAll(ctx)
		cancel()
	}
}

func (a *App) disableAllFunnels(instances []config.Instance) {
	var wait sync.WaitGroup
	for _, instance := range instances {
		if !instance.FunnelEnabled && instance.FunnelPort <= 0 {
			continue
		}
		wait.Add(1)
		go func(target config.Instance) {
			defer wait.Done()
			_ = a.disableFunnelForInstance(target)
		}(instance)
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
	}
}

func (a *App) recordInitError(err error) {
	a.mu.Lock()
	a.initErr = err
	a.health = SystemHealth{
		AppVersion:  a.version,
		Initialized: false,
		Error:       err.Error(),
		Warnings:    []string{},
	}
	a.mu.Unlock()
}

func (a *App) setHealth(health SystemHealth) {
	if health.Warnings == nil {
		health.Warnings = []string{}
	}
	a.mu.Lock()
	a.health = health
	a.mu.Unlock()
}

func (a *App) currentHealth() SystemHealth {
	a.mu.RLock()
	health := a.health
	store := a.store
	manager := a.manager
	a.mu.RUnlock()
	if manager != nil {
		health.Java = manager.Java()
		health.H2 = manager.H2()
	}
	if store != nil {
		health.Warnings = store.Warnings()
	}
	if health.Warnings == nil {
		health.Warnings = []string{}
	}
	return health
}

func (a *App) ready() error {
	a.mu.RLock()
	err := a.initErr
	manager := a.manager
	a.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("TailSQL backend is not initialized: %w", err)
	}
	if manager == nil {
		return fmt.Errorf("TailSQL backend is not initialized")
	}
	return nil
}

func (a *App) components() (*dbmanager.Manager, *config.Store, *tailscale.HostManager) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.manager, a.store, a.hosts
}

func (a *App) baseContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *App) emit(event string, payload any) {
	if a.shuttingDown.Load() {
		return
	}
	ctx := a.baseContext()
	if ctx == nil {
		return
	}
	runtime.EventsEmit(ctx, event, payload)
}

func (a *App) tailscaleStatus() tailscale.Status {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.health.Tailscale
}

func (a *App) updateTailscaleStatus(status tailscale.Status) {
	a.mu.Lock()
	a.health.Tailscale = status
	a.mu.Unlock()
	a.emit("tailscale:status", status)
}

func (a *App) instanceViews(instances []config.Instance) []InstanceView {
	views := make([]InstanceView, 0, len(instances))
	a.mu.RLock()
	manager := a.manager
	a.mu.RUnlock()
	for _, instance := range instances {
		view := InstanceView{
			ID:               instance.ID,
			Name:             instance.Name,
			Username:         instance.Username,
			Password:         instance.Password,
			Port:             instance.Port,
			Status:           instance.Status,
			PID:              instance.PID,
			TailscalePort:    instance.TailscalePort,
			TailscaleEnabled: instance.TailscaleEnabled,
			TailscaleIP:      instance.TailscaleIP,
			FunnelEnabled:    instance.FunnelEnabled,
			FunnelPort:       instance.FunnelPort,
			FunnelHost:       instance.FunnelHost,
			DatabaseFile:     instance.DatabaseFile,
			FileSizeBytes:    instance.FileSizeBytes,
			CreatedAt:        instance.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:        instance.UpdatedAt.UTC().Format(time.RFC3339),
			LastError:        instance.LastError,
			JDBCURL:          dbmanager.LocalJDBCURL(instance.Name, instance.Port),
		}
		if instance.StartedAt != nil {
			view.StartedAt = instance.StartedAt.UTC().Format(time.RFC3339)
		}
		if instance.TailscaleEnabled && instance.TailscaleIP != "" && instance.TailscalePort > 0 {
			view.TailscaleURL = fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s",
				instance.TailscaleIP, instance.TailscalePort, instance.Name)
		}
		if instance.FunnelEnabled && instance.FunnelHost != "" && instance.FunnelPort > 0 {
			view.FunnelURL = fmt.Sprintf("tcp://%s:%d", instance.FunnelHost, instance.FunnelPort)
			view.FunnelJDBCURL = fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s",
				instance.FunnelHost, instance.FunnelPort, instance.Name)
		}
		if manager != nil {
			view.LogPath = manager.LogPath(instance.ID)
		}
		views = append(views, view)
	}
	return views
}
