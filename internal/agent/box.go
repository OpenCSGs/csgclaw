package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
)

func (s *Service) createGatewayBox(ctx context.Context, rt sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
	if testCreateGatewayBoxHook != nil {
		return testCreateGatewayBoxHook(s, ctx, rt, image, name, botID, profile)
	}
	if rt == nil {
		return nil, sandbox.Info{}, fmt.Errorf("invalid sandbox runtime")
	}
	spec, err := s.gatewayCreateSpec(image, name, botID, profile)
	if err != nil {
		return nil, sandbox.Info{}, err
	}
	box, err := s.createBox(ctx, rt, spec)
	if err != nil {
		return nil, sandbox.Info{}, fmt.Errorf("create gateway box: %w", err)
	}
	info, err := s.boxInfo(ctx, box)
	if err != nil {
		_ = s.closeBox(box)
		return nil, sandbox.Info{}, fmt.Errorf("read gateway box info: %w", err)
	}
	return box, info, nil
}

func (s *Service) forceRemoveBox(ctx context.Context, rt sandbox.Runtime, idOrName string) error {
	if testForceRemoveBoxHook != nil {
		return testForceRemoveBoxHook(s, ctx, rt, idOrName)
	}
	if rt == nil {
		return fmt.Errorf("invalid sandbox runtime")
	}
	return rt.Remove(ctx, idOrName, sandbox.RemoveOptions{Force: true})
}

func (s *Service) gatewayCreateSpec(image, name, botID string, profile AgentProfile) (sandbox.CreateSpec, error) {
	profile = normalizeProfile(profile, name, "")
	if strings.TrimSpace(profile.ModelID) == "" {
		fallback := s.model.Resolved()
		profile.ModelID = fallback.ModelID
	}
	modelID := profile.ModelID
	managerBaseURL := resolveManagerBaseURL(s.server)
	llmBaseURL := llmBridgeBaseURL(managerBaseURL, botID)
	if _, err := ensureAgentPicoClawConfig(name, botID, s.server, config.ModelConfig{ModelID: modelID}); err != nil {
		return sandbox.CreateSpec{}, err
	}
	hostWorkspaceRoot, err := ensureAgentWorkspace(name, workspaceTemplateForAgent(name, botID))
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	projectsRoot, err := ensureAgentProjectsRoot()
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	envVars := picoclawBoxEnvVars(managerBaseURL, s.server.AccessToken, botID, llmBaseURL, modelID)
	addFeishuBoxEnvVars(envVars, botID, s.channels)
	addProfileEnvVars(envVars, profile.Env)
	envVars["HOME"] = "/home/picoclaw"
	spec := sandbox.CreateSpec{
		Image:      image,
		Name:       name,
		Detach:     true,
		AutoRemove: false,
		Env:        envVars,
		Cmd: []string{
			"/bin/sh",
			"-c",
			gatewayRunCommand(),
		},
	}

	for _, mount := range gatewayVolumeMounts(hostWorkspaceRoot, projectsRoot) {
		spec.Mounts = append(spec.Mounts, sandbox.Mount{
			HostPath:  mount.hostPath,
			GuestPath: mount.guestPath,
		})
	}

	return spec, nil
}

func addProfileEnvVars(envVars map[string]string, profileEnv map[string]string) {
	if len(profileEnv) == 0 {
		return
	}
	for key, value := range profileEnv {
		key = strings.TrimSpace(key)
		if key == "" || isReservedSandboxEnvKey(key) {
			continue
		}
		envVars[key] = value
	}
}

func isReservedSandboxEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "HOME" || upper == "OPENAI_BASE_URL" || upper == "OPENAI_API_KEY" || upper == "OPENAI_MODEL" {
		return true
	}
	return strings.HasPrefix(upper, "CSGCLAW_") || strings.HasPrefix(upper, "PICOCLAW_")
}

type gatewayVolumeMount struct {
	hostPath  string
	guestPath string
}

func gatewayVolumeMounts(hostWorkspaceRoot, projectsRoot string) []gatewayVolumeMount {
	return []gatewayVolumeMount{
		{
			hostPath:  hostWorkspaceRoot,
			guestPath: boxWorkspaceDir,
		},
		{
			hostPath:  projectsRoot,
			guestPath: boxProjectsDir,
		},
	}
}

func gatewayRunCommand() string {
	configPath := boxWorkspaceConfigPath(hostPicoClawConfig)
	securityPath := boxWorkspaceConfigPath(hostPicoClawSecurity)
	return "mkdir -p " + boxPicoClawDir +
		" && cp " + configPath + " " + filepath.Join(boxPicoClawDir, hostPicoClawConfig) +
		" && cp " + securityPath + " " + filepath.Join(boxPicoClawDir, hostPicoClawSecurity) +
		" && /usr/local/bin/picoclaw gateway -d 1>" + boxGatewayLogPath + " 2>/dev/null"
}

func boxWorkspaceConfigPath(name string) string {
	return filepath.Join(boxWorkspaceDir, filepath.FromSlash(hostPicoClawStateDir), name)
}

func gatewayStartCommand(debug bool) ([]string, []string) {
	if debug {
		return []string{"sleep"}, []string{"infinity"}
	}
	return []string{"tini"}, []string{"--", "picoclaw", "gateway", "-d"}
}

func ensureAgentProjectsRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	hostProjectsRoot := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if err := os.MkdirAll(hostProjectsRoot, 0o755); err != nil {
		return "", fmt.Errorf("create host projects dir: %w", err)
	}
	return hostProjectsRoot, nil
}

func ProjectsRoot() (string, error) {
	return ensureAgentProjectsRoot()
}

func llmBridgeBaseURL(managerBaseURL, botID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	return managerBaseURL + "/api/bots/" + strings.TrimSpace(botID) + "/llm"
}

func bridgeLLMEnvVars(llmBaseURL, accessToken, modelID string) map[string]string {
	return map[string]string{
		"CSGCLAW_LLM_BASE_URL": llmBaseURL,
		"CSGCLAW_LLM_API_KEY":  accessToken,
		"CSGCLAW_LLM_MODEL_ID": modelID,
		"OPENAI_BASE_URL":      llmBaseURL,
		"OPENAI_API_KEY":       accessToken,
		"OPENAI_MODEL":         modelID,
	}
}

func picoclawBoxEnvVars(baseURL, accessToken, botID, llmBaseURL, modelID string) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	picoclawModelID := picoclawBridgeModelID(modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BASE_URL"] = baseURL
	env["PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BOT_ID"] = botID
	env["PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_ID"] = picoclawModelID
	env["PICOCLAW_CUSTOM_MODEL_API_KEY"] = accessToken
	env["PICOCLAW_CUSTOM_MODEL_BASE_URL"] = llmBaseURL
	return env
}

func addFeishuBoxEnvVars(envVars map[string]string, botID string, channels config.ChannelsConfig) {
	if envVars == nil {
		return
	}
	botID = strings.TrimSpace(botID)
	if botID == "" || len(channels.Feishu) == 0 {
		return
	}
	feishu, ok := channels.Feishu[botID]
	if !ok {
		return
	}
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"] = feishu.AppID
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"] = feishu.AppSecret
}
