package agent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"csgclaw/internal/config"
)

const managerAgentsDirName = "agents"

//go:embed defaults/picoclaw-config.json
var defaultManagerPicoClawConfig []byte

//go:embed defaults/manager-security.yml
var defaultManagerSecurityConfig string

var managerSkillSourceDirResolver = bundledManagerSkillSourceDir

const managerMemoryContents = `# Manager Memory

When an admin asks you to arrange or reuse workers such as ux, dev, and qa:

- Do not do the implementation work yourself.
- Do not use message for status chatter or request restatement.
- Use the bundled manager-worker-dispatch workflow directly from ~/.picoclaw/workspace/skills/manager-worker-dispatch.
- Fast path:
  1. python scripts/manager_worker_api.py list-workers
  2. join the chosen workers to the room
  3. write todo.json under ~/.picoclaw/workspace/projects/<slug>/
  4. python scripts/manager_worker_api.py start-tracking --room-id <room> --todo-path <todo.json>
- Only open SKILL.md if the workflow must change or a required command is unclear.
- After tracking starts, send one concise assignment summary.
`

func ensureManagerPicoClawConfig(server config.ServerConfig, model config.ModelConfig) (string, error) {
	return ensureAgentPicoClawConfig(ManagerName, "u-manager", server, model)
}

func ensureAgentPicoClawConfig(agentName, botID string, server config.ServerConfig, model config.ModelConfig) (string, error) {
	hostRoot, err := agentPicoClawRoot(agentName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(hostRoot, hostPicoClawLogs), 0o755); err != nil {
		return "", fmt.Errorf("create manager picoclaw logs dir: %w", err)
	}
	if err := ensureAgentWorkspace(hostRoot, agentName); err != nil {
		return "", err
	}

	data, err := renderAgentPicoClawConfig(botID, server, model)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(hostRoot, hostPicoClawConfig)
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write manager picoclaw config: %w", err)
	}
	securityData := renderManagerSecurityConfig(server, model)
	securityPath := filepath.Join(hostRoot, ".security.yml")
	if err := os.WriteFile(securityPath, []byte(securityData), 0o600); err != nil {
		return "", fmt.Errorf("write manager security config: %w", err)
	}
	return hostRoot, nil
}

func managerPicoClawRoot() (string, error) {
	return agentPicoClawRoot(ManagerName)
}

func agentPicoClawRoot(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName, hostPicoClawDir), nil
}

func renderManagerPicoClawConfig(server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	return renderAgentPicoClawConfig("u-manager", server, model)
}

func renderAgentPicoClawConfig(botID string, server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(defaultManagerPicoClawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("decode embedded manager picoclaw config: %w", err)
	}

	if err := updateModelList(cfg, botID, server, model); err != nil {
		return nil, err
	}
	if err := updateCSGClawChannel(cfg, botID, server); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manager picoclaw config: %w", err)
	}
	return data, nil
}

func updateModelList(cfg map[string]any, botID string, server config.ServerConfig, modelCfg config.ModelConfig) error {
	modelList, ok := cfg["model_list"].([]any)
	if !ok || len(modelList) == 0 {
		return fmt.Errorf("embedded manager picoclaw config is missing model_list[0]")
	}
	model, ok := modelList[0].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config has invalid model_list[0]")
	}
	if modelCfg.ModelID != "" {
		model["model_name"] = modelCfg.ModelID
		model["model"] = modelCfg.ModelID
	}
	if agents, ok := cfg["agents"].(map[string]any); ok {
		if defaults, ok := agents["defaults"].(map[string]any); ok && modelCfg.ModelID != "" {
			defaults["model_name"] = modelCfg.ModelID
		}
	}

	if managerBaseURL := resolveManagerBaseURL(server); managerBaseURL != "" {
		model["api_base"] = llmBridgeBaseURL(managerBaseURL, botID)
	}
	if server.AccessToken != "" {
		model["api_key"] = server.AccessToken
	}
	return nil
}

