package tailscale

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ClientSnapshot describes one active proxied TCP connection.
type ClientSnapshot struct {
	ID          string `json:"id"`
	PeerIP      string `json:"peerIp"`
	ConnectedAt string `json:"connectedAt"`
	BytesIn     int64  `json:"bytesIn"`
	BytesOut    int64  `json:"bytesOut"`
}

// Snapshot is the live state of one instance's Tailscale proxy.
type Snapshot struct {
	InstanceID       string           `json:"instanceId"`
	Running          bool             `json:"running"`
	BindAddress      string           `json:"bindAddress"`
	Port             int              `json:"port"`
	TargetAddress    string           `json:"targetAddress"`
	ActiveClients    int              `json:"activeClients"`
	TotalConnections int64            `json:"totalConnections"`
	BytesIn          int64            `json:"bytesIn"`
	BytesOut         int64            `json:"bytesOut"`
	StartedAt        string           `json:"startedAt,omitempty"`
	Clients          []ClientSnapshot `json:"clients"`
	LastError        string           `json:"lastError,omitempty"`
}

// ProxyOptions configures one Tailscale-facing TCP listener.
type ProxyOptions struct {
	InstanceID    string
	BindHost      string
	Port          int
	TargetHost    string
	TargetPort    int
	AllowLoopback bool
}

type trackedConn struct {
	id          string
	peerIP      string
	connectedAt time.Time
	client      net.Conn
	target      net.Conn
	bytesIn     atomic.Int64
	bytesOut    atomic.Int64
}

// Proxy forwards a Tailscale-bound TCP listener to the loopback H2 server.
type Proxy struct {
	opts       ProxyOptions
	listener   net.Listener
	onSnapshot func(Snapshot)

	mu               sync.Mutex
	clients          map[string]*trackedConn
	running          bool
	lastError        string
	startedAt        time.Time
	totalConnections atomic.Int64
	totalIn          atomic.Int64
	totalOut         atomic.Int64
	sequence         atomic.Uint64

	stopped  chan struct{}
	stopOnce sync.Once
	workers  sync.WaitGroup
	handlers sync.WaitGroup
}

// StartProxy validates the bind address, starts listening, and begins accepting.
func StartProxy(opts ProxyOptions, onSnapshot func(Snapshot)) (*Proxy, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	address := net.JoinHostPort(opts.BindHost, strconv.Itoa(opts.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	if tcpAddress, ok := listener.Addr().(*net.TCPAddr); ok {
		opts.Port = tcpAddress.Port
	}

	proxy := &Proxy{
		opts:       opts,
		listener:   listener,
		onSnapshot: onSnapshot,
		clients:    map[string]*trackedConn{},
		running:    true,
		startedAt:  time.Now().UTC(),
		stopped:    make(chan struct{}),
	}
	proxy.workers.Add(2)
	go proxy.acceptLoop()
	go proxy.metricsLoop()
	proxy.emit()
	return proxy, nil
}

func validateOptions(opts ProxyOptions) error {
	if strings.TrimSpace(opts.InstanceID) == "" {
		return errors.New("proxy requires an instance id")
	}
	bindIP := net.ParseIP(strings.TrimSpace(opts.BindHost))
	if bindIP == nil || bindIP.To4() == nil {
		return fmt.Errorf("bind host %q must be an IPv4 literal", opts.BindHost)
	}
	if bindIP.IsUnspecified() {
		return errors.New("wildcard binding (0.0.0.0) is forbidden; bind to the Tailscale interface")
	}
	if bindIP.IsLoopback() {
		if !opts.AllowLoopback {
			return errors.New("loopback proxy binding is only permitted in tests")
		}
	} else if !IsTailscaleIPv4(bindIP) {
		return fmt.Errorf("bind host %s is not a Tailscale 100.64.0.0/10 address", bindIP)
	}
	if opts.Port < 0 || opts.Port > 65535 {
		return fmt.Errorf("invalid proxy port %d", opts.Port)
	}
	if opts.TargetPort < 1 || opts.TargetPort > 65535 {
		return fmt.Errorf("invalid target port %d", opts.TargetPort)
	}
	targetIP := net.ParseIP(strings.TrimSpace(opts.TargetHost))
	if targetIP == nil || !targetIP.IsLoopback() {
		return errors.New("proxy target must be a loopback address")
	}
	return nil
}

func (p *Proxy) acceptLoop() {
	defer p.workers.Done()
	for {
		connection, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.stopped:
				return
			default:
			}
			p.setLastError(fmt.Sprintf("accept failed: %v", err))
			p.emit()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		p.handlers.Add(1)
		go p.handle(connection)
	}
}

func (p *Proxy) handle(client net.Conn) {
	defer p.handlers.Done()
	peerIP := ""
	if address, ok := client.RemoteAddr().(*net.TCPAddr); ok {
		peerIP = address.IP.String()
	}

	targetAddress := net.JoinHostPort(p.opts.TargetHost, strconv.Itoa(p.opts.TargetPort))
	target, err := net.DialTimeout("tcp", targetAddress, 5*time.Second)
	if err != nil {
		p.setLastError(fmt.Sprintf("dial local H2 server at %s: %v", targetAddress, err))
		_ = client.Close()
		p.emit()
		return
	}

	tracked := &trackedConn{
		id:          strconv.FormatUint(p.sequence.Add(1), 10),
		peerIP:      peerIP,
		connectedAt: time.Now().UTC(),
		client:      client,
		target:      target,
	}

	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		_ = client.Close()
		_ = target.Close()
		return
	}
	p.clients[tracked.id] = tracked
	p.totalConnections.Add(1)
	p.mu.Unlock()
	p.emit()

	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		_, _ = io.Copy(&countingWriter{writer: target, total: &p.totalIn, own: &tracked.bytesIn}, client)
		closeWrite(target)
	}()
	go func() {
		defer copies.Done()
		_, _ = io.Copy(&countingWriter{writer: client, total: &p.totalOut, own: &tracked.bytesOut}, target)
		closeWrite(client)
	}()
	copies.Wait()

	_ = client.Close()
	_ = target.Close()

	p.mu.Lock()
	delete(p.clients, tracked.id)
	p.mu.Unlock()
	p.emit()
}

