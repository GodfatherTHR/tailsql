package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hysql/internal/codegen"
	"hysql/internal/config"
	"hysql/internal/dbmanager"
	"hysql/internal/jdbc"
	"hysql/internal/sqlrunner"
	"hysql/internal/tailscale"
)

// GetSystemHealth returns the cached diagnostics snapshot.
func (a *App) GetSystemHealth() SystemHealth {
	return a.currentHealth()
}

// RefreshSystemHealth re-runs Java, H2, and Tailscale discovery.
func (a *App) RefreshSystemHealth() SystemHealth {
	manager, store, _ := a.components()
	if manager == nil {
		return a.currentHealth()
	}
	manager.RefreshHealth()
	status := a.tailscaleStatus()
	if store != nil {
		ctx, cancel := context.WithTimeout(a.baseContext(), tailscaleCheck)
		status = tailscale.Detect(ctx, store.Settings().TailscalePath)
		cancel()
	}
	a.mu.Lock()
	a.health.Java = manager.Java()
	a.health.H2 = manager.H2()
	a.health.Tailscale = status
	a.mu.Unlock()
	a.emit("tailscale:status", status)
	return a.currentHealth()
}

// RefreshTailscale re-detects the Tailscale CLI state.
func (a *App) RefreshTailscale() tailscale.Status {
	_, store, _ := a.components()
	if store == nil {
		return a.tailscaleStatus()
	}
	ctx, cancel := context.WithTimeout(a.baseContext(), tailscaleCheck)
	status := tailscale.Detect(ctx, store.Settings().TailscalePath)
	cancel()
	a.updateTailscaleStatus(status)
	return status
}

// InstallH2Engine downloads the pinned H2 jar into the application data folder.
func (a *App) InstallH2Engine() (SystemHealth, error) {
	if err := a.ready(); err != nil {
		return a.currentHealth(), err
	}
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 3*time.Minute)
	defer cancel()
	if _, err := manager.InstallH2(ctx); err != nil {
		return a.currentHealth(), err
	}
	a.mu.Lock()
	a.health.Java = manager.Java()
	a.health.H2 = manager.H2()
	a.mu.Unlock()
	return a.currentHealth(), nil
}

// ListInstances returns all database instances with current file sizes.
func (a *App) ListInstances() ([]InstanceView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	manager, _, _ := a.components()
	return a.instanceViews(manager.List()), nil
}

// CreateInstance validates, persists, and starts a new database.
func (a *App) CreateInstance(request CreateInstanceRequest) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	manager, _, _ := a.components()
	instance, err := manager.Create(dbmanager.CreateInput{
		Name:     request.Name,
		Username: request.Username,
		Password: request.Password,
		Port:     request.Port,
	})
	if err != nil {
		return InstanceView{}, err
	}
	started, startErr := manager.Start(instance.ID)
	if startErr != nil {
		current, _ := manager.Get(instance.ID)
		return a.toView(current), fmt.Errorf("database created but failed to start: %w", startErr)
	}
	return a.toView(started), nil
}

// StartInstance launches the H2 TCP server.
func (a *App) StartInstance(id string) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	manager, _, hosts := a.components()
	if hosts != nil {
		hosts.Stop(id)
	}
	instance, err := manager.Start(id)
	if err != nil {
		return InstanceView{}, err
	}
	if instance.FunnelEnabled && instance.FunnelPort > 0 {
		if _, funnelErr := a.enableFunnelForInstance(instance, instance.FunnelPort); funnelErr != nil {
			return a.toView(instance), fmt.Errorf("database started, but public Funnel hosting could not be restored: %w", funnelErr)
		}
	}
	return a.toView(instance), nil
}

// StopInstance gracefully shuts down the H2 TCP server.
func (a *App) StopInstance(id string) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	manager, _, hosts := a.components()
	if current, ok := manager.Get(id); ok {
		if funnelErr := a.disableFunnelForInstance(current); funnelErr != nil {
			return a.toView(current), funnelErr
		}
	}
	if hosts != nil {
		hosts.Stop(id)
	}
	instance, err := manager.Stop(id)
	if err != nil {
		return a.toView(instance), err
	}
	return a.toView(instance), nil
}

