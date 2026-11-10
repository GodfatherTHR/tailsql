package dbmanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tailsql/internal/config"
	"tailsql/internal/jdbc"
)

// findInstanceH2Processes finds Java H2 servers whose command line matches
// this instance's port, database directory, and shutdown secret. It is used to
// recover servers that were never persisted or whose PID was cleared by a
// failed start.
func (m *Manager) findInstanceH2Processes(instance config.Instance) []int {
	pids := listJavaProcessIDs()
	matches := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		details := inspectProcess(pid)
		if !details.Alive || !isJavaImage(details.Image) {
			continue
		}
		if !matchesInstanceH2(details.CommandLine, instance, m.dbDir) {
			continue
		}
		matches = append(matches, pid)
	}
	return matches
}

func matchesInstanceH2(commandLine string, instance config.Instance, dbDir string) bool {
	if strings.TrimSpace(commandLine) == "" {
		return false
	}
	if !looksLikeH2CommandLine(commandLine, instance.Port) {
		return false
	}
	if instance.ServerPassword != "" && !strings.Contains(commandLine, instance.ServerPassword) {
		return false
	}
	if dbDir != "" && !strings.Contains(strings.ToLower(commandLine), strings.ToLower(dbDir)) {
		return false
	}
	return true
}

// reclaimOrphanH2 adopts a healthy H2 server for the instance, or terminates
// stale servers that still hold the database file so a fresh start can open it.
func (m *Manager) reclaimOrphanH2(instance config.Instance, java JavaInfo, h2 H2Info) (int, bool, error) {
	pids := m.findInstanceH2Processes(instance)
	if len(pids) == 0 {
		return 0, false, nil
	}

	listening := tcpReady("127.0.0.1", instance.Port)
	if listening && m.pingInstanceOnce(instance, java, h2) == nil {
		adopted := pids[0]
		if listener := findListeningPID(instance.Port); listener > 0 && containsPID(pids, listener) {
			adopted = listener
		}
		// Multiple matches can be a Java launcher shim plus its JVM child, so
		// do not treat them as duplicates. Only the non-reachable path below
		// terminates every matching process.
		return adopted, true, nil
	}

	// The server is not reachable. Ask H2 to close cleanly when the port is
	// still open, then terminate anything that remains.
	if listening && instance.ServerPassword != "" {
		_ = m.shutdownServer(java, h2, instance)
		_ = waitProcessExit(pids[0], 4*time.Second)
	}
	var stopErrs []string
	for _, pid := range pids {
		if err := terminateH2Process(pid); err != nil {
			stopErrs = append(stopErrs, err.Error())
		}
	}
	if len(stopErrs) > 0 {
		return 0, false, fmt.Errorf("could not release the database file lock: %s", strings.Join(stopErrs, "; "))
	}
	return 0, false, nil
}

func (m *Manager) pingInstanceOnce(instance config.Instance, java JavaInfo, h2 H2Info) error {
	bridge := &jdbc.Bridge{JavaPath: java.Path, JarPath: h2.Path, ClassDir: m.bridge.ClassDir}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err := bridge.Ping(ctx, LocalJDBCURL(instance.Name, instance.Port), instance.Username, instance.Password)
	return err
}

func terminateH2Process(pid int) error {
	if pid <= 0 || !processExists(pid) {
		return nil
	}
	if err := killProcessTree(pid); err != nil {
		return fmt.Errorf("stop H2 process %d: %w", pid, err)
	}
	if !waitProcessExit(pid, 4*time.Second) {
		return fmt.Errorf("H2 process %d did not exit", pid)
	}
	return nil
}

func containsPID(pids []int, pid int) bool {
	for _, candidate := range pids {
		if candidate == pid {
			return true
		}
	}
	return false
}
