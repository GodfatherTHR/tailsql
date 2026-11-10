package dbmanager

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

const (
	// FirstLocalPort is where local H2 TCP listeners begin.
	FirstLocalPort = 9092
	// FirstTailscalePort is where Tailscale proxy listeners begin.
	FirstTailscalePort = 19092
	// PortScanAttempts bounds port allocation to avoid unbounded loops.
	PortScanAttempts = 200
)

// ValidatePort checks the user-visible TCP port range.
func ValidatePort(port int) error {
	if port < 1024 || port > 65535 {
		return errors.New("port must be between 1024 and 65535")
	}
	return nil
}

// PortAvailable reports whether host:port can be bound right now.
func PortAvailable(host string, port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// FindAvailablePort scans upward from start on the given interface.
func FindAvailablePort(host string, start, attempts int) (int, error) {
	if start < 1 || start > 65535 {
		return 0, fmt.Errorf("invalid starting port %d", start)
	}
	if attempts <= 0 {
		attempts = PortScanAttempts
	}
	for port := start; port < start+attempts && port <= 65535; port++ {
		if PortAvailable(host, port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available TCP port found in range %d-%d", start, start+attempts-1)
}

// LocalJDBCURL builds the loopback JDBC URL for an instance.
func LocalJDBCURL(name string, port int) string {
	return fmt.Sprintf("jdbc:h2:tcp://127.0.0.1:%d/./%s", port, name)
}
