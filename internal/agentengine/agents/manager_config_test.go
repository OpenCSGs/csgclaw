package agents

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/config"
)

func TestResolveManagerBaseURLForDockerUsesHostAliasWhenAdvertiseURLIsImplicit(t *testing.T) {
	origDockerHostAliasEnabled := dockerHostAliasEnabled
	origLocalIPv4Resolver := localIPv4Resolver
	dockerHostAliasEnabled = func() bool { return true }
	localIPv4Resolver = func() string {
		t.Fatal("local IPv4 resolver should not be used for Docker Desktop callback URLs")
		return ""
	}
	defer func() {
		dockerHostAliasEnabled = origDockerHostAliasEnabled
		localIPv4Resolver = origLocalIPv4Resolver
	}()

	got := ResolveManagerBaseURLForSandboxProvider(config.ServerConfig{
		ListenAddr: "0.0.0.0:18080",
	}, config.DockerProvider)
	if want := "http://host.docker.internal:18080"; got != want {
		t.Fatalf("ResolveManagerBaseURLForSandboxProvider() = %q, want %q", got, want)
	}
}

func TestResolveLocalRuntimeBaseURLAlwaysUsesLoopback(t *testing.T) {
	got := ResolveLocalRuntimeBaseURL(config.ServerConfig{
		ListenAddr:       "0.0.0.0:18080",
		AdvertiseBaseURL: "https://public.example.test",
	})
	if want := "http://127.0.0.1:18080"; got != want {
		t.Fatalf("ResolveLocalRuntimeBaseURL() = %q, want %q", got, want)
	}
}

func TestResolveManagerBaseURLForLinuxDockerUsesHostLANAddress(t *testing.T) {
	origDockerHostAliasEnabled := dockerHostAliasEnabled
	origLocalIPv4Resolver := localIPv4Resolver
	dockerHostAliasEnabled = func() bool { return false }
	localIPv4Resolver = func() string { return "192.168.1.24" }
	defer func() {
		dockerHostAliasEnabled = origDockerHostAliasEnabled
		localIPv4Resolver = origLocalIPv4Resolver
	}()

	got := ResolveManagerBaseURLForSandboxProvider(config.ServerConfig{
		ListenAddr: "0.0.0.0:18080",
	}, config.DockerProvider)
	if want := "http://192.168.1.24:18080"; got != want {
		t.Fatalf("ResolveManagerBaseURLForSandboxProvider() = %q, want %q", got, want)
	}
}

func TestResolveManagerBaseURLForNonDockerUsesHostLANAddress(t *testing.T) {
	origDockerHostAliasEnabled := dockerHostAliasEnabled
	origLocalIPv4Resolver := localIPv4Resolver
	dockerHostAliasEnabled = func() bool { return true }
	localIPv4Resolver = func() string { return "192.168.1.24" }
	defer func() {
		dockerHostAliasEnabled = origDockerHostAliasEnabled
		localIPv4Resolver = origLocalIPv4Resolver
	}()

	got := ResolveManagerBaseURLForSandboxProvider(config.ServerConfig{
		ListenAddr: "0.0.0.0:18080",
	}, config.BoxLiteProvider)
	if want := "http://192.168.1.24:18080"; got != want {
		t.Fatalf("ResolveManagerBaseURLForSandboxProvider() = %q, want %q", got, want)
	}
}

func TestResolveManagerBaseURLForDockerKeepsExplicitAdvertiseURL(t *testing.T) {
	origDockerHostAliasEnabled := dockerHostAliasEnabled
	dockerHostAliasEnabled = func() bool { return true }
	defer func() {
		dockerHostAliasEnabled = origDockerHostAliasEnabled
	}()

	got := ResolveManagerBaseURLForSandboxProvider(config.ServerConfig{
		ListenAddr:       "0.0.0.0:18080",
		AdvertiseBaseURL: "http://tunnel.example.test/csgclaw/",
	}, config.DockerProvider)
	if want := "http://tunnel.example.test/csgclaw"; got != want {
		t.Fatalf("ResolveManagerBaseURLForSandboxProvider() = %q, want %q", got, want)
	}
}

