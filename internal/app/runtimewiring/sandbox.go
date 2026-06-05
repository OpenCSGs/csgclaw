package runtimewiring

import (
	"context"
	"log/slog"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/feishu"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
	"csgclaw/internal/sandbox"
)

type sandboxRuntimeEnvBuilder func(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID string, provider feishu.BotCredentialProvider) map[string]string

func withSandboxRuntimeHost(host agent.PicoClawRuntimeHost, feishuProvider feishu.BotCredentialProvider, buildRuntimeEnv sandboxRuntimeEnvBuilder, newRuntime func(sandboxgateway.Dependencies) agentruntime.Runtime) agent.ServiceOption {
	return func(s *agent.Service) error {
		return agent.WithRuntime(newRuntime(sandboxgateway.Dependencies{
			FeishuProvider:      feishuProvider,
			SandboxProviderName: host.SandboxProviderName,
			EnsureRuntime:       host.EnsureRuntime,
			RuntimeHome:         host.RuntimeHome,
			CloseRuntime:        host.CloseRuntime,
			ResolveBox: func(ctx context.Context, rt sandbox.Runtime, got sandboxgateway.AgentRef) (sandbox.Instance, string, error) {
				return host.ResolveBox(ctx, rt, agent.Agent{
					ID:        got.ID,
					Name:      got.Name,
					RuntimeID: got.RuntimeID,
					BoxID:     got.BoxID,
				})
			},
			CreateBox:      host.CreateBox,
			StartBox:       host.StartBox,
			StopBox:        host.StopBox,
			BoxInfo:        host.BoxInfo,
			ForceRemoveBox: host.ForceRemoveBox,
			CloseBox:       host.CloseBox,
			RunBoxCommand:  host.RunBoxCommand,
			ResolveAgent: func(h agentruntime.Handle) (sandboxgateway.AgentRef, error) {
				got, err := host.ResolveAgent(h)
				if err != nil {
					return sandboxgateway.AgentRef{}, err
				}
				return sandboxgateway.AgentRef{
					ID:        got.ID,
					Name:      got.Name,
					RuntimeID: strings.TrimSpace(got.RuntimeID),
					BoxID:     got.BoxID,
				}, nil
			},
			SyncHandle:      host.SyncHandle,
			BuildRuntimeEnv: buildRuntimeEnv,
			AddProfileEnv:   agentAddProfileEnv,
			StreamLogs:      host.StreamLogs,
		}))(s)
	}
}

func updateRuntimeFeishuProvider(svc *agent.Service, runtimeKind string, provider feishu.BotCredentialProvider) {
	if svc == nil {
		slog.Warn("skip feishu provider update: agent service is nil", "runtime_kind", runtimeKind)
		return
	}
	rt, err := svc.Runtime(runtimeKind)
	if err != nil {
		slog.Warn("skip feishu provider update: runtime not available", "runtime_kind", runtimeKind, "error", err)
		return
	}
	updater, ok := rt.(interface {
		SetFeishuProvider(feishu.BotCredentialProvider)
	})
	if !ok {
		slog.Warn("skip feishu provider update: runtime does not support provider updates", "runtime_kind", rt.Kind())
		return
	}
	updater.SetFeishuProvider(provider)
}

func agentAddProfileEnv(envVars map[string]string, profileEnv map[string]string) {
	for key, value := range profileEnv {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || isReservedSandboxEnvKey(key) {
			continue
		}
		envVars[key] = value
	}
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

func isReservedSandboxEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "HOME" || upper == "OPENAI_BASE_URL" || upper == "OPENAI_API_KEY" || upper == "OPENAI_MODEL" {
		return true
	}
	return strings.HasPrefix(upper, "CSGCLAW_") || strings.HasPrefix(upper, "PICOCLAW_")
}