// RestartInstance stops and starts the database.
func (a *App) RestartInstance(id string) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	manager, _, hosts := a.components()
	current, _ := manager.Get(id)
	if funnelErr := a.disableFunnelForInstance(current); funnelErr != nil {
		return a.toView(current), funnelErr
	}
	if hosts != nil {
		hosts.Stop(id)
	}
	instance, err := manager.Restart(id)
	if err != nil {
		return InstanceView{}, err
	}
	if current.FunnelEnabled && current.FunnelPort > 0 {
		if _, funnelErr := a.enableFunnelForInstance(instance, current.FunnelPort); funnelErr != nil {
			return a.toView(instance), fmt.Errorf("database restarted, but public Funnel hosting could not be restored: %w", funnelErr)
		}
	}
	return a.toView(instance), nil
}

// DeleteInstance removes metadata, stopping the server first if needed.
func (a *App) DeleteInstance(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	manager, _, hosts := a.components()
	if current, ok := manager.Get(id); ok {
		if funnelErr := a.disableFunnelForInstance(current); funnelErr != nil {
			return funnelErr
		}
	}
	if hosts != nil {
		hosts.Stop(id)
	}
	return manager.Delete(id)
}

// EnableTailscaleHosting starts a proxy bound to the detected Tailscale IPv4.
func (a *App) EnableTailscaleHosting(id string) (tailscale.Snapshot, error) {
	if err := a.ready(); err != nil {
		return inactiveProxySnapshot(id), err
	}
	manager, store, hosts := a.components()
	if hosts == nil || store == nil {
		return inactiveProxySnapshot(id), errors.New("Tailscale host manager is not initialized")
	}
	instance, ok := manager.Get(id)
	if !ok {
		return inactiveProxySnapshot(id), fmt.Errorf("database instance %q was not found", id)
	}
	if instance.Status != config.StatusRunning {
		return inactiveProxySnapshot(id), fmt.Errorf("start %s before hosting it on Tailscale", instance.Name)
	}

	if snapshot := hosts.Snapshot(id); snapshot.Running {
		ip := instance.TailscaleIP
		if ip == "" {
			ip = a.tailscaleStatus().IPv4
		}
		if _, stateErr := manager.SetTailscaleState(id, true, ip, snapshot.Port); stateErr != nil {
			return snapshot, stateErr
		}
		return snapshot, nil
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), tailscaleCheck)
	status := tailscale.Detect(ctx, store.Settings().TailscalePath)
	cancel()
	a.updateTailscaleStatus(status)
	if !status.Running || status.IPv4 == "" {
		message := status.Error
		if message == "" {
			message = "Tailscale is not ready; open Tailscale and sign in."
		}
		return inactiveProxySnapshot(id), errors.New(message)
	}

	startPort := instance.TailscalePort
	if startPort < 1024 {
		startPort = dbmanager.FirstTailscalePort
	}
	var lastErr error
	for port := startPort; port <= 65535 && port < startPort+25; port++ {
		snapshot, err := hosts.Start(tailscale.ProxyOptions{
			InstanceID: id,
			BindHost:   status.IPv4,
			Port:       port,
			TargetHost: "127.0.0.1",
			TargetPort: instance.Port,
		})
		if err == nil {
			if _, stateErr := manager.SetTailscaleState(id, true, status.IPv4, port); stateErr != nil {
				hosts.Stop(id)
				return inactiveProxySnapshot(id), stateErr
			}
			return snapshot, nil
		}
		lastErr = err
		if !tailscale.IsAddressInUse(err) {
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no proxy port was available")
	}
	return inactiveProxySnapshot(id), fmt.Errorf("could not start Tailscale hosting: %w", lastErr)
}

// DisableTailscaleHosting closes the proxy and clears the persisted flag.
func (a *App) DisableTailscaleHosting(id string) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	manager, _, hosts := a.components()
	if hosts != nil {
		hosts.Stop(id)
	}
	instance, err := manager.SetTailscaleState(id, false, "", 0)
	if err != nil {
		return InstanceView{}, err
	}
	return a.toView(instance), nil
}