func updateCSGClawChannel(cfg map[string]any, botID string, server config.ServerConfig) error {
	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing channels")
	}
	channel, ok := channels["csgclaw"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing channels.csgclaw")
	}
	if baseURL := resolveManagerBaseURL(server); baseURL != "" {
		channel["base_url"] = baseURL
	}
	if server.AccessToken != "" {
		channel["access_token"] = server.AccessToken
	}
	channel["bot_id"] = botID
	channel["enabled"] = true
	return nil
}

func resolveManagerBaseURL(server config.ServerConfig) string {
	port := config.ListenPort(server.ListenAddr)
	if ip := localIPv4Resolver(); ip != "" {
		return fmt.Sprintf("http://%s:%s", ip, port)
	}
	if server.AdvertiseBaseURL != "" {
		return strings.TrimRight(server.AdvertiseBaseURL, "/")
	}
	return ""
}

func localIPv4() string {
	if ip := outboundIPv4(); ip != "" {
		return ip
	}
	return interfaceIPv4()
}

func outboundIPv4() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	ip := addr.IP.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

func interfaceIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ip := ipv4FromAddr(addr); ip != "" {
				return ip
			}
		}
	}
	return ""
}

func ipv4FromAddr(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.IPNet:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	case *net.IPAddr:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	default:
		return ""
	}
}

func renderManagerSecurityConfig(server config.ServerConfig, model config.ModelConfig) string {
	modelID := model.ModelID
	apiKey := strings.TrimSpace(server.AccessToken)
	if apiKey == "" {
		apiKey = model.APIKey
	}

	content := strings.ReplaceAll(defaultManagerSecurityConfig, "__MODEL_ID__", modelID)
	content = strings.ReplaceAll(content, "__API_KEY__", apiKey)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

func ensureAgentWorkspace(hostRoot, agentName string) error {
	workspaceRoot := filepath.Join(hostRoot, "workspace")
	for _, dir := range []string{
		filepath.Join(workspaceRoot, "memory"),
		filepath.Join(workspaceRoot, "projects"),
		filepath.Join(workspaceRoot, "sessions"),
		filepath.Join(workspaceRoot, "state"),
		filepath.Join(workspaceRoot, "skills"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create agent workspace dir %q: %w", dir, err)
		}
	}
	if !strings.EqualFold(strings.TrimSpace(agentName), ManagerName) {
		return nil
	}

	dstRoot := filepath.Join(workspaceRoot, "skills", "manager-worker-dispatch")
	srcRoot, err := managerSkillSourceDirResolver()
	if err != nil {
		return fmt.Errorf("resolve bundled manager skill: %w", err)
	}
	if err := copyDirTree(srcRoot, dstRoot); err != nil {
		return fmt.Errorf("seed bundled manager skill: %w", err)
	}
	memoryPath := filepath.Join(workspaceRoot, "memory", "MEMORY.md")
	if err := os.WriteFile(memoryPath, []byte(managerMemoryContents), 0o644); err != nil {
		return fmt.Errorf("write manager memory: %w", err)
	}
	return nil
}

func bundledManagerSkillSourceDir() (string, error) {
	candidates := make([]string, 0, 4)
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe), filepath.Dir(filepath.Dir(exe)))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Dir(file), filepath.Dir(filepath.Dir(filepath.Dir(file))))
	}

	for _, base := range candidates {
		if found := findManagerSkillUnder(base); found != "" {
			return found, nil
		}
	}
	return "", os.ErrNotExist
}

func findManagerSkillUnder(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	for current := base; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "skills", "manager-worker-dispatch")
		if info, err := os.Stat(filepath.Join(candidate, "SKILL.md")); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func copyDirTree(srcRoot, dstRoot string) error {
	srcRoot = filepath.Clean(srcRoot)
	dstRoot = filepath.Clean(dstRoot)
	if err := os.RemoveAll(dstRoot); err != nil {
		return err
	}

	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		target := dstRoot
		if rel != "." {
			target = filepath.Join(dstRoot, rel)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return err
		}
		return nil
	})
}
