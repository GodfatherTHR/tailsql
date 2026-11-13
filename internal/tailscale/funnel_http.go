package tailscale

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"hysql/internal/executil"
)

// HTTPFunnelState describes the public HTTPS mapping for the REST gateway.
type HTTPFunnelState struct {
	Available    bool   `json:"available"`
	Enabled      bool   `json:"enabled"`
	Port         int    `json:"port"`
	Host         string `json:"host"`
	PublicURL    string `json:"publicUrl"`
	AllowedPorts []int  `json:"allowedPorts"`
	Error        string `json:"error"`
	CheckedAt    string `json:"checkedAt"`
}

// IsAllowedFunnelPort reports whether Tailscale Funnel accepts the port.
func IsAllowedFunnelPort(port int) bool {
	return isAllowedFunnelPort(port)
}

// EnableHTTPFunnel publishes a loopback HTTP service over Tailscale Funnel.
// Unlike the H2 TCP funnel, this terminates TLS and forwards HTTP.
func EnableHTTPFunnel(ctx context.Context, binary, magicDNS string, localPort, publicPort int) (HTTPFunnelState, error) {
	state := baseHTTPFunnelState(magicDNS, publicPort)
	state.Available = strings.TrimSpace(binary) != ""
	if !state.Available {
		return state, fmt.Errorf("Tailscale CLI was not found")
	}
	if !isAllowedFunnelPort(publicPort) {
		return state, fmt.Errorf("Funnel only supports ports 443, 8443, and 10000 (requested %d)", publicPort)
	}
	if localPort < 1 || localPort > 65535 {
		return state, fmt.Errorf("invalid local REST port %d", localPort)
	}
	if strings.TrimSpace(magicDNS) == "" {
		return state, fmt.Errorf("Tailscale MagicDNS name is unavailable; reconnect Tailscale and refresh")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
	}
	args := []string{
		"funnel",
		"--bg",
		"--yes",
		fmt.Sprintf("--https=%d", publicPort),
		fmt.Sprintf("http://127.0.0.1:%d", localPort),
	}
	cmd := executil.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return state, fmt.Errorf("enable HTTP Funnel: %v: %s", err, truncate(detail, 800))
		}
		return state, fmt.Errorf("enable HTTP Funnel: %w", err)
	}
	state.Enabled = true
	state.PublicURL = httpFunnelURL(magicDNS, publicPort)
	return state, nil
}

// DisableHTTPFunnel removes the HTTPS mapping for the given public port.
func DisableHTTPFunnel(ctx context.Context, binary string, publicPort int) error {
	if strings.TrimSpace(binary) == "" {
		return fmt.Errorf("Tailscale CLI was not found")
	}
	if !isAllowedFunnelPort(publicPort) {
		return fmt.Errorf("invalid Funnel port %d", publicPort)
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
	}
	cmd := executil.CommandContext(ctx, binary,
		"funnel",
		fmt.Sprintf("--https=%d", publicPort),
		"--yes",
		"off",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if strings.Contains(strings.ToLower(detail), "serve config does not exist") {
			return nil
		}
		if detail != "" {
			return fmt.Errorf("disable HTTP Funnel: %v: %s", err, truncate(detail, 800))
		}
		return fmt.Errorf("disable HTTP Funnel: %w", err)
	}
	return nil
}

// HTTPFunnelURL renders the public base URL for a Funnel port.
func HTTPFunnelURL(host string, port int) string {
	return httpFunnelURL(host, port)
}

func baseHTTPFunnelState(magicDNS string, port int) HTTPFunnelState {
	return HTTPFunnelState{
		Port:         port,
		Host:         strings.TrimSuffix(strings.TrimSpace(magicDNS), "."),
		AllowedPorts: append([]int(nil), FunnelAllowedPorts...),
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func httpFunnelURL(host string, port int) string {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" || port <= 0 {
		return ""
	}
	if port == 443 {
		return "https://" + host
	}
	return "https://" + host + ":" + strconv.Itoa(port)
}
