package dbmanager

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"tailsql/internal/config"
	"tailsql/internal/executil"
	"tailsql/internal/jdbc"
)

const (
	startupTimeout     = 20 * time.Second
	shutdownTimeout    = 10 * time.Second
	processStopTimeout = 7 * time.Second
)

// Start launches the H2 TCP server for an instance and waits until the
// database is reachable with the stored credentials.
func (m *Manager) Start(id string) (config.Instance, error) {
	m.mu.Lock()
	instance, ok := m.store.Get(id)
	if !ok {
		m.mu.Unlock()
		return config.Instance{}, fmt.Errorf("database instance %q was not found", id)
	}
	if instance.Status == config.StatusRunning {
		if _, running := m.runtimes[id]; running {
			m.mu.Unlock()
			return m.enrich(instance), nil
		}
	}
	if instance.Status == config.StatusStarting || instance.Status == config.StatusStopping {
		m.mu.Unlock()
		return config.Instance{}, fmt.Errorf("database %s is %s; wait for the current operation to finish", instance.Name, instance.Status)
	}

	java, h2 := m.java, m.h2
	if !java.Supported {
		m.mu.Unlock()
		return config.Instance{}, errors.New(java.Error)
	}
	if !h2.Available {
		m.mu.Unlock()
		return config.Instance{}, errors.New(h2.Error)
	}
	if instance.ServerPassword == "" {
		generated, err := GenerateServerPassword()
		if err != nil {
			m.mu.Unlock()
			return config.Instance{}, err
		}
		instance.ServerPassword = generated
	}

	// A previous app instance may have left an H2 server holding the database
	// file even though its PID was never persisted (or was cleared by a failed
	// start). Adopt a healthy server or stop the stale one before launching a
	// second server against the same file.
	if adoptedPID, adopted, reclaimErr := m.reclaimOrphanH2(instance, java, h2); reclaimErr != nil {
		m.mu.Unlock()
		return config.Instance{}, reclaimErr
	} else if adopted {
		startedAt := time.Now().UTC()
		instance.Status = config.StatusRunning
		instance.PID = adoptedPID
		instance.StartedAt = &startedAt
		instance.LastError = ""
		instance.TailscaleEnabled = false
		instance.UpdatedAt = startedAt
		if err := m.store.Update(instance); err != nil {
			m.mu.Unlock()
			return config.Instance{}, err
		}
		m.runtimes[id] = &runtimeState{adopted: true, startedAt: startedAt}
		m.mu.Unlock()
		m.emit()
		go m.watchAdopted(id, adoptedPID)
		current, _ := m.Get(id)
		return current, nil
	}

	now := time.Now().UTC()
	instance.Status = config.StatusStarting
	instance.PID = 0
	instance.StartedAt = nil
	instance.LastError = ""
	instance.TailscaleEnabled = false
	instance.UpdatedAt = now
	if err := m.store.Update(instance); err != nil {
		m.mu.Unlock()
		return config.Instance{}, err
	}
	m.mu.Unlock()
	m.emit()

	if err := m.runServer(instance, java, h2); err != nil {
		failure := m.describeStartFailure(id, err)
		m.failStart(id, java, h2, failure)
		current, _ := m.Get(id)
		return current, failure
	}

	current, _ := m.Get(id)
	return current, nil
}

