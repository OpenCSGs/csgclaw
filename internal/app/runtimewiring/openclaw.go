package runtimewiring

import (
	"fmt"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/feishu"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/sandboxgateway"
)

func WithOpenClawSandboxRuntime(feishuProvider feishu.BotCredentialProvider) agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		host := s.OpenClawRuntimeHost()
		return withSandboxRuntimeHost(host, feishuProvider, openClawBoxEnvVars, func(deps sandboxgateway.Dependencies) agentruntime.Runtime {
			return openclawsandbox.New(deps)
		})(s)
	}
}

func UpdateOpenClawFeishuProvider(svc *agent.Service, provider feishu.BotCredentialProvider) {
	updateRuntimeFeishuProvider(svc, agentruntime.KindOpenClawSandbox, provider)
}

func openClawBoxEnvVars(baseURL, accessToken, botID, llmBaseURL, modelID string, _ feishu.BotCredentialProvider) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["CSGCLAW_BOT_ID"] = botID
	return env
}
