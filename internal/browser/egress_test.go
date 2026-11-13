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

func TestBrowserBlocksPrivateNetworkByDefault(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv("CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK", "false")
	var hits atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("<h1>Private network fixture</h1>"))
	}))
	defer site.Close()
	service := New()
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := service.Open(ctx, "egress", site.URL); err == nil {
		t.Fatal("browser read a loopback service without private-network opt-in")
	}
	if hits.Load() != 0 {
		t.Fatalf("private service received %d requests", hits.Load())
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
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
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
			http.Redirect(w, r, private.URL+"/redirect-target", 302)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<h1>Public page</h1><iframe src="%s/frame"></iframe><img src="%s/image"><script src="%s/script"></script><script>fetch('%s/fetch').catch(()=>{})</script>`, private.URL, private.URL, private.URL, private.URL)
	}))
	defer public.Close()
	s := New()
	defer s.Close()
	s.policy.lookup = func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		if host != "public.test" {
			return nil, errors.New("unknown test host")
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	s.policy.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasPrefix(address, "8.8.8.8:") {
			t.Errorf("unchecked destination dialed: %s", address)
			return nil, ErrEgressDenied
		}
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(public.URL, "http://"))
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
