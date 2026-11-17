package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

func TestBrowserAllowsLoopbackByDefault(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv(PrivateNetworkEnv, "")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/form", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<h1>Local loopback page</h1>")
	})
	ipv4 := httptest.NewServer(handler)
	defer ipv4.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(ipv4.URL, "http://"))
	urls := []string{ipv4.URL, "http://localhost:" + port, ipv4.URL + "/redirect"}
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatal("IPv6 loopback listener: ", err)
	}
	ipv6 := httptest.NewUnstartedServer(handler)
	ipv6.Listener = listener
	ipv6.Start()
	defer ipv6.Close()
	urls = append(urls, ipv6.URL)
	service := New()
	defer service.Close()
	for _, address := range urls {
		t.Run(address, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			state, err := service.Open(ctx, "loopback", address)
			if err != nil {
				t.Fatal(err)
			}
			defer service.CloseSession(ctx, "loopback", state.ID)
			snapshot, err := service.Snapshot(ctx, "loopback", state.ID)
			if err != nil || !strings.Contains(snapshot.Content, "Local loopback page") {
				t.Fatalf("loopback snapshot: %v %s", err, snapshot.Content)
			}
			png, err := service.Screenshot(ctx, "loopback", state.ID, false)
			if err != nil || !strings.HasPrefix(string(png), "\x89PNG\r\n\x1a\n") {
				t.Fatalf("loopback screenshot: %v", err)
			}
		})
	}
}

func TestBrowserDefaultDestinationPolicy(t *testing.T) {
	policy := newEgressPolicy(false)
	policy.lookup = func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		switch host {
		case "localhost", "local.test":
			return []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, nil
		case "mixed.test":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("10.0.0.1")}, nil
		default:
			return nil, fmt.Errorf("unknown fixture host: %s", host)
		}
	}
	for _, address := range []string{"127.0.0.1:3000", "127.9.8.7:3000", "[::1]:3000", "[::ffff:127.0.0.1]:3000", "localhost:3000", "local.test:3000", "8.8.8.8:443"} {
		if _, _, err := policy.destination(context.Background(), address); err != nil {
			t.Errorf("allowed %s: %v", address, err)
		}
	}
	for _, address := range []string{"10.0.0.1:80", "192.168.1.1:80", "172.16.0.1:80", "169.254.169.254:80", "[fc00::1]:80", "0.0.0.0:80", "mixed.test:80"} {
		if _, _, err := policy.destination(context.Background(), address); !errors.Is(err, ErrEgressDenied) {
			t.Errorf("blocked %s: %v", address, err)
		}
	}
	policy.allowPrivate = true
	if _, _, err := policy.destination(context.Background(), "192.168.1.1:80"); err != nil {
		t.Fatalf("administrator private-network opt-in: %v", err)
	}
}

func TestBrowserPublicAddressPolicy(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "224.0.0.1", "198.18.0.1", "192.0.2.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::a00:1", "2002:7f00:1::", "2001:db8::1", "::ffff:0:7f00:1", "4000::1", "2001:20::1"} {
		if publicAddress(netip.MustParseAddr(address)) {
			t.Errorf("non-public address accepted: %s", address)
		}
	}
	for _, address := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicAddress(netip.MustParseAddr(address)) {
			t.Errorf("public address rejected: %s", address)
		}
	}
}

func TestBrowserProxyPinsDNSAndRejectsCONNECT(t *testing.T) {
	policy := newEgressPolicy(false)
	var calls atomic.Int32
	policy.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		if calls.Add(1) == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
	}
	var dialed []string
	policy.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		return nil, errors.New("test dial")
	}
	proxy, err := newEgressProxy(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	_, _ = proxy.dial(context.Background(), "tcp", "rebind.test:80")
	if len(dialed) != 1 || dialed[0] != "8.8.8.8:80" {
		t.Fatalf("hostname was dialed instead of checked IP: %v", dialed)
	}
	if _, err = proxy.dial(context.Background(), "tcp", "rebind.test:80"); !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("DNS rebinding accepted: %v", err)
	}
	address := strings.TrimPrefix(proxy.URL(), "http://")
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "CONNECT 169.254.169.254:443 HTTP/1.1\r\nHost: 169.254.169.254:443\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 502 || len(dialed) != 1 {
		t.Fatal("private CONNECT reached the network")
	}
}

func TestBrowserProxyBlocksRedirectsAndSubresources(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv(PrivateNetworkEnv, "false")
	var privateHits atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { privateHits.Add(1); w.Write([]byte("PRIVATE-MARKER")) }))
	defer private.Close()
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://private.test/redirect-target", 302)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<h1>Public page</h1><iframe src="%s/frame"></iframe><img src="%s/image"><script src="%s/script"></script><script>fetch('%s/fetch').catch(()=>{})</script>`, "http://private.test", "http://private.test", "http://private.test", "http://private.test")
	}))
	defer public.Close()
	s := New()
	defer s.Close()
	s.policy.lookup = func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		if host == "private.test" {
			return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
		}
		if host != "public.test" {
			return nil, errors.New("unknown test host")
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	s.policy.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		switch {
		case strings.HasPrefix(address, "8.8.8.8:"):
			return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(public.URL, "http://"))
		case strings.HasPrefix(address, "10.0.0.1:"):
			// If policy enforcement regresses, expose the attempted request to
			// the private fixture so the assertion below observes the breach.
			return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(private.URL, "http://"))
		default:
			t.Errorf("unchecked destination dialed: %s", address)
			return nil, ErrEgressDenied
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	state, err := s.Open(ctx, "egress", "http://public.test/")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Snapshot(ctx, "egress", state.ID)
	if err != nil || !strings.Contains(snapshot.Content, "Public page") {
		t.Fatalf("public page failed: %v %s", err, snapshot.Content)
	}
	_, _ = s.Run(ctx, "egress", state.ID, Command{Action: "navigate", URL: "http://public.test/redirect"})
	if privateHits.Load() != 0 {
		t.Fatalf("redirect or subresource bypassed egress policy: %d", privateHits.Load())
	}
}
