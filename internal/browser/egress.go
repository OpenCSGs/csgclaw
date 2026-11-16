package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sync"
	"time"
)

const PrivateNetworkEnv = "CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK"

var ErrEgressDenied = errors.New("browser destination is not public; private-network browsing requires administrator opt-in")

type egressPolicy struct {
	allowPrivate bool
	lookup       func(context.Context, string, string) ([]netip.Addr, error)
	dial         func(context.Context, string, string) (net.Conn, error)
}

func newEgressPolicy(allowPrivate bool) *egressPolicy {
	return &egressPolicy{allowPrivate: allowPrivate, lookup: net.DefaultResolver.LookupNetIP, dial: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext}
}

var nonPublicRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range nonPublicRanges {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
func (p *egressPolicy) destination(ctx context.Context, address string) ([]netip.Addr, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, "", fmt.Errorf("invalid browser destination: %w", err)
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		ips, err = p.lookup(ctx, "ip", host)
		if err != nil {
			return nil, "", fmt.Errorf("resolve browser destination: %w", err)
		}
	}
	if len(ips) == 0 {
		return nil, "", errors.New("browser destination has no addresses")
	}
	for _, ip := range ips {
		if !p.allowPrivate && !publicAddress(ip) {
			return nil, "", ErrEgressDenied
		}
	}
	return ips, port, nil
}
func (p *egressPolicy) checkURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	_, _, err = p.destination(ctx, net.JoinHostPort(u.Hostname(), port))
	return err
}

type egressProxy struct {
	policy      *egressPolicy
	listener    net.Listener
	server      *http.Server
	transport   *http.Transport
	mu          sync.Mutex
	closed      bool
	connections map[*egressConn]bool
}
type egressConn struct {
	net.Conn
	owner *egressProxy
	once  sync.Once
}

func (c *egressConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.owner.mu.Lock(); delete(c.owner.connections, c); c.owner.mu.Unlock() })
	return err
}
func (p *egressProxy) track(conn net.Conn) (net.Conn, error) {
	c := &egressConn{Conn: conn, owner: p}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		conn.Close()
		return nil, net.ErrClosed
	}
	p.connections[c] = true
	return c, nil
}
func newEgressProxy(ctx context.Context, policy *egressPolicy) (*egressProxy, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &egressProxy{policy: policy, listener: listener, connections: map[*egressConn]bool{}}
	p.transport = &http.Transport{Proxy: nil, DialContext: p.dial, DisableCompression: true, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second}
	forward := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.Out.Header.Del("Proxy-Authorization") }, Transport: p.transport, ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Browser destination blocked or unavailable", http.StatusBadGateway)
	}}
	p.server = &http.Server{ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			p.connect(w, r)
			return
		}
		if r.URL.Scheme != "http" || r.URL.Host == "" {
			http.Error(w, "Unsupported browser proxy request", 400)
			return
		}
		forward.ServeHTTP(w, r)
	})}
	go func() { _ = p.server.Serve(listener) }()
	return p, nil
}
func (p *egressProxy) URL() string { return "http://" + p.listener.Addr().String() }
func (p *egressProxy) dial(ctx context.Context, network, address string) (net.Conn, error) {
	ips, port, err := p.policy.destination(ctx, address)
	if err != nil {
		return nil, err
	}
	_, ownPort, _ := net.SplitHostPort(p.listener.Addr().String())
	for _, ip := range ips {
		if ip.Unmap().IsLoopback() && port == ownPort {
			return nil, ErrEgressDenied
		}
	}
	// Dial the checked IP, never the hostname: DNS cannot change between the
	// policy decision and the TCP connection. Every CONNECT and resource uses it.
	for _, ip := range ips {
		var conn net.Conn
		conn, err = p.policy.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return p.track(conn)
		}
	}
	return nil, err
}
func (p *egressProxy) connect(w http.ResponseWriter, r *http.Request) {
	upstream, err := p.dial(r.Context(), "tcp", r.Host)
	if err != nil {
		http.Error(w, "Browser destination blocked or unavailable", 502)
		return
	}
	defer upstream.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Proxy tunneling unavailable", 500)
		return
	}
	raw, buffer, err := hijacker.Hijack()
	if err != nil {
		return
	}
	client, err := p.track(raw)
	if err != nil {
		return
	}
	defer client.Close()
	if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err = buffer.Flush(); err != nil {
		return
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, buffer); _ = upstream.Close(); _ = client.Close(); close(done) }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	_ = upstream.Close()
	<-done
}
func (p *egressProxy) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	connections := make([]*egressConn, 0, len(p.connections))
	for c := range p.connections {
		connections = append(connections, c)
	}
	p.mu.Unlock()
	_ = p.server.Close()
	p.transport.CloseIdleConnections()
	for _, c := range connections {
		_ = c.Close()
	}
}
