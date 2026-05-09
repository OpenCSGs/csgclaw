package runtimewiring

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/sandbox"
)

func WithPicoClawSandboxRuntime(channels config.ChannelsConfig) agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		host := s.PicoClawRuntimeHost()
		return agent.WithRuntime(picoclawsandbox.New(picoclawsandbox.Dependencies{
			ModelFallback:  host.ModelFallback,
			Server:         host.Server,
			Channels:       channels,
			ResolveBaseURL: resolveManagerBaseURL,
			EnsureRuntime:  host.EnsureRuntime,
			RuntimeHome:    host.RuntimeHome,
			CloseRuntime:   host.CloseRuntime,
			ResolveBox: func(ctx context.Context, rt sandbox.Runtime, got picoclawsandbox.AgentRef) (sandbox.Instance, string, error) {
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
			ResolveAgent: func(h agentruntime.Handle) (picoclawsandbox.AgentRef, error) {
				got, err := host.ResolveAgent(h)
				if err != nil {
					return picoclawsandbox.AgentRef{}, err
				}
				return picoclawsandbox.AgentRef{
					ID:        got.ID,
					Name:      got.Name,
					RuntimeID: strings.TrimSpace(got.RuntimeID),
					BoxID:     got.BoxID,
				}, nil
			},
			SyncHandle:          host.SyncHandle,
			EnsureGatewayConfig: host.EnsureGatewayConfig,
			EnsureWorkspace:     host.EnsureWorkspace,
			WorkspaceTemplate:   host.WorkspaceTemplate,
			EnsureProjectsRoot:  host.EnsureProjectsRoot,
			BuildRuntimeEnv: func(baseURL, accessToken, botID, llmBaseURL, modelID string, channels config.ChannelsConfig) map[string]string {
				env := picoClawBoxEnvVars(baseURL, accessToken, botID, llmBaseURL, modelID)
				addFeishuBoxEnvVars(env, botID, channels)
				return env
			},
			AddProfileEnv: agentAddProfileEnv,
			StreamLogs:    host.StreamLogs,
		}))(s)
	}
}

func UpdatePicoClawChannels(svc *agent.Service, channels config.ChannelsConfig) {
	if svc == nil {
		slog.Warn("skip picoclaw channel reload: agent service is nil")
		return
	}
	rt, err := svc.Runtime(agentruntime.KindPicoClawSandbox)
	if err != nil {
		slog.Warn("skip picoclaw channel reload: runtime not available", "runtime_kind", agentruntime.KindPicoClawSandbox, "error", err)
		return
	}
	updater, ok := rt.(interface {
		SetChannels(config.ChannelsConfig)
	})
	if !ok {
		slog.Warn("skip picoclaw channel reload: runtime does not support channel updates", "runtime_kind", rt.Kind())
		return
	}
	updater.SetChannels(channels)
}

func resolveManagerBaseURL(server config.ServerConfig) string {
	if server.AdvertiseBaseURL != "" {
		return strings.TrimRight(server.AdvertiseBaseURL, "/")
	}
	port := config.ListenPort(server.ListenAddr)
	if ip := localIPv4(); ip != "" {
		return fmt.Sprintf("http://%s:%s", ip, port)
	}
	return ""
}

func picoClawBoxEnvVars(baseURL, accessToken, botID, llmBaseURL, modelID string) map[string]string {
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

func agentAddProfileEnv(envVars map[string]string, profileEnv map[string]string) {
	for key, value := range profileEnv {
		key = strings.TrimSpace(key)
		if key == "" || isReservedSandboxEnvKey(key) {
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

func isReservedSandboxEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "HOME" || upper == "OPENAI_BASE_URL" || upper == "OPENAI_API_KEY" || upper == "OPENAI_MODEL" {
		return true
	}
	return strings.HasPrefix(upper, "CSGCLAW_") || strings.HasPrefix(upper, "PICOCLAW_")
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
		if ip := v.IP.To4(); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
			return ip.String()
		}
	case *net.IPAddr:
		if ip := v.IP.To4(); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
			return ip.String()
		}
	}
	return ""
}
