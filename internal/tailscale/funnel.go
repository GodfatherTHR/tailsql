package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"hysql/internal/executil"
)

// FunnelAllowedPorts are the only ports Tailscale Funnel accepts.
var FunnelAllowedPorts = []int{443, 8443, 10000}

// FunnelDefaultPort avoids the HTTPS Funnel ports commonly used by web apps.
const FunnelDefaultPort = 10000

// FunnelState is the UI-facing public-hosting snapshot for one database.
type FunnelState struct {
	InstanceID   string `json:"instanceId"`
	Available    bool   `json:"available"`
	Enabled      bool   `json:"enabled"`
	Port         int    `json:"port"`
	Host         string `json:"host"`
	PublicURL    string `json:"publicUrl"`
	JDBCURL      string `json:"jdbcUrl"`
	AllowedPorts []int  `json:"allowedPorts"`
	Error        string `json:"error"`
	CheckedAt    string `json:"checkedAt"`
}

type funnelTCPHandler struct {
	HTTPS        bool   `json:"HTTPS"`
	HTTP         bool   `json:"HTTP"`
	TCPForward   string `json:"TCPForward"`
	TerminateTLS string `json:"TerminateTLS"`
}

type funnelStatus struct {
	TCP         map[string]funnelTCPHandler `json:"TCP"`
	AllowFunnel map[string]bool             `json:"AllowFunnel"`
}

// FunnelStatus reads the current Funnel mapping for the database's loopback
// H2 port. It never mutates Tailscale configuration.
func FunnelStatus(ctx context.Context, binary, magicDNS, instanceID, databaseName string, h2Port int) FunnelState {
	state := baseFunnelState(instanceID, magicDNS, databaseName, h2Port)
	state.Available = strings.TrimSpace(binary) != ""
	if !state.Available {
		state.Error = "Tailscale CLI was not found."
		return state
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
	}

	status, err := readFunnelStatus(ctx, binary)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	if port, ok := findFunnelPort(status, magicDNS, h2Port); ok {
		state.Enabled = true
		state.Port = port
		state.PublicURL = funnelPublicURL(magicDNS, port)
		state.JDBCURL = funnelJDBCURL(magicDNS, port, databaseName)
	}
	return state
}

// EnableFunnel exposes the loopback H2 port through Tailscale Funnel on one of
// the allowed public ports.
func EnableFunnel(ctx context.Context, binary, magicDNS, instanceID, databaseName string, h2Port, port int) (FunnelState, error) {
	state := baseFunnelState(instanceID, magicDNS, databaseName, h2Port)
	state.Available = strings.TrimSpace(binary) != ""
	if !state.Available {
		return state, fmt.Errorf("Tailscale CLI was not found")
	}
	if !isAllowedFunnelPort(port) {
		return state, fmt.Errorf("Funnel only supports ports 443, 8443, and 10000 (requested %d)", port)
	}
	if h2Port < 1 || h2Port > 65535 {
		return state, fmt.Errorf("invalid local H2 port %d", h2Port)
	}
	if strings.TrimSpace(magicDNS) == "" {
		return state, fmt.Errorf("Tailscale MagicDNS name is unavailable; reconnect Tailscale and refresh")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	current, err := readFunnelStatus(ctx, binary)
	if err != nil {
		return state, err
	}
	if existingPort, ok := findFunnelPort(current, magicDNS, h2Port); ok {
		state.Enabled = true
		state.Port = existingPort
		state.PublicURL = funnelPublicURL(magicDNS, existingPort)
		state.JDBCURL = funnelJDBCURL(magicDNS, existingPort, databaseName)
		return state, nil
	}
	if handler, exists := current.TCP[strconv.Itoa(port)]; exists {
		if targetPort, ok := forwardTargetPort(handler.TCPForward); ok && targetPort == h2Port {
			return state, fmt.Errorf("port %d is already serving this database privately on your tailnet. Run `tailscale serve --tcp=%d off`, then enable Funnel again", port, port)
		}
		return state, fmt.Errorf("Funnel port %d is already in use by another Tailscale Serve or Funnel service", port)
	}

	args := []string{
		"funnel",
		"--bg",
		"--yes",
		fmt.Sprintf("--tcp=%d", port),
		fmt.Sprintf("tcp://127.0.0.1:%d", h2Port),
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
			return state, fmt.Errorf("enable Tailscale Funnel: %v: %s", err, truncate(detail, 800))
		}
		return state, fmt.Errorf("enable Tailscale Funnel: %w", err)
	}

	state.Enabled = true
	state.Port = port
	state.PublicURL = funnelPublicURL(magicDNS, port)
	state.JDBCURL = funnelJDBCURL(magicDNS, port, databaseName)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		confirmed, statusErr := readFunnelStatus(ctx, binary)
		if statusErr == nil {
			if confirmedPort, ok := findFunnelPort(confirmed, magicDNS, h2Port); ok {
				state.Port = confirmedPort
				state.PublicURL = funnelPublicURL(magicDNS, confirmedPort)
				state.JDBCURL = funnelJDBCURL(magicDNS, confirmedPort, databaseName)
				return state, nil
			}
		}
		select {
		case <-ctx.Done():
			state.Error = "Funnel was enabled, but status confirmation was interrupted. Click refresh."
			return state, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
	state.Error = "Funnel was enabled, but the mapping has not appeared in status yet. Click refresh to confirm."
	return state, nil
}

// DisableFunnel removes the Funnel mapping that targets the H2 loopback port.
func DisableFunnel(ctx context.Context, binary, magicDNS, instanceID, databaseName string, h2Port, port int) (FunnelState, error) {
	state := baseFunnelState(instanceID, magicDNS, databaseName, h2Port)
	state.Available = strings.TrimSpace(binary) != ""
	if !state.Available {
		return state, fmt.Errorf("Tailscale CLI was not found")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
	}

	current, err := readFunnelStatus(ctx, binary)
	if err != nil {
		return state, err
	}
	if port <= 0 {
		detected, ok := findFunnelPort(current, magicDNS, h2Port)
		if !ok {
			return state, nil
		}
		port = detected
	} else if handler, exists := current.TCP[strconv.Itoa(port)]; exists {
		targetPort, targetOK := forwardTargetPort(handler.TCPForward)
		if !targetOK || targetPort != h2Port {
			return state, fmt.Errorf("Funnel port %d does not target this database; refusing to remove it", port)
		}
		if !funnelAllowedFor(current, magicDNS, port) {
			return state, fmt.Errorf("port %d is served privately on your tailnet, not through Funnel; refusing to remove it", port)
		}
	}

	cmd := executil.CommandContext(ctx, binary,
		"funnel",
		fmt.Sprintf("--tcp=%d", port),
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
			return state, nil
		}
		if detail != "" {
			return state, fmt.Errorf("disable Tailscale Funnel: %v: %s", err, truncate(detail, 800))
		}
		return state, fmt.Errorf("disable Tailscale Funnel: %w", err)
	}
	return state, nil
}