func (m *Manager) runServer(instance config.Instance, java JavaInfo, h2 H2Info) error {
	m.mu.Lock()
	latest, ok := m.store.Get(instance.ID)
	if !ok || latest.Status != config.StatusStarting {
		m.mu.Unlock()
		return errors.New("startup was canceled")
	}
	m.mu.Unlock()

	logFile, err := os.OpenFile(m.LogPath(instance.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	_, _ = fmt.Fprintf(logFile, "\n===== TailSQL start %s =====\nport: %d\ndatabase: %s\nbaseDir: %s\n",
		timestamp, instance.Port, instance.DatabaseFile, m.dbDir)

	ctx, cancel := context.WithCancel(context.Background())
	args := []string{
		"-Dh2.bindAddress=127.0.0.1",
		"-Dfile.encoding=UTF-8",
		"-cp", h2.Path,
		"org.h2.tools.Server",
		"-tcp",
		"-tcpPort", strconv.Itoa(instance.Port),
		"-tcpPassword", instance.ServerPassword,
		"-baseDir", m.dbDir,
		"-ifNotExists",
	}
	cmd := executil.CommandContext(ctx, java.Path, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		cancel()
		_ = logFile.Close()
		return fmt.Errorf("start H2 server: %w", err)
	}

	done := make(chan struct{})
	startedAt := time.Now().UTC()
	runtime := &runtimeState{cmd: cmd, cancel: cancel, done: done, startedAt: startedAt}

	m.mu.Lock()
	m.runtimes[instance.ID] = runtime
	current, _ := m.store.Get(instance.ID)
	current.PID = cmd.Process.Pid
	current.UpdatedAt = time.Now().UTC()
	updateErr := m.store.Update(current)
	m.mu.Unlock()
	if updateErr != nil {
		cancel()
		_ = killProcessTree(cmd.Process.Pid)
		_ = logFile.Close()
		return updateErr
	}
	m.emit()
	go m.waitProcess(instance.ID, cmd, done, logFile)

	if err := waitForPort("127.0.0.1", instance.Port, startupTimeout, done); err != nil {
		return err
	}
	if err := m.pingInstance(instance, java, h2); err != nil {
		return fmt.Errorf("H2 opened port %d but the database handshake failed: %w", instance.Port, err)
	}

	m.mu.Lock()
	current, ok = m.store.Get(instance.ID)
	if !ok || current.Status != config.StatusStarting {
		m.mu.Unlock()
		return errors.New("startup was canceled")
	}
	select {
	case <-done:
		m.mu.Unlock()
		return errors.New("H2 process exited during startup")
	default:
	}
	finishedAt := time.Now().UTC()
	current.Status = config.StatusRunning
	current.PID = cmd.Process.Pid
	current.LastError = ""
	current.StartedAt = &finishedAt
	current.TailscaleEnabled = false
	current.UpdatedAt = finishedAt
	err = m.store.Update(current)
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("persist running state: %w", err)
	}
	m.emit()
	return nil
}

func (m *Manager) waitProcess(id string, cmd *exec.Cmd, done chan struct{}, logFile *os.File) {
	waitErr := cmd.Wait()
	close(done)
	_ = logFile.Close()

	m.mu.Lock()
	if runtime, ok := m.runtimes[id]; ok && runtime.cmd == cmd {
		delete(m.runtimes, id)
	}
	instance, found := m.store.Get(id)
	unexpected := false
	message := ""
	if found && instance.Status == config.StatusRunning {
		unexpected = true
		message = "H2 server exited unexpectedly"
		if waitErr != nil {
			message += ": " + waitErr.Error()
		}
		instance.Status = config.StatusError
		instance.PID = 0
		instance.StartedAt = nil
		instance.TailscaleEnabled = false
		instance.LastError = message
		instance.UpdatedAt = time.Now().UTC()
		_ = m.store.Update(instance)
	}
	m.mu.Unlock()

	if unexpected {
		m.emit()
		m.notifyError(id, message)
	}
}

func (m *Manager) pingInstance(instance config.Instance, java JavaInfo, h2 H2Info) error {
	bridge := &jdbc.Bridge{JavaPath: java.Path, JarPath: h2.Path, ClassDir: m.bridge.ClassDir}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := bridge.Ping(ctx, LocalJDBCURL(instance.Name, instance.Port), instance.Username, instance.Password)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(400 * time.Millisecond)
	}
	return lastErr
}

func (m *Manager) failStart(id string, java JavaInfo, h2 H2Info, cause error) {
	m.mu.Lock()
	instance, exists := m.store.Get(id)
	runtime := m.runtimes[id]
	m.mu.Unlock()
	if !exists {
		return
	}

	if runtime != nil {
		_ = m.shutdownServer(java, h2, instance)
		if !waitChannel(runtime.done, 5*time.Second) {
			runtime.cancel()
			if !waitChannel(runtime.done, 3*time.Second) && runtime.cmd.Process != nil {
				_ = killProcessTree(runtime.cmd.Process.Pid)
			}
		}
		m.mu.Lock()
		delete(m.runtimes, id)
		m.mu.Unlock()
	}

	m.mu.Lock()
	fresh, ok := m.store.Get(id)
	if ok && fresh.Status == config.StatusStarting {
		fresh.Status = config.StatusError
		fresh.PID = 0
		fresh.StartedAt = nil
		fresh.TailscaleEnabled = false
		fresh.LastError = cause.Error()
		fresh.UpdatedAt = time.Now().UTC()
		_ = m.store.Update(fresh)
	}
	m.mu.Unlock()
	m.emit()
	m.notifyError(id, cause.Error())
}

