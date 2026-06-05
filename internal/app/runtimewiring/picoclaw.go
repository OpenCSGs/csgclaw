package runtimewiring

import (
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/feishu"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/runtime/sandboxgateway"
)

func WithPicoClawSandboxRuntime(feishuProvider feishu.BotCredentialProvider) agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		host := s.PicoClawRuntimeHost()
		return withSandboxRuntimeHost(host, feishuProvider, picoClawRuntimeEnvVars, func(deps sandboxgateway.Dependencies) agentruntime.Runtime {
			return picoclawsandbox.New(deps)
		})(s)
	}
}

func UpdatePicoClawFeishuProvider(svc *agent.Service, provider feishu.BotCredentialProvider) {
	updateRuntimeFeishuProvider(svc, agentruntime.KindPicoClawSandbox, provider)
}

func picoClawRuntimeEnvVars(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID string, provider feishu.BotCredentialProvider) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	picoclawModelID := picoclawBridgeModelID(modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_ENABLED"] = "true"
	env["PICOCLAW_CHANNELS_CSGCLAW_BASE_URL"] = baseURL
	env["PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID"] = participantID
	env["PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_ID"] = picoclawModelID
	env["PICOCLAW_CUSTOM_MODEL_API_KEY"] = accessToken
	env["PICOCLAW_CUSTOM_MODEL_BASE_URL"] = llmBaseURL
	addFeishuBoxEnvVars(env, agentID, provider)
	return env
}

func addFeishuBoxEnvVars(envVars map[string]string, botID string, provider feishu.BotCredentialProvider) {
	if envVars == nil {
		return
	}
	botID = strings.TrimSpace(botID)
	if botID == "" || provider == nil {
		return
	}
	app, ok := provider.BotConfig(botID)
	if !ok {
		return
	}
	appID := strings.TrimSpace(app.AppID)
	appSecret := strings.TrimSpace(app.AppSecret)
	if appID == "" || appSecret == "" {
		return
	}
	envVars["PICOCLAW_CHANNELS_FEISHU_ENABLED"] = "true"
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"] = appID
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"] = appSecret
}

func picoclawBridgeModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(modelID), "openai/") {
		return modelID
	}
	if prefix, rest, ok := strings.Cut(modelID, ":"); ok && strings.EqualFold(strings.TrimSpace(prefix), "openai") && strings.TrimSpace(rest) != "" {
		return "openai/" + strings.TrimSpace(rest)
	}
	return "openai/" + modelID
}
