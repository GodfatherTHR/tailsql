//go:build !windows

package dbmanager

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"hysql/internal/executil"
)

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
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
	output, err := executil.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=,args=").Output()
	if err != nil {
		return processDetails{}
	}
	line := strings.TrimSpace(string(output))
	if line == "" {
		return processDetails{}
	}
	parts := strings.SplitN(line, " ", 2)
	details := processDetails{Alive: true, Image: parts[0]}
	if len(parts) > 1 {
		details.CommandLine = strings.TrimSpace(parts[1])
	}
	return details
}

func listJavaProcessIDs() []int {
	output, err := executil.Command("ps", "-eo", "pid=,comm=").Output()
	if err != nil {
		return nil
	}
	pids := make([]int, 0, 8)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !isJavaImage(fields[1]) {
			continue
		}
		pid, parseErr := strconv.Atoi(fields[0])
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
	output, err := executil.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(line))
		if parseErr == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill process %d: %w", pid, err)
	}
	return nil
}