// GetProxySnapshot returns live client/metric state for an instance.
func (a *App) GetProxySnapshot(id string) tailscale.Snapshot {
	_, _, hosts := a.components()
	if hosts == nil {
		return inactiveProxySnapshot(id)
	}
	return hosts.Snapshot(id)
}

// GetFunnelState reports whether this database is published through Tailscale
// Funnel and which public endpoint is active.
func (a *App) GetFunnelState(id string) (tailscale.FunnelState, error) {
	state := tailscale.FunnelState{
		InstanceID:   id,
		AllowedPorts: append([]int(nil), tailscale.FunnelAllowedPorts...),
	}
	if err := a.ready(); err != nil {
		state.Error = err.Error()
		return state, err
	}
	manager, _, _ := a.components()
	instance, ok := manager.Get(id)
	if !ok {
		err := fmt.Errorf("database instance %q was not found", id)
		state.Error = err.Error()
		return state, err
	}
	status := a.currentTailscaleForFunnel()
	ctx, cancel := context.WithTimeout(a.baseContext(), 12*time.Second)
	defer cancel()
	state = tailscale.FunnelStatus(ctx, status.BinaryPath, status.MagicDNS, id, instance.Name, instance.Port)
	state.Available = state.Available && status.Running
	if !status.Running {
		if state.Error == "" {
			state.Error = status.Error
		}
		return state, nil
	}
	if state.Error == "" {
		a.syncFunnelState(instance, state)
	}
	return state, nil
}

// EnableFunnel publishes the database on the internet through Tailscale Funnel.
func (a *App) EnableFunnel(request FunnelRequest) (tailscale.FunnelState, error) {
	state := tailscale.FunnelState{
		InstanceID:   request.ID,
		AllowedPorts: append([]int(nil), tailscale.FunnelAllowedPorts...),
	}
	if err := a.ready(); err != nil {
		state.Error = err.Error()
		return state, err
	}
	manager, _, _ := a.components()
	instance, ok := manager.Get(request.ID)
	if !ok {
		err := fmt.Errorf("database instance %q was not found", request.ID)
		state.Error = err.Error()
		return state, err
	}
	if instance.Status != config.StatusRunning {
		err := fmt.Errorf("start %s before enabling public Funnel hosting", instance.Name)
		state.Error = err.Error()
		return state, err
	}
	return a.enableFunnelForInstance(instance, request.Port)
}

// DisableFunnel removes the public internet mapping for this database.
func (a *App) DisableFunnel(id string) (tailscale.FunnelState, error) {
	state := tailscale.FunnelState{
		InstanceID:   id,
		AllowedPorts: append([]int(nil), tailscale.FunnelAllowedPorts...),
	}
	if err := a.ready(); err != nil {
		state.Error = err.Error()
		return state, err
	}
	manager, _, _ := a.components()
	instance, ok := manager.Get(id)
	if !ok {
		err := fmt.Errorf("database instance %q was not found", id)
		state.Error = err.Error()
		return state, err
	}
	status := a.currentTailscaleForFunnel()
	if status.BinaryPath == "" {
		err := errors.New("Tailscale CLI is unavailable; cannot change Funnel hosting")
		state.Error = err.Error()
		return state, err
	}
	ctx, cancel := context.WithTimeout(a.baseContext(), 20*time.Second)
	defer cancel()
	state, err := tailscale.DisableFunnel(ctx, status.BinaryPath, status.MagicDNS, id, instance.Name, instance.Port, instance.FunnelPort)
	state.Available = true
	if err != nil {
		state.Error = err.Error()
		return state, err
	}
	state.Enabled = false
	state.Port = 0
	state.PublicURL = ""
	state.JDBCURL = ""
	if _, stateErr := manager.SetFunnelState(id, false, 0, ""); stateErr != nil {
		state.Error = stateErr.Error()
		return state, stateErr
	}
	a.emit("funnel:state", state)
	return state, nil
}

