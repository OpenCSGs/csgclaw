package agents

// Fixtures exercise Runtime-owned gateway rendering without production Engine dependencies.
import (
	feishu "csgclaw/internal/channel/feishu"
	config "csgclaw/internal/config"
	picoclawsandbox "csgclaw/internal/runtime/picoclawsandbox"
)

func ensureManagerPicoClawConfig(server config.ServerConfig, model config.ModelConfig) (string, error) {
	return ensureAgentPicoClawConfigForParticipant(ManagerName, ManagerParticipantID, ManagerUserID, server, model)
}

func ensureAgentPicoClawConfig(agentName, agentID string, server config.ServerConfig, model config.ModelConfig) (string, error) {
	return ensureAgentPicoClawConfigForParticipant(agentName, agentID, agentID, server, model)
}

func ensureAgentPicoClawConfigForParticipant(agentName, participantID, agentID string, server config.ServerConfig, model config.ModelConfig) (string, error) {
	return ensureAgentPicoClawConfigForParticipantWithResolver(agentName, participantID, agentID, server, model, resolveManagerBaseURL)
}

func ensureAgentPicoClawConfigForParticipantWithResolver(agentName, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL picoclawsandbox.BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) (string, error) {
	agentHome, err := agentHomeDir(agentID)
	if err != nil {
		return "", err
	}
	return ensureAgentPicoClawConfigAtHome(agentHome, participantID, agentID, server, model, resolveBaseURL, feishuProviders...)
}

func (s *Controller) ensureAgentPicoClawConfigForParticipantWithResolver(agentName, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL picoclawsandbox.BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) (string, error) {
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return "", err
	}
	return ensureAgentPicoClawConfigAtHome(agentHome, participantID, agentID, server, model, resolveBaseURL, feishuProviders...)
}

func ensureAgentPicoClawConfigAtHome(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL picoclawsandbox.BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) (string, error) {
	return picoclawsandbox.EnsureConfig(agentHome, participantID, agentID, server, model, resolveBaseURL, feishuProviders...)
}

func managerPicoClawRoot() (string, error) {
	return agentPicoClawRoot(ManagerUserID)
}

func agentWorkspacePicoClawConfigRoot(agentName string) (string, error) {
	return agentPicoClawRoot(agentName)
}

func agentPicoClawRoot(agentID string) (string, error) {
	homeDir, err := agentHomeDir(agentID)
	if err != nil {
		return "", err
	}
	return picoclawRootForAgentHome(homeDir), nil
}

func picoclawRootForAgentHome(homeDir string) string {
	return picoclawsandbox.Root(homeDir)
}

func renderManagerPicoClawConfig(server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	return renderAgentPicoClawConfigForParticipant(ManagerParticipantID, ManagerUserID, server, model)
}

func renderAgentPicoClawConfig(agentID string, server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	return renderAgentPicoClawConfigForParticipant(agentID, agentID, server, model)
}

func renderAgentPicoClawConfigForParticipant(participantID, agentID string, server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	return picoclawsandbox.RenderConfig(participantID, agentID, server, model, resolveManagerBaseURL)
}

func picoclawBridgeModelID(modelID string) string {
	return picoclawsandbox.BridgeModelID(modelID)
}

func renderManagerSecurityConfig(server config.ServerConfig, model config.ModelConfig) string {
	return picoclawsandbox.RenderSecurityConfig(server, model)
}
