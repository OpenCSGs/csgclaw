package agent

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
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
}

func TestPicoclawBridgeModelIDPrefixesOpenAIForSlashModelNames(t *testing.T) {
	if got, want := picoclawBridgeModelID("Qwen/Qwen3-0.6B-GGUF"), "openai/Qwen/Qwen3-0.6B-GGUF"; got != want {
		t.Fatalf("picoclawBridgeModelID() = %q, want %q", got, want)
	}
	if got, want := picoclawBridgeModelID("openai/gpt-5.4"), "openai/gpt-5.4"; got != want {
		t.Fatalf("picoclawBridgeModelID() = %q, want %q", got, want)
	}
}

func TestEnsureAgentPicoClawConfigUsesDirectoryMountRoot(t *testing.T) {
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

	if info, err := os.Stat(root); err != nil {
		t.Fatalf("os.Stat(root) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("config root %q is not a directory", root)
	}
	for _, path := range []string{
		filepath.Join(root, hostPicoClawConfig),
		filepath.Join(root, ".security.yml"),
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

	root, err := ensureAgentWorkspace("alice", workspaceTemplateWorker)
	if err != nil {
		t.Fatalf("ensureAgentWorkspace(worker) error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(root, "USER.md"),
		filepath.Join(root, "AGENT.md"),
		filepath.Join(root, "SOUL.md"),
		filepath.Join(root, "memory", "MEMORY.md"),
		filepath.Join(root, "skills", "weather", "SKILL.md"),
	} {
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("os.Stat(%q) error = %v", path, err)
		} else if info.IsDir() {
			t.Fatalf("workspace file %q is unexpectedly a directory", path)
		}
	}

	managerRoot, err := ensureAgentWorkspace("manager", workspaceTemplateForAgent(ManagerName, ManagerUserID))
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