func (a *App) enableFunnelForInstance(instance config.Instance, port int) (tailscale.FunnelState, error) {
	if port <= 0 {
		port = tailscale.FunnelDefaultPort
	}
	state := tailscale.FunnelState{
		InstanceID:   instance.ID,
		AllowedPorts: append([]int(nil), tailscale.FunnelAllowedPorts...),
	}
	manager, store, _ := a.components()
	if manager == nil || store == nil {
		err := errors.New("HySQL backend is not initialized")
		state.Error = err.Error()
		return state, err
	}
	ctx, cancel := context.WithTimeout(a.baseContext(), 45*time.Second)
	defer cancel()
	status := tailscale.Detect(ctx, store.Settings().TailscalePath)
	a.updateTailscaleStatus(status)
	if !status.Running {
		message := status.Error
		if message == "" {
			message = "Tailscale is not ready; open Tailscale and sign in."
		}
		state.Available = status.BinaryPath != ""
		state.Error = message
		return state, errors.New(message)
	}
	if status.BinaryPath == "" {
		err := errors.New("Tailscale CLI was not found")
		state.Error = err.Error()
		return state, err
	}
	if status.MagicDNS == "" {
		err := errors.New("Tailscale MagicDNS is unavailable; reconnect Tailscale and refresh")
		state.Error = err.Error()
		return state, err
	}

	state, err := tailscale.EnableFunnel(ctx, status.BinaryPath, status.MagicDNS, instance.ID, instance.Name, instance.Port, port)
	state.Available = true
	if err != nil {
		state.Error = err.Error()
		return state, err
	}
	if _, stateErr := manager.SetFunnelState(instance.ID, true, state.Port, state.Host); stateErr != nil {
		state.Error = stateErr.Error()
		return state, stateErr
	}
	a.emit("funnel:state", state)
	return state, nil
}