func stringifyJSONList(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		result = append(result, text)
	}
	return result
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEnsureManagerWorkspaceCopiesEmbeddedTemplate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	managerTemplate, err := resolveRuntimeTemplateRoot(RuntimeKindCodex, RoleManager)
	if err != nil {
		t.Fatalf("resolveRuntimeTemplateRoot(manager) error = %v", err)
	}
	managerRoot, err := testBuiltinWorkspaceRoot("manager", RuntimeKindCodex)
	if err != nil {
		t.Fatalf("testBuiltinWorkspaceRoot(manager) error = %v", err)
	}
	managerRoot, err = ensureWorkspaceAtRoot(managerRoot, managerTemplate)
	if err != nil {
		t.Fatalf("ensureAgentWorkspace(manager) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(managerRoot, "skills", "agent-creator", "SKILL.md")); err != nil {
		t.Fatalf("os.Stat(manager creation skill) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(managerRoot, "skills", "agent-teams", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os.Stat(manager team skill) error = %v, want os.ErrNotExist", err)
	}
}

func TestIPv4FromAddr(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want string
	}{
		{
			name: "ipv4 net",
			addr: &net.IPNet{IP: net.ParseIP("192.168.1.20"), Mask: net.CIDRMask(24, 32)},
			want: "192.168.1.20",
		},
		{
			name: "ipv4 addr",
			addr: &net.IPAddr{IP: net.ParseIP("10.0.0.8")},
			want: "10.0.0.8",
		},
		{
			name: "loopback",
			addr: &net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			want: "",
		},
		{
			name: "ipv6",
			addr: &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipv4FromAddr(tt.addr); got != tt.want {
				t.Fatalf("ipv4FromAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInterfaceIPv4PrefersPrivateNonPointToPointInterface(t *testing.T) {
	ifaces := []net.Interface{
		{Index: 1, Name: "utun4", Flags: net.FlagUp | net.FlagPointToPoint},
		{Index: 2, Name: "en0", Flags: net.FlagUp | net.FlagBroadcast},
	}
	addrMap := map[int][]net.Addr{
		1: {
			&net.IPNet{IP: net.ParseIP("198.19.0.1"), Mask: net.CIDRMask(24, 32)},
		},
		2: {
			&net.IPNet{IP: net.ParseIP("192.168.1.13"), Mask: net.CIDRMask(24, 32)},
		},
	}
	detector := localIPDetector{
		listInterfaces: func() ([]net.Interface, error) { return ifaces, nil },
		interfaceAddrs: func(iface net.Interface) ([]net.Addr, error) { return addrMap[iface.Index], nil },
	}

	if got, want := detector.interfaceIPv4(), "192.168.1.13"; got != want {
		t.Fatalf("detector.interfaceIPv4() = %q, want %q", got, want)
	}
}

func TestInterfaceIPv4FallsBackToFirstNonPrivateAddress(t *testing.T) {
	ifaces := []net.Interface{
		{Index: 1, Name: "eth0", Flags: net.FlagUp | net.FlagBroadcast},
		{Index: 2, Name: "eth1", Flags: net.FlagUp | net.FlagBroadcast},
	}
	addrMap := map[int][]net.Addr{
		1: {
			&net.IPNet{IP: net.ParseIP("100.64.0.2"), Mask: net.CIDRMask(10, 32)},
		},
		2: {
			&net.IPNet{IP: net.ParseIP("203.0.113.8"), Mask: net.CIDRMask(24, 32)},
		},
	}
	detector := localIPDetector{
		listInterfaces: func() ([]net.Interface, error) { return ifaces, nil },
		interfaceAddrs: func(iface net.Interface) ([]net.Addr, error) { return addrMap[iface.Index], nil },
	}

	if got, want := detector.interfaceIPv4(), "100.64.0.2"; got != want {
		t.Fatalf("detector.interfaceIPv4() = %q, want %q", got, want)
	}
}

func TestLocalIPv4FallsBackToOutboundWhenInterfacesUnavailable(t *testing.T) {
	detector := localIPDetector{
		listInterfaces: func() ([]net.Interface, error) { return nil, errors.New("boom") },
		interfaceAddrs: func(net.Interface) ([]net.Addr, error) { return nil, nil },
		dialUDP4: func() (net.Conn, error) {
			return &fakeConn{
				localAddr: &net.UDPAddr{IP: net.ParseIP("10.0.0.8"), Port: 34567},
			}, nil
		},
	}

	if got, want := detector.localIPv4(), "10.0.0.8"; got != want {
		t.Fatalf("detector.localIPv4() = %q, want %q", got, want)
	}
}

type fakeConn struct {
	localAddr net.Addr
}

func (c *fakeConn) Read([]byte) (int, error)         { return 0, errors.New("not implemented") }
func (c *fakeConn) Write([]byte) (int, error)        { return 0, errors.New("not implemented") }
func (c *fakeConn) Close() error                     { return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return c.localAddr }
func (c *fakeConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }
