package tailscale

import (
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIsTailscaleIPv4(t *testing.T) {
	cases := []struct {
		address string
		want    bool
	}{
		{"100.64.0.1", true},
		{"100.76.208.54", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false},
		{"100.128.0.1", false},
		{"192.168.1.10", false},
		{"127.0.0.1", false},
		{"0.0.0.0", false},
		{"fd7a:115c:a1e0::1", false},
	}
	for _, testCase := range cases {
		ip := net.ParseIP(testCase.address)
		if got := IsTailscaleIPv4(ip); got != testCase.want {
			t.Errorf("IsTailscaleIPv4(%s) = %v, want %v", testCase.address, got, testCase.want)
		}
	}
	if IsTailscaleIPv4(nil) {
		t.Error("nil IP must not be considered a Tailscale address")
	}
}

func TestParseStatusRunning(t *testing.T) {
	payload := map[string]any{
		"BackendState": "Running",
		"Self": map[string]any{
			"DNSName":      "workstation.tailnet-abc.ts.net.",
			"HostName":     "workstation",
			"TailscaleIPs": []string{"fd7a:115c:a1e0::1", "100.76.208.54"},
		},
		"Peer": map[string]any{"peer-1": map[string]any{}, "peer-2": map[string]any{}},
	}
	raw, _ := json.Marshal(payload)
	status := parseStatus(raw, "tailscale", time.Now().UTC())
	if !status.Running {
		t.Fatalf("expected running status, got %+v", status)
	}
	if status.IPv4 != "100.76.208.54" {
		t.Fatalf("unexpected IPv4: %q", status.IPv4)
	}
	if status.MagicDNS != "workstation.tailnet-abc.ts.net" {
		t.Fatalf("MagicDNS not normalized: %q", status.MagicDNS)
	}
	if status.Hostname != "workstation" {
		t.Fatalf("hostname mismatch: %q", status.Hostname)
	}
	if status.PeerCount != 2 {
		t.Fatalf("peer count mismatch: %d", status.PeerCount)
	}
	if status.Error != "" {
		t.Fatalf("unexpected error: %s", status.Error)
	}
}

func TestParseStatusNotRunning(t *testing.T) {
	raw := []byte(`{"BackendState":"NeedsLogin","Self":{"TailscaleIPs":[]}}`)
	status := parseStatus(raw, "tailscale", time.Now().UTC())
	if status.Running {
		t.Fatal("NeedsLogin must not be reported as running")
	}
	if !strings.Contains(status.Error, "NeedsLogin") {
		t.Fatalf("expected actionable backend state in error, got %q", status.Error)
	}
}

func TestParseStatusRejectsNonTailscaleIPv4(t *testing.T) {
	raw := []byte(`{"BackendState":"Running","Self":{"DNSName":"host.tail.ts.net.","TailscaleIPs":["192.168.1.20","100.64.9.9"]}}`)
	status := parseStatus(raw, "tailscale", time.Now().UTC())
	if status.IPv4 != "100.64.9.9" {
		t.Fatalf("expected CGNAT address, got %q", status.IPv4)
	}
}

func TestParseStatusMalformed(t *testing.T) {
	status := parseStatus([]byte("{nope"), "tailscale", time.Now().UTC())
	if status.Running || status.Error == "" {
		t.Fatalf("expected parse error, got %+v", status)
	}
}

func TestValidateOptionsGuardrails(t *testing.T) {
	base := ProxyOptions{
		InstanceID: "abc",
		BindHost:   "100.76.208.54",
		Port:       19092,
		TargetHost: "127.0.0.1",
		TargetPort: 9092,
	}
	if err := validateOptions(base); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}

	wildcard := base
	wildcard.BindHost = "0.0.0.0"
	if err := validateOptions(wildcard); err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("wildcard bind must be rejected, got %v", err)
	}

	lan := base
	lan.BindHost = "192.168.1.5"
	if err := validateOptions(lan); err == nil {
		t.Fatal("non-Tailscale bind must be rejected")
	}

	loopback := base
	loopback.BindHost = "127.0.0.1"
	if err := validateOptions(loopback); err == nil {
		t.Fatal("loopback bind must be rejected outside tests")
	}
	loopback.AllowLoopback = true
	if err := validateOptions(loopback); err != nil {
		t.Fatalf("loopback test override rejected: %v", err)
	}

	badTarget := base
	badTarget.TargetHost = "10.0.0.5"
	if err := validateOptions(badTarget); err == nil {
		t.Fatal("non-loopback target must be rejected")
	}
}