func (a *App) disableFunnelForInstance(instance config.Instance) error {
	if !instance.FunnelEnabled && instance.FunnelPort <= 0 {
		return nil
	}
	status := a.currentTailscaleForFunnel()
	if status.BinaryPath == "" {
		if instance.FunnelEnabled {
			return errors.New("Tailscale CLI is unavailable; disable public Funnel hosting before continuing")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := tailscale.DisableFunnel(ctx, status.BinaryPath, status.MagicDNS, instance.ID, instance.Name, instance.Port, instance.FunnelPort); err != nil {
		return fmt.Errorf("disable public Funnel hosting before continuing: %w", err)
	}
	manager, _, _ := a.components()
	if manager != nil {
		if _, err := manager.SetFunnelState(instance.ID, false, 0, ""); err != nil {
			return err
		}
	}
	a.emit("funnel:state", tailscale.FunnelState{
		InstanceID:   instance.ID,
		Available:    true,
		AllowedPorts: append([]int(nil), tailscale.FunnelAllowedPorts...),
	})
	return nil
}

func (a *App) cleanupFunnelForInstanceID(instanceID string) {
	manager, _, _ := a.components()
	if manager == nil {
		return
	}
	instance, ok := manager.Get(instanceID)
	if !ok {
		return
	}
	_ = a.disableFunnelForInstance(instance)
}

func (a *App) reconcileFunnels() {
	manager, _, _ := a.components()
	if manager == nil {
		return
	}
	for _, instance := range manager.List() {
		if !instance.FunnelEnabled && instance.FunnelPort <= 0 {
			continue
		}
		if instance.Status != config.StatusRunning {
			_ = a.disableFunnelForInstance(instance)
			continue
		}
		_, _ = a.GetFunnelState(instance.ID)
	}
}

func (a *App) syncFunnelState(instance config.Instance, state tailscale.FunnelState) {
	host := state.Host
	if !state.Enabled {
		host = ""
	}
	if state.Enabled == instance.FunnelEnabled &&
		state.Port == instance.FunnelPort &&
		host == instance.FunnelHost {
		return
	}
	manager, _, _ := a.components()
	if manager == nil {
		return
	}
	if _, err := manager.SetFunnelState(instance.ID, state.Enabled, state.Port, host); err != nil {
		return
	}
	a.emit("funnel:state", state)
}

func (a *App) currentTailscaleForFunnel() tailscale.Status {
	status := a.tailscaleStatus()
	if status.BinaryPath != "" {
		return status
	}
	_, store, _ := a.components()
	if store == nil {
		return status
	}
	ctx, cancel := context.WithTimeout(a.baseContext(), tailscaleCheck)
	defer cancel()
	refreshed := tailscale.Detect(ctx, store.Settings().TailscalePath)
	a.updateTailscaleStatus(refreshed)
	return refreshed
}

// GetJavaCodegen renders connectivity artifacts for local or Tailscale mode.
func (a *App) GetJavaCodegen(request JavaCodegenRequest) (codegen.Artifacts, error) {
	if err := a.ready(); err != nil {
		return codegen.Artifacts{}, err
	}
	manager, _, hosts := a.components()
	instance, ok := manager.Get(request.ID)
	if !ok {
		return codegen.Artifacts{}, fmt.Errorf("database instance %q was not found", request.ID)
	}
	mode := codegen.Mode(strings.ToLower(strings.TrimSpace(request.Mode)))
	renderRequest := codegen.Request{
		InstanceID: instance.ID,
		Mode:       mode,
		Database:   instance.Name,
		Username:   instance.Username,
		Password:   instance.Password,
		Available:  true,
	}
	switch mode {
	case codegen.ModeLocal:
		renderRequest.Host = "127.0.0.1"
		renderRequest.Port = instance.Port
	case codegen.ModeTailscale:
		renderRequest.Host = instance.TailscaleIP
		renderRequest.Port = instance.TailscalePort
		status := a.tailscaleStatus()
		renderRequest.MagicDNS = status.MagicDNS
		switch {
		case !instance.TailscaleEnabled || instance.TailscaleIP == "" || instance.TailscalePort <= 0:
			renderRequest.Available = false
			renderRequest.Reason = "Tailscale hosting is off. Enable it from the overview to generate a remote JDBC URL."
		case !status.Running:
			renderRequest.Available = false
			renderRequest.Reason = "Tailscale is not connected. Reconnect and refresh before generating a remote URL."
		case status.IPv4 != "" && status.IPv4 != instance.TailscaleIP:
			renderRequest.Available = false
			renderRequest.Reason = "The Tailscale address changed since hosting was enabled. Turn hosting off and on again."
		case hosts != nil && !hosts.Snapshot(instance.ID).Running:
			renderRequest.Available = false
			renderRequest.Reason = "The Tailscale proxy is not running. Toggle hosting off and on again."
		}
	default:
		return codegen.Artifacts{}, fmt.Errorf("unknown connection mode %q; use \"local\" or \"tailscale\"", request.Mode)
	}
	return codegen.Generate(renderRequest), nil
}

// ExecuteQuery runs one SQL statement against a running instance.
func (a *App) ExecuteQuery(request QueryRequest) (QueryResult, error) {
	if err := a.ready(); err != nil {
		return QueryResult{}, err
	}
	defer a.markDatabaseActivity(request.ID)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	startedAt := time.Now()
	result, err := runner.Execute(ctx, request.ID, request.SQL)
	if err != nil {
		return QueryResult{}, err
	}
	if operation, schema, table := mutationTarget(request.SQL); operation != "" {
		a.emitRealtimeChange(RealtimeChangeEvent{
			InstanceID:  request.ID,
			Schema:      schema,
			Table:       table,
			Operation:   operation,
			Source:      "sql-workbench",
			UpdateCount: result.UpdateCount,
		})
	}
	columns := result.Columns
	if columns == nil {
		columns = []jdbc.Column{}
	}
	rows := result.Rows
	if rows == nil {
		rows = [][]*string{}
	}
	return QueryResult{
		Columns:          columns,
		Rows:             rows,
		RowCount:         result.RowCount,
		UpdateCount:      result.UpdateCount,
		HasResultSet:     result.HasResultSet,
		Truncated:        result.Truncated,
		EngineDurationMS: result.DurationMS,
		DurationMS:       time.Since(startedAt).Milliseconds(),
		ExecutedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ListTables reads the user schema of a running instance.
func (a *App) ListTables(id string) (jdbc.SchemaSnapshot, error) {
	if err := a.ready(); err != nil {
		return jdbc.SchemaSnapshot{}, err
	}
	defer a.markDatabaseActivity(id)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	return runner.Tables(ctx, id)
}

// GetTableRows reads one page of rows for the table editor.
func (a *App) GetTableRows(request TableRowsRequest) (TableRowsResult, error) {
	if err := a.ready(); err != nil {
		return TableRowsResult{}, err
	}
	defer a.markDatabaseActivity(request.ID)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	result, err := runner.Rows(ctx, sqlrunner.TableRowsRequest{
		InstanceID: request.ID,
		Schema:     request.Schema,
		Table:      request.Table,
		Limit:      request.Limit,
		Offset:     request.Offset,
		OrderBy:    request.OrderBy,
		Descending: request.Descending,
	})
	if err != nil {
		return TableRowsResult{}, err
	}
	return TableRowsResult{
		Columns: result.Columns,
		Rows:    result.Rows,
		HasMore: result.HasMore,
		Limit:   result.Limit,
		Offset:  result.Offset,
	}, nil
}

// InsertTableRow adds one row through the table editor.
func (a *App) InsertTableRow(request TableMutationRequest) (TableMutationResult, error) {
	if err := a.ready(); err != nil {
		return TableMutationResult{}, err
	}
	defer a.markDatabaseActivity(request.ID)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	count, err := runner.Insert(ctx, mutationFromRequest(request))
	if err != nil {
		return TableMutationResult{}, err
	}
	a.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID:  request.ID,
		Schema:      request.Schema,
		Table:       request.Table,
		Operation:   "insert",
		Source:      "table-editor",
		UpdateCount: count,
	})
	return TableMutationResult{UpdateCount: count}, nil
}

// UpdateTableRow updates the row selected by its primary key.
func (a *App) UpdateTableRow(request TableMutationRequest) (TableMutationResult, error) {
	if err := a.ready(); err != nil {
		return TableMutationResult{}, err
	}
	defer a.markDatabaseActivity(request.ID)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	count, err := runner.Update(ctx, mutationFromRequest(request))
	if err != nil {
		return TableMutationResult{}, err
	}
	a.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID:  request.ID,
		Schema:      request.Schema,
		Table:       request.Table,
		Operation:   "update",
		Source:      "table-editor",
		UpdateCount: count,
	})
	return TableMutationResult{UpdateCount: count}, nil
}