func baseFunnelState(instanceID, magicDNS, databaseName string, h2Port int) FunnelState {
	return FunnelState{
		InstanceID:   instanceID,
		Host:         strings.TrimSuffix(strings.TrimSpace(magicDNS), "."),
		AllowedPorts: append([]int(nil), FunnelAllowedPorts...),
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func readFunnelStatus(ctx context.Context, binary string) (funnelStatus, error) {
	cmd := executil.CommandContext(ctx, binary, "funnel", "status", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if isNoServeConfig(detail) {
			return funnelStatus{}, nil
		}
		if detail != "" {
			return funnelStatus{}, fmt.Errorf("`tailscale funnel status` failed: %v: %s", err, truncate(detail, 600))
		}
		return funnelStatus{}, fmt.Errorf("`tailscale funnel status` failed: %w", err)
	}
	trimmed := bytes.TrimSpace(stdout.Bytes())
	if len(trimmed) == 0 || isNoServeConfig(string(trimmed)) {
		return funnelStatus{}, nil
	}
	var status funnelStatus
	if err := json.Unmarshal(trimmed, &status); err != nil {
		return funnelStatus{}, fmt.Errorf("could not parse Tailscale Funnel status: %w", err)
	}
	return status, nil
}

func isNoServeConfig(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "no serve config") ||
		strings.Contains(lower, "serve config does not exist")
}

func findFunnelPort(status funnelStatus, magicDNS string, h2Port int) (int, bool) {
	for portText, handler := range status.TCP {
		targetPort, ok := forwardTargetPort(handler.TCPForward)
		if !ok || targetPort != h2Port {
			continue
		}
		port, err := strconv.Atoi(portText)
		if err != nil || !isAllowedFunnelPort(port) {
			continue
		}
		if !funnelAllowedFor(status, magicDNS, port) {
			continue
		}
		return port, true
	}
	return 0, false
}

func funnelAllowedFor(status funnelStatus, magicDNS string, port int) bool {
	for key, allowed := range status.AllowFunnel {
		if !allowed {
			continue
		}
		host, portText, err := net.SplitHostPort(key)
		if err != nil {
			continue
		}
		keyPort, err := strconv.Atoi(portText)
		if err != nil || keyPort != port {
			continue
		}
		if strings.TrimSpace(magicDNS) == "" {
			return true
		}
		if strings.EqualFold(strings.TrimSuffix(host, "."), strings.TrimSuffix(magicDNS, ".")) {
			return true
		}
	}
	return false
}

func forwardTargetPort(forward string) (int, bool) {
	value := strings.TrimSpace(forward)
	if value == "" {
		return 0, false
	}
	for _, prefix := range []string{"tcp://", "tcp4://", "tcp6://", "tls-terminated-tcp://"} {
		value = strings.TrimPrefix(value, prefix)
	}
	_, portText, err := net.SplitHostPort(value)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func isAllowedFunnelPort(port int) bool {
	for _, allowed := range FunnelAllowedPorts {
		if port == allowed {
			return true
		}
	}
	return false
}

func funnelPublicURL(host string, port int) string {
	if strings.TrimSpace(host) == "" || port <= 0 {
		return ""
	}
	return fmt.Sprintf("tcp://%s:%d", host, port)
}

func funnelJDBCURL(host string, port int, databaseName string) string {
	if strings.TrimSpace(host) == "" || port <= 0 || strings.TrimSpace(databaseName) == "" {
		return ""
	}
	return fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s", host, port, databaseName)
}
