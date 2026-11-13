package server

import (
	"net"
	"testing"
)

type memoryListener struct{ address net.Addr }

func (l memoryListener) Addr() net.Addr            { return l.address }
func (l memoryListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l memoryListener) Close() error              { return nil }

func TestAgentMemoryAutomaticallyFollowsBoundListener(t *testing.T) {
	for _, tc := range []struct {
		name, ip, want string
		desktop, cloud bool
	}{
		{name: "local web", ip: "127.0.0.1", want: "local"},
		{name: "IPv6 local web", ip: "::1", want: "local"},
		{name: "IPv4 mapped loopback", ip: "::ffff:127.0.0.1", want: "local"},
		{name: "all IPv4 interfaces", ip: "0.0.0.0", want: "disabled"},
		{name: "all IPv6 interfaces", ip: "::", want: "disabled"},
		{name: "LAN", ip: "192.0.2.1", want: "disabled"},
		{name: "desktop", ip: "127.0.0.1", desktop: true, want: "desktop"},
		{name: "shared desktop", ip: "0.0.0.0", desktop: true, want: "disabled"},
		{name: "cloud loopback", ip: "127.0.0.1", cloud: true, want: "disabled"},
		{name: "cloud desktop", ip: "127.0.0.1", cloud: true, desktop: true, want: "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{MemoryCloud: tc.cloud, NoAuth: true}
			if tc.desktop {
				opts.Desktop = &DesktopOptions{}
			}
			listener := memoryListener{&net.TCPAddr{IP: net.ParseIP(tc.ip), Port: 18080}}
			if got := agentMemoryMode(opts, listener); got != tc.want {
				t.Fatalf("mode=%s want %s", got, tc.want)
			}
		})
	}
	for _, listener := range []net.Listener{nil, memoryListener{&net.UnixAddr{Name: "fixture", Net: "unix"}}, memoryListener{(*net.TCPAddr)(nil)}} {
		if got := agentMemoryMode(Options{}, listener); got != "disabled" {
			t.Fatalf("invalid listener mode=%s", got)
		}
	}
}
