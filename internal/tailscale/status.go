// Package tailscale detects the local Tailscale daemon and exposes a
// loopback-to-tailnet TCP proxy. It never binds wildcard addresses.
package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tailsql/internal/executil"
)

// Status is the UI-facing Tailscale health snapshot.
type Status struct {
	Installed    bool   `json:"installed"`
	Running      bool   `json:"running"`
	BackendState string `json:"backendState"`
	IPv4         string `json:"ipv4"`
	MagicDNS     string `json:"magicDns"`
	Hostname     string `json:"hostname"`
	PeerCount    int    `json:"peerCount"`
	BinaryPath   string `json:"binaryPath"`
	Error        string `json:"error"`
	CheckedAt    string `json:"checkedAt"`
}

// IsTailscaleIPv4 reports whether ip belongs to the CGNAT range Tailscale uses:
// 100.64.0.0/10.
func IsTailscaleIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}

type rawStatus struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName      string   `json:"DNSName"`
		HostName     string   `json:"HostName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
	Peer map[string]json.RawMessage `json:"Peer"`
}

// Detect runs `tailscale status --json` using an optional path override.
func Detect(ctx context.Context, override string) Status {
	checkedAt := time.Now().UTC()
	status := Status{CheckedAt: checkedAt.Format(time.RFC3339)}

	binary, err := resolveBinary(override)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Installed = true
	status.BinaryPath = binary

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
	}

	cmd := executil.CommandContext(ctx, binary, "status", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		status.Error = fmt.Sprintf("`tailscale status --json` failed: %v", err)
		if detail != "" {
			status.Error += ": " + truncate(detail, 400)
		}
		status.CheckedAt = time.Now().UTC().Format(time.RFC3339)
		return status
	}

	parsed := parseStatus(stdout.Bytes(), binary, checkedAt)
	return parsed
}

func parseStatus(data []byte, binary string, checkedAt time.Time) Status {
	status := Status{Installed: true, BinaryPath: binary, CheckedAt: checkedAt.UTC().Format(time.RFC3339)}
	var raw rawStatus
	if err := json.Unmarshal(data, &raw); err != nil {
		status.Error = "Could not parse Tailscale status output: " + err.Error()
		return status
	}

	status.BackendState = raw.BackendState
	status.Running = raw.BackendState == "Running"
	status.MagicDNS = strings.TrimSuffix(strings.TrimSpace(raw.Self.DNSName), ".")
	status.Hostname = strings.TrimSpace(raw.Self.HostName)
	if status.Hostname == "" && status.MagicDNS != "" {
		status.Hostname = strings.SplitN(status.MagicDNS, ".", 2)[0]
	}
	status.PeerCount = len(raw.Peer)

	for _, candidate := range raw.Self.TailscaleIPs {
		ip := net.ParseIP(strings.TrimSpace(candidate))
		if ip != nil && IsTailscaleIPv4(ip) {
			status.IPv4 = ip.To4().String()
			break
		}
	}

	if !status.Running {
		state := raw.BackendState
		if state == "" {
			state = "unknown"
		}
		status.Error = fmt.Sprintf("Tailscale is not connected (backend state: %s). Open Tailscale and sign in.", state)
		return status
	}
	if status.IPv4 == "" {
		status.Error = "Tailscale is running but no 100.64.0.0/10 IPv4 address was reported."
	}
	return status
}

func resolveBinary(override string) (string, error) {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		if fileExists(trimmed) {
			return trimmed, nil
		}
		return "", fmt.Errorf("configured Tailscale path does not exist: %s", trimmed)
	}
	if found, err := exec.LookPath("tailscale"); err == nil {
		return found, nil
	}
	for _, candidate := range binaryCandidates() {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("Tailscale CLI not found. Install Tailscale and ensure `tailscale` is on PATH, or set settings.tailscalePath in ~/.tailsql/instances.json")
}

func binaryCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		var candidates []string
		for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if root := os.Getenv(variable); root != "" {
				candidates = append(candidates, filepath.Join(root, "Tailscale", "tailscale.exe"))
			}
		}
		candidates = append(candidates,
			`C:\Program Files\Tailscale\tailscale.exe`,
			`C:\Program Files (x86)\Tailscale\tailscale.exe`,
		)
		return candidates
	case "darwin":
		return []string{
			"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
			"/usr/local/bin/tailscale",
			"/opt/homebrew/bin/tailscale",
			"/usr/bin/tailscale",
		}
	default:
		return []string{
			"/usr/bin/tailscale",
			"/usr/local/bin/tailscale",
			"/snap/bin/tailscale",
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