// Stop shuts down an instance, first asking H2 to close cleanly, then falling
// back to force termination for processes TailSQL owns.
func (m *Manager) Stop(id string) (config.Instance, error) {
	m.mu.Lock()
	instance, ok := m.store.Get(id)
	if !ok {
		m.mu.Unlock()
		return config.Instance{}, fmt.Errorf("database instance %q was not found", id)
	}
	if instance.Status == config.StatusStopped && instance.PID == 0 {
		m.mu.Unlock()
		return m.enrich(instance), nil
	}
	if instance.Status == config.StatusStarting {
		m.mu.Unlock()
		return instance, errors.New("database is still starting; wait for it to finish before stopping")
	}
	if instance.Status == config.StatusStopping {
		m.mu.Unlock()
		return instance, errors.New("database is already stopping")
	}
	java, h2 := m.java, m.h2
	runtime := m.runtimes[id]
	pid := instance.PID

	instance.Status = config.StatusStopping
	instance.UpdatedAt = time.Now().UTC()
	if err := m.store.Update(instance); err != nil {
		m.mu.Unlock()
		return config.Instance{}, err
	}
	m.mu.Unlock()
	m.emit()

	var stopErrors []string
	alive := runtime != nil || (pid > 0 && processExists(pid))
	if alive {
		if err := m.shutdownServer(java, h2, instance); err != nil {
			stopErrors = append(stopErrors, err.Error())
		}
	}

	if runtime != nil && !runtime.adopted {
		if !waitChannel(runtime.done, processStopTimeout) {
			if runtime.cancel != nil {
				runtime.cancel()
			}
			if !waitChannel(runtime.done, 2*time.Second) {
				if runtime.cmd != nil && runtime.cmd.Process != nil {
					_ = killProcessTree(runtime.cmd.Process.Pid)
				}
				if !waitChannel(runtime.done, 2*time.Second) {
					stopErrors = append(stopErrors, "H2 process did not exit after termination")
				}
			}
		}
	} else if pid > 0 && !waitProcessExit(pid, processStopTimeout) {
		details := inspectProcess(pid)
		if details.Alive && looksLikeH2CommandLine(details.CommandLine, instance.Port) {
			if err := killProcessTree(pid); err != nil {
				stopErrors = append(stopErrors, err.Error())
			} else {
				_ = waitProcessExit(pid, 2*time.Second)
			}
		} else if details.Alive {
			stopErrors = append(stopErrors, fmt.Sprintf("process %d is still running but could not be verified as TailSQL's H2 server; not terminating it", pid))
		}
	}

	m.mu.Lock()
	delete(m.runtimes, id)
	fresh, exists := m.store.Get(id)
	stillAlive := false
	if exists {
		stillAlive = pid > 0 && processExists(pid)
		fresh.TailscaleEnabled = false
		fresh.UpdatedAt = time.Now().UTC()
		if stillAlive {
			fresh.Status = config.StatusError
			fresh.PID = pid
			if len(stopErrors) > 0 {
				fresh.LastError = strings.Join(stopErrors, "; ")
			} else {
				fresh.LastError = "H2 process is still running"
			}
		} else {
			fresh.Status = config.StatusStopped
			fresh.PID = 0
			fresh.StartedAt = nil
			if len(stopErrors) > 0 {
				fresh.LastError = strings.Join(stopErrors, "; ")
			} else {
				fresh.LastError = ""
			}
		}
		_ = m.store.Update(fresh)
	}
	m.mu.Unlock()
	m.emit()

	current, _ := m.Get(id)
	if stillAlive {
		return current, errors.New(current.LastError)
	}
	if len(stopErrors) > 0 {
		return current, errors.New(strings.Join(stopErrors, "; "))
	}
	return current, nil
}

