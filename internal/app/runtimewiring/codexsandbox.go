package runtimewiring

import (
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/feishu"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codexsandbox"
	"csgclaw/internal/runtime/sandboxgateway"
)

func WithCodexSandboxRuntime(feishuProvider feishu.AgentCredentialProvider) agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		host := s.PicoClawRuntimeHost()
		return withSandboxRuntimeHost(host, feishuProvider, codexSandboxRuntimeEnvVars, func(deps sandboxgateway.Dependencies) agentruntime.Runtime {
			return codexsandbox.New(deps)
		})(s)
	}
}

func UpdateCodexSandboxFeishuProvider(svc *agent.Service, provider feishu.AgentCredentialProvider) {
	updateRuntimeFeishuProvider(svc, agentruntime.KindCodexSandbox, provider)
}

func codexSandboxRuntimeEnvVars(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID string, provider feishu.AgentCredentialProvider) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["CSGCLAW_PARTICIPANT_ID"] = participantID
	env["CSGCLAW_AGENT_ID"] = agentID
	env["LARK_CHANNEL_HOME"] = codexsandbox.BoxDir
	env["LARK_CHANNEL_PROFILE"] = codexsandbox.ProfileName
	env["LARK_CHANNEL_CONFIG"] = codexsandbox.BoxConfigPath
	env["LARK_CHANNEL_CODEX_BIN"] = "/usr/local/bin/codex"
	env["CODEX_HOME"] = codexsandbox.BoxCodexHomeDir
	env["CSGCLAW_CODEX_GATEWAY_WORKSPACE"] = codexsandbox.BoxProjectsDir
	addCodexSandboxFeishuEnvVars(env, agentID, provider)
	return env
}

func addCodexSandboxFeishuEnvVars(envVars map[string]string, agentID string, provider feishu.AgentCredentialProvider) {
	if envVars == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || provider == nil {
		return
	}
	participantID, app, ok := provider.BotConfigForAgent(agentID)
	if !ok {
		return
	}
	appID := strings.TrimSpace(app.AppID)
	appSecret := strings.TrimSpace(app.AppSecret)
	if appID == "" || appSecret == "" {
		return
	}
	envVars["LARK_CHANNEL_APP_ID"] = appID
	envVars["LARK_CHANNEL_PARTICIPANT_ID"] = strings.TrimSpace(participantID)
	envVars["LARK_CHANNEL_TENANT"] = "feishu"
	envVars["APP_SECRET"] = appSecret
}
