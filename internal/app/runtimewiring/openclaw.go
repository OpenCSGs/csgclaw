package runtimewiring

import (
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/channel/feishu"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/sandboxgateway"
	"fmt"
)

func WithOpenClawSandboxRuntime(feishuProvider feishu.AgentCredentialProvider) agent.ControllerOption {
	return func(s *agent.Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		host := s.OpenClawRuntimeHost()
		return withSandboxRuntimeHost(host, feishuProvider, openClawBoxEnvVars, func(deps sandboxgateway.Dependencies) agentruntime.Runtime {
			return openclawsandbox.New(deps)
		})(s)
	}
}

func UpdateOpenClawFeishuProvider(svc *agent.Controller, provider feishu.AgentCredentialProvider) {
	updateRuntimeFeishuProvider(svc, agentruntime.KindOpenClawSandbox, provider)
}

func openClawBoxEnvVars(baseURL, accessToken, participantID, _ string, llmBaseURL, modelID string, _ feishu.AgentCredentialProvider) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["CSGCLAW_BOT_ID"] = participantID
	return env
}
