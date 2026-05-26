package agent

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/templates"
)

func TestRenderManagerSecurityConfig(t *testing.T) {
	got := renderManagerSecurityConfig(config.ServerConfig{
		AccessToken: "shared-token",
	}, config.ModelConfig{
		ModelID: "minimax-m2.7",
		APIKey:  "sk-1234567890",
	})

	for _, want := range []string{
		"model_list:\n",
		"  minimax-m2.7:0:\n",
		"    api_keys:\n",
		"      - shared-token\n",
		"channels: {}\n",
		"web: {}\n",
		"skills: {}\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderManagerSecurityConfig() missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderAgentPicoClawConfigUsesBridgeModelEndpoint(t *testing.T) {
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = localIPv4 }()

	data, err := renderAgentPicoClawConfig("u-ux", config.ServerConfig{
		ListenAddr:  "0.0.0.0:18080",
		AccessToken: "shared-token",
	}, config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		ModelID:  "gpt-5.4",
		BaseURL:  "https://cloud.infini-ai.com/maas/v1",
		APIKey:   "sk-upstream",
	})
	if err != nil {
		t.Fatalf("renderAgentPicoClawConfig() error = %v", err)
	}

	text := string(data)
	for _, want := range []string{
		`"model_name": "gpt-5.4"`,
		`"model": "openai/gpt-5.4"`,
		`"api_base": "http://10.0.0.8:18080/api/bots/u-ux/llm"`,
		`"api_key": "shared-token"`,
		`"bot_id": "u-ux"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("renderAgentPicoClawConfig() missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "cloud.infini-ai.com") {
		t.Fatalf("renderAgentPicoClawConfig() leaked upstream base URL:\n%s", text)
	}

	var rendered map[string]any
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("renderAgentPicoClawConfig() produced invalid JSON: %v", err)
	}
	tools, ok := rendered["tools"].(map[string]any)
	if !ok {
		t.Fatalf("renderAgentPicoClawConfig() missing tools in:\n%s", text)
	}
	execTool, ok := tools["exec"].(map[string]any)
	if !ok {
		t.Fatalf("renderAgentPicoClawConfig() missing tools.exec in:\n%s", text)
	}
	if got, want := execTool["timeout_seconds"], float64(600); got != want {
		t.Fatalf("tools.exec.timeout_seconds = %v, want %v", got, want)
	}
	session, ok := rendered["session"].(map[string]any)
	if !ok {
		t.Fatalf("renderAgentPicoClawConfig() missing session in:\n%s", text)
	}
	dimensions, ok := session["dimensions"].([]any)
	if !ok {
		t.Fatalf("session.dimensions = %T, want array in:\n%s", session["dimensions"], text)
	}
	if got, want := stringifyJSONList(dimensions), []string{"chat", "topic"}; !stringSlicesEqual(got, want) {
		t.Fatalf("session.dimensions = %v, want %v", got, want)
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

func TestPicoclawBridgeModelIDPrefixesOpenAIForSlashModelNames(t *testing.T) {
	if got, want := picoclawBridgeModelID("Qwen/Qwen3-0.6B-GGUF"), "openai/Qwen/Qwen3-0.6B-GGUF"; got != want {
		t.Fatalf("picoclawBridgeModelID() = %q, want %q", got, want)
	}
	if got, want := picoclawBridgeModelID("openai/gpt-5.4"), "openai/gpt-5.4"; got != want {
		t.Fatalf("picoclawBridgeModelID() = %q, want %q", got, want)
	}
}

func TestEnsureAgentPicoClawConfigUsesRuntimeRoot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	root, err := ensureAgentPicoClawConfig("ux", "u-ux", config.ServerConfig{
		ListenAddr:  "0.0.0.0:18080",
		AccessToken: "shared-token",
	}, config.ModelConfig{
		ModelID: "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("ensureAgentPicoClawConfig() error = %v", err)
	}

	wantRoot := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "ux", picoclawsandbox.HostDir)
	if root != wantRoot {
		t.Fatalf("ensureAgentPicoClawConfig() = %q, want %q", root, wantRoot)
	}
	if info, err := os.Stat(root); err != nil {
		t.Fatalf("os.Stat(root) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("config root %q is not a directory", root)
	}
	for _, path := range []string{
		filepath.Join(root, picoclawsandbox.HostConfig),
		filepath.Join(root, picoclawsandbox.HostSecurity),
	} {
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("os.Stat(%q) error = %v", path, err)
		} else if info.IsDir() {
			t.Fatalf("config artifact %q is unexpectedly a directory", path)
		}
	}
}

func TestEnsureAgentWorkspaceCopiesEmbeddedTemplate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	root, err := ensureAgentWorkspace("alice", templates.PicoClawWorkerRoot)
	if err != nil {
		t.Fatalf("ensureAgentWorkspace(worker) error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(root, "USER.md"),
		filepath.Join(root, "AGENT.md"),
		filepath.Join(root, "SOUL.md"),
		filepath.Join(root, "memory", "MEMORY.md"),
		filepath.Join(root, "skills", "skill-creator", "SKILL.md"),
	} {
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("os.Stat(%q) error = %v", path, err)
		} else if info.IsDir() {
			t.Fatalf("workspace file %q is unexpectedly a directory", path)
		}
	}

	managerTemplate, err := resolveRuntimeTemplateRoot(RuntimeKindPicoClawSandbox, RoleManager)
	if err != nil {
		t.Fatalf("resolveRuntimeTemplateRoot(manager) error = %v", err)
	}
	managerRoot, err := ensureAgentWorkspace("manager", managerTemplate)
	if err != nil {
		t.Fatalf("ensureAgentWorkspace(manager) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(managerRoot, "skills", "manager-worker-dispatch", "SKILL.md")); err != nil {
		t.Fatalf("os.Stat(manager skill) error = %v", err)
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