// DeleteTableRow deletes the row selected by its primary key.
func (a *App) DeleteTableRow(request TableMutationRequest) (TableMutationResult, error) {
	if err := a.ready(); err != nil {
		return TableMutationResult{}, err
	}
	defer a.markDatabaseActivity(request.ID)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	count, err := runner.Delete(ctx, mutationFromRequest(request))
	if err != nil {
		return TableMutationResult{}, err
	}
	a.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID:  request.ID,
		Schema:      request.Schema,
		Table:       request.Table,
		Operation:   "delete",
		Source:      "table-editor",
		UpdateCount: count,
	})
	return TableMutationResult{UpdateCount: count}, nil
}

// DropTable removes a table and all of its rows through the table editor.
func (a *App) DropTable(request TableDropRequest) error {
	if err := a.ready(); err != nil {
		return err
	}
	defer a.markDatabaseActivity(request.ID)
	manager, _, _ := a.components()
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	runner := sqlrunner.New(manager, manager.Bridge())
	if err := runner.DropTable(ctx, request.ID, request.Schema, request.Table); err != nil {
		return err
	}
	a.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID: request.ID,
		Schema:     request.Schema,
		Table:      request.Table,
		Operation:  "ddl",
		Source:     "table-editor",
	})
	return nil
}

func mutationFromRequest(request TableMutationRequest) sqlrunner.RowMutation {
	return sqlrunner.RowMutation{
		InstanceID: request.ID,
		Schema:     request.Schema,
		Table:      request.Table,
		Values:     request.Values,
		Key:        request.Key,
	}
}

// GetInstanceLog returns the tail of the H2 server log.
func (a *App) GetInstanceLog(request LogRequest) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	manager, _, _ := a.components()
	return manager.ReadLog(request.ID, request.Lines)
}

func (a *App) toView(instance config.Instance) InstanceView {
	views := a.instanceViews([]config.Instance{instance})
	if len(views) == 0 {
		return InstanceView{}
	}
	return views[0]
}

func inactiveProxySnapshot(id string) tailscale.Snapshot {
	return tailscale.Snapshot{InstanceID: id, Clients: []tailscale.ClientSnapshot{}}
}
