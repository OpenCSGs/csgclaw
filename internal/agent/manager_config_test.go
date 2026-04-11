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

func TestEnsureAgentPicoClawConfigUsesDirectoryMountRoot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	managerSkillSourceDirResolver = func() (string, error) {
		src := filepath.Join(t.TempDir(), "skills", "manager-worker-dispatch")
		if err := os.MkdirAll(filepath.Join(src, "scripts"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll(skill source) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("name: test\n"), 0o644); err != nil {
			t.Fatalf("os.WriteFile(SKILL.md) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(src, "scripts", "manager_worker_api.py"), []byte("print('ok')\n"), 0o755); err != nil {
			t.Fatalf("os.WriteFile(manager_worker_api.py) error = %v", err)
		}
		return src, nil
	}
	defer func() { managerSkillSourceDirResolver = bundledManagerSkillSourceDir }()

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
		t.Fatalf("mount root %q is not a directory", root)
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

	mounts := gatewayVolumeMounts(root, "/tmp/projects")
	if len(mounts) != 2 {
		t.Fatalf("gatewayVolumeMounts() len = %d, want 2", len(mounts))
	}
	if mounts[0].hostPath != root || mounts[0].guestPath != boxPicoClawDir {
		t.Fatalf("gatewayVolumeMounts()[0] = %+v, want %q => %q", mounts[0], root, boxPicoClawDir)
	}
	if strings.HasSuffix(mounts[0].hostPath, hostPicoClawConfig) || strings.HasSuffix(mounts[0].hostPath, ".security.yml") {
		t.Fatalf("gatewayVolumeMounts()[0].hostPath = %q, want directory mount root", mounts[0].hostPath)
	}
}

func TestEnsureAgentPicoClawConfigSeedsManagerSkill(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	src := filepath.Join(t.TempDir(), "skills", "manager-worker-dispatch")
	for _, dir := range []string{
		src,
		filepath.Join(src, "scripts"),
		filepath.Join(src, "agents"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll(%q) error = %v", dir, err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(src, "SKILL.md"):                         "name: manager-worker-dispatch\n",
		filepath.Join(src, "scripts", "manager_worker_api.py"): "#!/usr/bin/env python\nprint('ok')\n",
		filepath.Join(src, "agents", "openai.yaml"):            "interface: {}\n",
	} {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".py") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", path, err)
		}
	}

	managerSkillSourceDirResolver = func() (string, error) { return src, nil }
	defer func() { managerSkillSourceDirResolver = bundledManagerSkillSourceDir }()

	root, err := ensureAgentPicoClawConfig("manager", "u-manager", config.ServerConfig{
		ListenAddr:  "0.0.0.0:18080",
		AccessToken: "shared-token",
	}, config.ModelConfig{
		ModelID: "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("ensureAgentPicoClawConfig() error = %v", err)
	}

	for _, rel := range []string{
		filepath.Join("workspace", "memory", "MEMORY.md"),
		filepath.Join("workspace", "skills", "manager-worker-dispatch", "SKILL.md"),
		filepath.Join("workspace", "skills", "manager-worker-dispatch", "scripts", "manager_worker_api.py"),
		filepath.Join("workspace", "skills", "manager-worker-dispatch", "agents", "openai.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("os.Stat(%q) error = %v", filepath.Join(root, rel), err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "workspace", "memory", "MEMORY.md"))
	if err != nil {
		t.Fatalf("os.ReadFile(MEMORY.md) error = %v", err)
	}
	if !strings.Contains(string(data), "python scripts/manager_worker_api.py list-workers") {
		t.Fatalf("MEMORY.md = %q, want dispatch fast path", string(data))
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
