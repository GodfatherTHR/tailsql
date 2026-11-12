package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tailsql/internal/config"
	"tailsql/internal/snapshots"
	"tailsql/internal/sqlrunner"
)

func (a *App) snapshotStore() (*snapshots.Store, error) {
	a.mu.RLock()
	store := a.snapshots
	a.mu.RUnlock()
	if store == nil {
		return nil, errors.New("snapshot store is not initialized")
	}
	return store, nil
}

// ListSnapshots returns checkpoints for one database, newest first.
func (a *App) ListSnapshots(id string) ([]SnapshotView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	instance, err := a.resolveInstance(id)
	if err != nil {
		return nil, err
	}
	store, err := a.snapshotStore()
	if err != nil {
		return nil, err
	}
	list, err := store.List(instance.Name)
	if err != nil {
		return nil, err
	}
	views := make([]SnapshotView, 0, len(list))
	for _, entry := range list {
		views = append(views, SnapshotView{
			ID:        entry.ID,
			Label:     entry.Label,
			SizeBytes: entry.SizeBytes,
			CreatedAt: entry.CreatedAt,
		})
	}
	return views, nil
}

// CreateSnapshot creates a manual checkpoint via online H2 backup or file copy.
func (a *App) CreateSnapshot(request SnapshotRequest) (SnapshotView, error) {
	if err := a.ready(); err != nil {
		return SnapshotView{}, err
	}
	instance, err := a.resolveInstance(request.ID)
	if err != nil {
		return SnapshotView{}, err
	}
	label := strings.TrimSpace(request.Label)
	if label == "" {
		label = "Manual checkpoint"
	}
	return a.createCheckpoint(instance, label)
}

func (a *App) createCheckpoint(instance config.Instance, label string) (SnapshotView, error) {
	store, err := a.snapshotStore()
	if err != nil {
		return SnapshotView{}, err
	}
	manager, _, _ := a.components()
	if manager == nil {
		return SnapshotView{}, errors.New("database manager is not initialized")
	}
	var entry snapshots.Snapshot
	if instance.Status == config.StatusRunning {
		tempZip := filepath.Join(os.TempDir(), fmt.Sprintf("tailsql-backup-%d.zip", time.Now().UnixNano()))
		defer os.Remove(tempZip)
		statement := "BACKUP TO '" + escapeSQLPath(filepath.ToSlash(tempZip)) + "'"
		runner := sqlrunner.New(manager, manager.Bridge())
		ctx, cancel := context.WithTimeout(a.baseContext(), 120*time.Second)
		defer cancel()
		if _, err := runner.Execute(ctx, instance.ID, statement); err != nil {
			return SnapshotView{}, fmt.Errorf("H2 online backup failed: %w", err)
		}
		entry, err = store.SaveBackup(instance.Name, tempZip, label)
	} else {
		databasePath, pathErr := manager.DatabasePath(instance.ID)
		if pathErr != nil {
			return SnapshotView{}, pathErr
		}
		if _, statErr := os.Stat(databasePath); statErr != nil {
			return SnapshotView{}, fmt.Errorf("database file is not available: %w", statErr)
		}
		entry, err = store.SaveDatabaseFile(instance.Name, databasePath, label)
	}
	if err != nil {
		return SnapshotView{}, err
	}
	if prefs, ok := a.preferences(); ok {
		retention := prefs.Get().SnapshotRetention
		_ = store.Prune(instance.Name, retention)
	}
	a.emit("snapshots:changed", map[string]string{"instanceId": instance.ID})
	return SnapshotView{
		ID:        entry.ID,
		Label:     entry.Label,
		SizeBytes: entry.SizeBytes,
		CreatedAt: entry.CreatedAt,
	}, nil
}

// RestoreSnapshot stops H2, replaces the database file, and restarts on the
// same port, re-enabling Tailscale hosting/Funnel when they were active.
func (a *App) RestoreSnapshot(request SnapshotRequest) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	if strings.TrimSpace(request.SnapshotID) == "" {
		return InstanceView{}, errors.New("select a checkpoint to restore")
	}
	instance, err := a.resolveInstance(request.ID)
	if err != nil {
		return InstanceView{}, err
	}
	store, err := a.snapshotStore()
	if err != nil {
		return InstanceView{}, err
	}
	targetPath, err := store.RestorePath(instance.Name, request.SnapshotID)
	if err != nil {
		return InstanceView{}, err
	}
	_ = targetPath

	wasTailscale := instance.TailscaleEnabled
	wasFunnel := instance.FunnelEnabled
	funnelPort := instance.FunnelPort

	if _, safetyErr := a.createCheckpoint(instance, "Pre-Restore safety copy"); safetyErr != nil {
		a.emit("app:error", AppErrorEvent{
			InstanceID: instance.ID,
			Message:    "Restore continued without a safety checkpoint: " + safetyErr.Error(),
		})
	}

	manager, _, hosts := a.components()
	if hosts != nil {
		hosts.Stop(instance.ID)
	}
	if instance.Status == config.StatusRunning || instance.Status == config.StatusError {
		if _, stopErr := manager.Stop(instance.ID); stopErr != nil {
			return InstanceView{}, fmt.Errorf("stop %s before restore: %w", instance.Name, stopErr)
		}
	} else if instance.Status == config.StatusStarting || instance.Status == config.StatusStopping {
		return InstanceView{}, fmt.Errorf("%s is %s; wait a moment and try again", instance.Name, instance.Status)
	}

	current, ok := manager.Get(instance.ID)
	if !ok {
		return InstanceView{}, fmt.Errorf("database %q was not found", instance.Name)
	}
	if current.Status != config.StatusStopped {
		return InstanceView{}, fmt.Errorf("%s did not stop cleanly; refusing to replace its file", instance.Name)
	}
	databasePath, err := manager.DatabasePath(instance.ID)
	if err != nil {
		return InstanceView{}, err
	}
	if err := store.Restore(instance.Name, request.SnapshotID, databasePath); err != nil {
		return InstanceView{}, err
	}
	restored, err := manager.Start(instance.ID)
	if err != nil {
		return InstanceView{}, fmt.Errorf("database file was restored, but H2 failed to start: %w", err)
	}
	if wasTailscale {
		if _, proxyErr := a.EnableTailscaleHosting(instance.ID); proxyErr != nil {
			a.emit("app:error", AppErrorEvent{
				InstanceID: instance.ID,
				Message:    "Restored, but Tailscale hosting did not restart: " + proxyErr.Error(),
			})
		}
	}
	if wasFunnel {
		if _, funnelErr := a.enableFunnelForInstance(restored, funnelPort); funnelErr != nil {
			a.emit("app:error", AppErrorEvent{
				InstanceID: instance.ID,
				Message:    "Restored, but Funnel hosting did not restart: " + funnelErr.Error(),
			})
		}
	}
	a.emit("snapshots:changed", map[string]string{"instanceId": instance.ID})
	return a.toView(restored), nil
}

// DeleteSnapshot removes one checkpoint file.
func (a *App) DeleteSnapshot(request SnapshotRequest) error {
	if err := a.ready(); err != nil {
		return err
	}
	instance, err := a.resolveInstance(request.ID)
	if err != nil {
		return err
	}
	store, err := a.snapshotStore()
	if err != nil {
		return err
	}
	if err := store.Delete(instance.Name, request.SnapshotID); err != nil {
		return err
	}
	a.emit("snapshots:changed", map[string]string{"instanceId": instance.ID})
	return nil
}

func escapeSQLPath(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}
