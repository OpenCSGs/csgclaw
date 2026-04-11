package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

func (s *Service) createGatewayBox(ctx context.Context, rt *boxlite.Runtime, image, name, botID string, modelCfg config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
	if testCreateGatewayBoxHook != nil {
		return testCreateGatewayBoxHook(s, ctx, rt, image, name, botID, modelCfg)
	}
	if !runtimeValid(rt) {
		return nil, nil, fmt.Errorf("invalid boxlite runtime")
	}
	boxOpts, err := s.gatewayBoxOptions(name, botID, modelCfg)
	if err != nil {
		return nil, nil, err
	}
	box, err := rt.Create(ctx, image, boxOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create gateway box: %w", err)
	}
	if err := box.Start(ctx); err != nil {
		_ = s.closeBox(box)
		return nil, nil, fmt.Errorf("start gateway box: %w", err)
	}
	info, err := box.Info(ctx)
	if err != nil {
		_ = s.closeBox(box)
		return nil, nil, fmt.Errorf("read gateway box info: %w", err)
	}
	return box, info, nil
}

func (s *Service) forceRemoveBox(ctx context.Context, rt *boxlite.Runtime, idOrName string) error {
	if testForceRemoveBoxHook != nil {
		return testForceRemoveBoxHook(s, ctx, rt, idOrName)
	}
	if !runtimeValid(rt) {
		return fmt.Errorf("invalid boxlite runtime")
	}
	return rt.ForceRemove(ctx, idOrName)
}

func (s *Service) gatewayBoxOptions(name, botID string, modelCfg config.ModelConfig) ([]boxlite.BoxOption, error) {
	modelCfg = modelCfg.Resolved()
	if strings.TrimSpace(modelCfg.ModelID) == "" {
		modelCfg = s.model.Resolved()
	}
	modelID := modelCfg.ModelID
	managerBaseURL := resolveManagerBaseURL(s.server)
	llmBaseURL := llmBridgeBaseURL(managerBaseURL, botID)
	envVars := picoclawBoxEnvVars(managerBaseURL, s.server.AccessToken, botID, llmBaseURL, modelID)
	addFeishuBoxEnvVars(envVars, botID, s.channels)
	opts := []boxlite.BoxOption{
		boxlite.WithName(name),
		boxlite.WithDetach(true),
		boxlite.WithAutoRemove(false),
		//boxlite.WithPort(managerHostPort, managerGuestPort),
		boxlite.WithEnv("HOME", "/home/picoclaw"),
	}
	for key, value := range envVars {
		opts = append(opts, boxlite.WithEnv(key, value))
	}
	//entrypoint, cmd := gatewayStartCommand(managerDebugMode)
	opts = append(opts,
		//boxlite.WithEntrypoint(entrypoint...),
		//boxlite.WithCmd(cmd...),
		boxlite.WithCmd("/bin/sh", "-c", "/usr/local/bin/picoclaw gateway -d 1>~/.picoclaw/gateway.log 2>/dev/null"),
		//boxlite.WithCmd("sleep", "infinity"),
	)

	hostPicoClawRoot, err := ensureAgentPicoClawConfig(name, botID, s.server, modelCfg)
	if err != nil {
		return nil, err
	}
	projectsRoot, err := ensureAgentProjectsRoot()
	if err != nil {
		return nil, err
	}
	for _, mount := range gatewayVolumeMounts(hostPicoClawRoot, projectsRoot) {
		opts = append(opts, boxlite.WithVolume(mount.hostPath, mount.guestPath))
	}

	return opts, nil
}

type gatewayVolumeMount struct {
	hostPath  string
	guestPath string
}

func gatewayVolumeMounts(hostPicoClawRoot, projectsRoot string) []gatewayVolumeMount {
	return []gatewayVolumeMount{
		{
			hostPath:  hostPicoClawRoot,
			guestPath: boxPicoClawDir,
		},
		{
			hostPath:  projectsRoot,
			guestPath: boxProjectsDir,
		},
	}
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
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BASE_URL"] = baseURL
	env["PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BOT_ID"] = botID
	env["PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_ID"] = modelID
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