func TestProxyForwardsAndTracksConnections(t *testing.T) {
	echoAddress, echoPort := startEchoServer(t)

	var mu sync.Mutex
	latest := Snapshot{}
	startSnapshot := make(chan struct{}, 1)
	proxy, err := StartProxy(ProxyOptions{
		InstanceID:    "test-instance",
		BindHost:      "127.0.0.1",
		Port:          0,
		TargetHost:    "127.0.0.1",
		TargetPort:    echoPort,
		AllowLoopback: true,
	}, func(snapshot Snapshot) {
		mu.Lock()
		latest = snapshot
		mu.Unlock()
		select {
		case startSnapshot <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("StartProxy: %v", err)
	}
	defer proxy.Stop()

	snapshot := proxy.Snapshot()
	if snapshot.Port == 0 {
		t.Fatal("expected an allocated proxy port")
	}
	if snapshot.BindAddress == "" || strings.HasPrefix(snapshot.BindAddress, "0.0.0.0") {
		t.Fatalf("unexpected bind address: %q", snapshot.BindAddress)
	}
	target := net.JoinHostPort("127.0.0.1", strconv.Itoa(snapshot.Port))
	connection, err := net.DialTimeout("tcp", target, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := connection.Write([]byte("hello-through-proxy")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buffer := make([]byte, 64)
	read, err := connection.Read(buffer)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buffer[:read]) != "hello-through-proxy" {
		t.Fatalf("echo mismatch: %q", string(buffer[:read]))
	}

	waitForCondition(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return latest.ActiveClients == 1 && latest.Clients[0].PeerIP == "127.0.0.1" && latest.BytesIn > 0 && latest.BytesOut > 0
	})
	_ = connection.Close()
	waitForCondition(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return latest.ActiveClients == 0 && latest.TotalConnections == 1 && latest.BytesIn > 0 && latest.BytesOut > 0
	})

	proxy.Stop()
	if final := proxy.Snapshot(); final.Running {
		t.Fatal("proxy still reports running after Stop")
	}
	_ = echoAddress
}

func TestProxyRejectsWildcardAndBusyPort(t *testing.T) {
	_, echoPort := startEchoServer(t)
	_, err := StartProxy(ProxyOptions{
		InstanceID: "bad",
		BindHost:   "0.0.0.0",
		Port:       19092,
		TargetHost: "127.0.0.1",
		TargetPort: echoPort,
	}, nil)
	if err == nil {
		t.Fatal("expected wildcard bind rejection")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()
	busyPort := listener.Addr().(*net.TCPAddr).Port
	_, err = StartProxy(ProxyOptions{
		InstanceID:    "busy",
		BindHost:      "127.0.0.1",
		Port:          busyPort,
		TargetHost:    "127.0.0.1",
		TargetPort:    echoPort,
		AllowLoopback: true,
	}, nil)
	if err == nil {
		t.Fatal("expected busy port error")
	}
	if !IsAddressInUse(err) {
		t.Fatalf("expected address-in-use classification, got %v", err)
	}
}

func TestHostManagerReturnsInactiveSnapshot(t *testing.T) {
	manager := NewHostManager(nil)
	snapshot := manager.Snapshot("missing")
	if snapshot.Running || snapshot.InstanceID != "missing" || snapshot.Clients == nil {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func startEchoServer(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start echo server: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(connection)
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	return listener.Addr().String(), address.Port
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