// Restart stops and starts an instance.
func (m *Manager) Restart(id string) (config.Instance, error) {
	if _, err := m.Stop(id); err != nil {
		return config.Instance{}, err
	}
	return m.Start(id)
}

// Delete stops (if needed) and removes instance metadata. Database files are
// intentionally retained.
func (m *Manager) Delete(id string) error {
	instance, ok := m.store.Get(id)
	if !ok {
		return fmt.Errorf("database instance %q was not found", id)
	}
	if instance.Status != config.StatusStopped || instance.PID != 0 {
		if _, err := m.Stop(id); err != nil {
			return fmt.Errorf("stop before delete: %w", err)
		}
	}
	m.mu.Lock()
	err := m.store.Delete(id)
	delete(m.runtimes, id)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	m.emit()
	return nil
}

// StopAll is used during application shutdown. It attempts graceful shutdowns,
// then force-kills any process handles that remain.
func (m *Manager) StopAll(ctx context.Context) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
	}
	for _, instance := range m.store.List() {
		if ctx.Err() != nil {
			break
		}
		if instance.Status == config.StatusStopped && instance.PID == 0 {
			continue
		}
		_, _ = m.Stop(instance.ID)
	}

	m.mu.Lock()
	var confirmedStopped []string
	for id, runtime := range m.runtimes {
		if runtime.adopted {
			// The process may still be alive and we cannot reliably stop it in
			// the shutdown budget. Leave the persisted running state so the next
			// launch can adopt and stop it through the verified path.
			continue
		}
		pid := 0
		if runtime.cmd != nil && runtime.cmd.Process != nil {
			pid = runtime.cmd.Process.Pid
		}
		if runtime.cancel != nil {
			runtime.cancel()
		}
		if runtime.cmd != nil && runtime.cmd.Process != nil {
			_ = killProcessTree(runtime.cmd.Process.Pid)
		}
		delete(m.runtimes, id)
		if pid > 0 && processExists(pid) {
			instance, ok := m.store.Get(id)
			if ok {
				instance.LastError = "Process did not stop during application shutdown; it will be reconciled on the next launch."
				instance.UpdatedAt = time.Now().UTC()
				_ = m.store.Update(instance)
			}
			continue
		}
		confirmedStopped = append(confirmedStopped, id)
	}
	for _, id := range confirmedStopped {
		instance, ok := m.store.Get(id)
		if !ok {
			continue
		}
		instance.Status = config.StatusStopped
		instance.PID = 0
		instance.StartedAt = nil
		instance.TailscaleEnabled = false
		instance.UpdatedAt = time.Now().UTC()
		_ = m.store.Update(instance)
	}
	m.mu.Unlock()
}

