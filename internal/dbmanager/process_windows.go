//go:build windows

package dbmanager

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"hysql/internal/executil"
)

const stillActive = 259

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	const queryLimitedInformation = 0x1000
	handle, err := windows.OpenProcess(queryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return true
	}
	return exitCode == stillActive
}

type processDetails struct {
	Alive       bool
	Image       string
	CommandLine string
}

func inspectProcess(pid int) processDetails {
	if pid <= 0 {
		return processDetails{}
	}
	output, err := executil.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return processDetails{}
	}
	line := strings.TrimSpace(string(output))
	if line == "" || !strings.HasPrefix(line, "\"") {
		return processDetails{}
	}
	records, err := csv.NewReader(strings.NewReader(line)).Read()
	if err != nil || len(records) == 0 {
		return processDetails{}
	}
	details := processDetails{Alive: true, Image: records[0]}
	if strings.EqualFold(details.Image, "java.exe") {
		script := fmt.Sprintf("(Get-CimInstance Win32_Process -Filter \"ProcessId = %d\").CommandLine", pid)
		if cmdOut, cmdErr := executil.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output(); cmdErr == nil {
			details.CommandLine = strings.TrimSpace(string(cmdOut))
		}
	}
	return details
}

func listJavaProcessIDs() []int {
	output, err := executil.Command("tasklist", "/FI", "IMAGENAME eq java.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimSpace(string(output)))).ReadAll()
	if err != nil {
		return nil
	}
	pids := make([]int, 0, len(records))
	for _, record := range records {
		if len(record) < 2 {
			continue
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(record[1]))
		if parseErr == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

func findListeningPID(port int) int {
	if port <= 0 {
		return 0
	}
	script := fmt.Sprintf("(Get-NetTCPConnection -LocalPort %d -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1).OwningProcess", port)
	output, err := executil.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return 0
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if parseErr != nil {
		return 0
	}
	return pid
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	output, err := executil.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