func (p *Proxy) metricsLoop() {
	defer p.workers.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopped:
			return
		case <-ticker.C:
			if snapshot := p.Snapshot(); snapshot.ActiveClients > 0 {
				p.deliver(snapshot)
			}
		}
	}
}

// Stop closes the listener and any active forwarded connections.
func (p *Proxy) Stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.running = false
		close(p.stopped)
		_ = p.listener.Close()
		for _, tracked := range p.clients {
			_ = tracked.client.Close()
			_ = tracked.target.Close()
		}
		p.mu.Unlock()

		p.handlers.Wait()
		p.workers.Wait()
		p.emit()
	})
}

// Snapshot returns a consistent copy of the proxy's live state.
func (p *Proxy) Snapshot() Snapshot {
	p.mu.Lock()
	clients := make([]ClientSnapshot, 0, len(p.clients))
	for _, tracked := range p.clients {
		clients = append(clients, ClientSnapshot{
			ID:          tracked.id,
			PeerIP:      tracked.peerIP,
			ConnectedAt: tracked.connectedAt.UTC().Format(time.RFC3339),
			BytesIn:     tracked.bytesIn.Load(),
			BytesOut:    tracked.bytesOut.Load(),
		})
	}
	running := p.running
	lastError := p.lastError
	started := p.startedAt
	p.mu.Unlock()

	snapshot := Snapshot{
		InstanceID:       p.opts.InstanceID,
		Running:          running,
		BindAddress:      net.JoinHostPort(p.opts.BindHost, strconv.Itoa(p.opts.Port)),
		Port:             p.opts.Port,
		TargetAddress:    net.JoinHostPort(p.opts.TargetHost, strconv.Itoa(p.opts.TargetPort)),
		ActiveClients:    len(clients),
		TotalConnections: p.totalConnections.Load(),
		BytesIn:          p.totalIn.Load(),
		BytesOut:         p.totalOut.Load(),
		StartedAt:        started.UTC().Format(time.RFC3339),
		Clients:          clients,
		LastError:        lastError,
	}
	return snapshot
}

func (p *Proxy) setLastError(message string) {
	p.mu.Lock()
	p.lastError = message
	p.mu.Unlock()
}

func (p *Proxy) emit() {
	p.deliver(p.Snapshot())
}

func (p *Proxy) deliver(snapshot Snapshot) {
	if p.onSnapshot != nil {
		p.onSnapshot(snapshot)
	}
}

func closeWrite(connection net.Conn) {
	if tcp, ok := connection.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}

type countingWriter struct {
	writer io.Writer
	total  *atomic.Int64
	own    *atomic.Int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	if written > 0 {
		w.total.Add(int64(written))
		w.own.Add(int64(written))
	}
	return written, err
}

// HostManager owns all per-instance proxies.
type HostManager struct {
	mu         sync.Mutex
	proxies    map[string]*Proxy
	onSnapshot func(Snapshot)
}

// NewHostManager builds a registry that forwards snapshots to onSnapshot.
func NewHostManager(onSnapshot func(Snapshot)) *HostManager {
	return &HostManager{proxies: map[string]*Proxy{}, onSnapshot: onSnapshot}
}

// Start creates or returns the proxy for an instance.
func (h *HostManager) Start(opts ProxyOptions) (Snapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if existing, ok := h.proxies[opts.InstanceID]; ok {
		snapshot := existing.Snapshot()
		if snapshot.Running {
			return snapshot, nil
		}
		delete(h.proxies, opts.InstanceID)
	}

	proxy, err := StartProxy(opts, func(snapshot Snapshot) {
		if h.onSnapshot != nil {
			h.onSnapshot(snapshot)
		}
	})
	if err != nil {
		return Snapshot{InstanceID: opts.InstanceID, Clients: []ClientSnapshot{}}, err
	}
	h.proxies[opts.InstanceID] = proxy
	return proxy.Snapshot(), nil
}

// Stop closes one instance's proxy.
func (h *HostManager) Stop(instanceID string) {
	h.mu.Lock()
	proxy := h.proxies[instanceID]
	delete(h.proxies, instanceID)
	h.mu.Unlock()
	if proxy != nil {
		proxy.Stop()
	}
}

// StopAll closes every proxy.
func (h *HostManager) StopAll() {
	h.mu.Lock()
	proxies := make([]*Proxy, 0, len(h.proxies))
	for _, proxy := range h.proxies {
		proxies = append(proxies, proxy)
	}
	h.proxies = map[string]*Proxy{}
	h.mu.Unlock()
	for _, proxy := range proxies {
		proxy.Stop()
	}
}

// Snapshot returns the proxy state, or an inactive snapshot when absent.
func (h *HostManager) Snapshot(instanceID string) Snapshot {
	h.mu.Lock()
	proxy := h.proxies[instanceID]
	h.mu.Unlock()
	if proxy != nil {
		return proxy.Snapshot()
	}
	return Snapshot{InstanceID: instanceID, Clients: []ClientSnapshot{}}
}

// IsAddressInUse reports whether a listen error is a port conflict.
func IsAddressInUse(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address already in use") ||
		strings.Contains(message, "only one usage of each socket address") ||
		strings.Contains(message, "address in use")
}
