package tailscale

import "testing"

func TestForwardTargetPort(t *testing.T) {
	cases := []struct {
		input string
		port  int
		ok    bool
	}{
		{"127.0.0.1:9092", 9092, true},
		{"tcp://127.0.0.1:9092", 9092, true},
		{"tcp://localhost:10000", 10000, true},
		{"[::1]:9092", 9092, true},
		{"unix:/tmp/h2.sock", 0, false},
		{"", 0, false},
		{"127.0.0.1:notaport", 0, false},
	}
	for _, testCase := range cases {
		port, ok := forwardTargetPort(testCase.input)
		if ok != testCase.ok || port != testCase.port {
			t.Fatalf("forwardTargetPort(%q) = (%d, %v), want (%d, %v)",
				testCase.input, port, ok, testCase.port, testCase.ok)
		}
	}
}

func TestFindFunnelPort(t *testing.T) {
	status := funnelStatus{
		TCP: map[string]funnelTCPHandler{
			"443":   {HTTPS: true},
			"8443":  {TCPForward: "127.0.0.1:9999"},
			"10000": {TCPForward: "tcp://127.0.0.1:9092"},
		},
		AllowFunnel: map[string]bool{
			"desktop.tail6cb521.ts.net:443":   true,
			"desktop.tail6cb521.ts.net:10000": true,
		},
	}
	port, ok := findFunnelPort(status, "desktop.tail6cb521.ts.net", 9092)
	if !ok || port != 10000 {
		t.Fatalf("findFunnelPort = (%d, %v), want (10000, true)", port, ok)
	}

	if _, ok := findFunnelPort(status, "desktop.tail6cb521.ts.net", 9999); ok {
		t.Fatal("port 8443 is not an allowed Funnel port and must not match")
	}

	status.AllowFunnel["desktop.tail6cb521.ts.net:10000"] = false
	if _, ok := findFunnelPort(status, "desktop.tail6cb521.ts.net", 9092); ok {
		t.Fatal("a TCPForward without AllowFunnel is Serve-private, not Funnel")
	}
}

func TestFunnelAllowedFor(t *testing.T) {
	status := funnelStatus{AllowFunnel: map[string]bool{
		"desktop.tail6cb521.ts.net:10000": true,
		"other.tail6cb521.ts.net:8443":    true,
	}}
	if !funnelAllowedFor(status, "desktop.tail6cb521.ts.net", 10000) {
		t.Fatal("expected the matching hostname and port to be allowed")
	}
	if funnelAllowedFor(status, "desktop.tail6cb521.ts.net", 8443) {
		t.Fatal("expected a different hostname to be rejected")
	}
	if !funnelAllowedFor(status, "", 8443) {
		t.Fatal("with no MagicDNS, the port alone should match")
	}
}

func TestAllowedFunnelPorts(t *testing.T) {
	for _, port := range FunnelAllowedPorts {
		if !isAllowedFunnelPort(port) {
			t.Fatalf("expected port %d to be allowed", port)
		}
	}
	for _, port := range []int{0, 80, 3000, 9999} {
		if isAllowedFunnelPort(port) {
			t.Fatalf("expected port %d to be rejected", port)
		}
	}
}

func TestFunnelURLs(t *testing.T) {
	if got := funnelPublicURL("desktop.tail6cb521.ts.net", 10000); got != "tcp://desktop.tail6cb521.ts.net:10000" {
		t.Fatalf("unexpected public URL: %s", got)
	}
	if got := funnelJDBCURL("desktop.tail6cb521.ts.net", 10000, "inventory_db"); got != "jdbc:h2:tcp://desktop.tail6cb521.ts.net:10000/./inventory_db" {
		t.Fatalf("unexpected JDBC URL: %s", got)
	}
	if got := funnelPublicURL("", 10000); got != "" {
		t.Fatalf("empty host should produce an empty URL, got %s", got)
	}
}