// Reconcile detects whether persisted PIDs from a previous session still belong
// to TailSQL's H2 servers. Valid processes are adopted; stale ones are cleaned
// up without touching unrelated Java processes.
func (m *Manager) Reconcile(ctx context.Context) {
	java, h2 := m.Java(), m.H2()
	for _, stored := range m.store.List() {
		adopt := false
		adoptedPID := stored.PID
		if stored.PID > 0 {
			details := inspectProcess(stored.PID)
			if details.Alive && isJavaImage(details.Image) {
				commandLineKnown := strings.TrimSpace(details.CommandLine) != ""
				verified := looksLikeH2CommandLine(details.CommandLine, stored.Port)
				listening := tcpReady("127.0.0.1", stored.Port)
				switch {
				case listening && (verified || !commandLineKnown):
					// The PID is a Java process on the expected port; adopt it.
					adopt = true
				case verified && !listening:
					// Verified as TailSQL's H2 server but not serving: clean it up.
					if stored.ServerPassword != "" {
						_ = m.shutdownServer(java, h2, stored)
						_ = waitProcessExit(stored.PID, 4*time.Second)
					}
					if remaining := inspectProcess(stored.PID); remaining.Alive {
						_ = killProcessTree(stored.PID)
					}
				}
			}
		}

		// The persisted PID is not authoritative: a crash or failed start may
		// leave an H2 server that still holds the database file. Find it by
		// its command line, adopt it when healthy, or stop it when it is
		// wedged so the next start can open the database.
		var orphanErr error
		if !adopt {
			pid, orphanAdopted, err := m.reclaimOrphanH2(stored, java, h2)
			if err != nil {
				orphanErr = err
			} else if orphanAdopted {
				adopt = true
				adoptedPID = pid
			}
		}

		m.mu.Lock()
		fresh, ok := m.store.Get(stored.ID)
		if !ok {
			m.mu.Unlock()
			continue
		}
		fresh.TailscaleEnabled = false
		fresh.UpdatedAt = time.Now().UTC()
		if adopt {
			alreadyRunning := fresh.Status == config.StatusRunning
			fresh.Status = config.StatusRunning
			fresh.PID = adoptedPID
			fresh.LastError = ""
			if fresh.StartedAt == nil {
				started := fresh.UpdatedAt
				fresh.StartedAt = &started
			}
			m.runtimes[stored.ID] = &runtimeState{adopted: true, startedAt: fresh.UpdatedAt}
			if fresh.ServerPassword == "" {
				if generated, err := GenerateServerPassword(); err == nil {
					fresh.ServerPassword = generated
				}
			}
			_ = m.store.Update(fresh)
			m.mu.Unlock()
			if !alreadyRunning {
				go m.watchAdopted(stored.ID, adoptedPID)
			}
			continue
		}

		fresh.Status = config.StatusStopped
		fresh.PID = 0
		fresh.StartedAt = nil
		if orphanErr != nil {
			fresh.LastError = orphanErr.Error()
		} else {
			fresh.LastError = ""
		}
		delete(m.runtimes, stored.ID)
		_ = m.store.Update(fresh)
		m.mu.Unlock()
	}
	m.emit()
}

func (m *Manager) watchAdopted(id string, pid int) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.Lock()
		instance, ok := m.store.Get(id)
		stillOurs := ok && instance.Status == config.StatusRunning && instance.PID == pid
		if !stillOurs {
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()

		if processExists(pid) {
			continue
		}
		m.mu.Lock()
		delete(m.runtimes, id)
		if current, exists := m.store.Get(id); exists && current.PID == pid {
			current.Status = config.StatusError
			current.PID = 0
			current.StartedAt = nil
			current.TailscaleEnabled = false
			current.LastError = "Adopted H2 process exited; start the database again"
			current.UpdatedAt = time.Now().UTC()
			_ = m.store.Update(current)
		}
		m.mu.Unlock()
		m.emit()
		m.notifyError(id, "Adopted H2 process exited; start the database again")
		return
	}
}

func (m *Manager) shutdownServer(java JavaInfo, h2 H2Info, instance config.Instance) error {
	if !java.Supported {
		return errors.New("Java is unavailable; cannot request graceful H2 shutdown")
	}
	if !h2.Available {
		return errors.New("H2 jar is unavailable; cannot request graceful H2 shutdown")
	}
	if instance.ServerPassword == "" {
		return errors.New("instance has no H2 shutdown secret")
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	cmd := executil.CommandContext(ctx, java.Path,
		"-cp", h2.Path,
		"org.h2.tools.Server",
		"-tcpShutdown", fmt.Sprintf("tcp://127.0.0.1:%d", instance.Port),
		"-tcpPassword", instance.ServerPassword,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("H2 shutdown request failed: %v (%s)", err, truncateOutput(string(output), 300))
	}
	return nil
}

func (m *Manager) describeStartFailure(id string, cause error) error {
	tail, err := m.ReadLog(id, 12)
	if err != nil || strings.TrimSpace(tail) == "" {
		return cause
	}
	return fmt.Errorf("%w — server log: %s", cause, truncateOutput(tail, 1200))
}

func waitForPort(host string, port int, timeout time.Duration, done <-chan struct{}) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(host, strconv.Itoa(port))
	for {
		select {
		case <-done:
			return errors.New("H2 process exited during startup")
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("H2 did not open %s within %s", address, timeout)
		}
		connection, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func waitChannel(channel <-chan struct{}, timeout time.Duration) bool {
	if channel == nil {
		return false
	}
	select {
	case <-channel:
		return true
	case <-time.After(timeout):
		return false
	}
}

func waitProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return !processExists(pid)
}

func tcpReady(host string, port int) bool {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}
