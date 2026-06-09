package onboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

var (
	loadIMBootstrap      = im.LoadBootstrap
	openParticipantStore = participant.NewStore
	openAgentState       = func(cfg config.Config, path, managerImage string) (agentStateReader, error) {
		return agent.NewServiceWithLLM(
			effectiveLLMConfig(cfg),
			cfg.Server,
			managerImage,
			path,
			runtimewiring.WithPicoClawSandboxRuntime(nil),
			runtimewiring.WithOpenClawSandboxRuntime(nil),
			agent.WithGatewayRuntime(config.RuntimeKindPicoClawSandbox),
			agent.WithBootstrapDefaultTemplates(cfg.Bootstrap),
		)
	}
)

type agentStateReader interface {
	Agent(id string) (agent.Agent, bool)
}

type DetectStateOptions struct {
	ConfigPath string
}

type DetectStateResult struct {
	ConfigPath                 string
	Config                     config.Config
	ConfigExists               bool
	ConfigComplete             bool
	IMBootstrapComplete        bool
	ManagerAgentComplete       bool
	AdminParticipantComplete   bool
	ManagerParticipantComplete bool
}

func (r DetectStateResult) Complete() bool {
	return r.ConfigExists &&
		r.ConfigComplete &&
		r.IMBootstrapComplete &&
		r.ManagerAgentComplete &&
		r.AdminParticipantComplete &&
		r.ManagerParticipantComplete
}

func DetectState(opts DetectStateOptions) (DetectStateResult, error) {
	path, err := configPath(opts.ConfigPath)
	if err != nil {
		return DetectStateResult{}, err
	}

	cfg, hasExistingConfig, err := loadConfig(path)
	if err != nil {
		return DetectStateResult{}, err
	}
	if !hasExistingConfig {
		cfg = defaultConfig()
	}

	result := DetectStateResult{
		ConfigPath:   path,
		Config:       cfg,
		ConfigExists: hasExistingConfig,
	}
	if hasExistingConfig {
		data, err := os.ReadFile(path)
		if err != nil {
			return DetectStateResult{}, fmt.Errorf("read config: %w", err)
		}
		result.ConfigComplete = !configNeedsCompletion(string(data))
	}

	agentsPath, imStatePath, err := bootstrapPaths()
	if err != nil {
		return DetectStateResult{}, err
	}

	imState, err := loadIMBootstrap(imStatePath)
	if err != nil {
		return DetectStateResult{}, err
	}
	result.IMBootstrapComplete = imBootstrapComplete(imState)

	hubSvc, err := hub.NewService(cfg.Hub, hub.DefaultStoreFactory)
	if err != nil {
		return DetectStateResult{}, err
	}
	bootstrapDefaults, err := hub.ResolveBootstrapDefaults(context.Background(), cfg.Bootstrap, hubSvc)
	if err != nil {
		return DetectStateResult{}, err
	}

	agentState, err := openAgentState(cfg, agentsPath, bootstrapDefaults.ManagerImage)
	if err != nil {
		return DetectStateResult{}, err
	}
	result.ManagerAgentComplete = managerAgentComplete(agentState)

	store, err := openParticipantStore(filepath.Join(filepath.Dir(imStatePath), "participants.json"))
	if err != nil {
		return DetectStateResult{}, err
	}
	participants := store.List(participant.ListOptions{Channel: participant.ChannelCSGClaw})
	result.AdminParticipantComplete = adminParticipantComplete(participants)
	result.ManagerParticipantComplete = managerParticipantComplete(participants)

	return result, nil
}

func imBootstrapComplete(state im.Bootstrap) bool {
	state = im.NewServiceFromBootstrap(state).Bootstrap()
	if len(state.InviteDraftUserIDs) > 0 {
		return false
	}
	if !hasIMUser(state.Users, im.AdminUserID, "admin", "admin") {
		return false
	}
	if !hasIMUser(state.Users, agent.ManagerParticipantID, "manager", "manager") {
		return false
	}
	for _, room := range state.Rooms {
		if room.IsDirect &&
			len(room.Members) == 2 &&
			containsMember(room.Members, im.AdminUserID) &&
			containsMember(room.Members, agent.ManagerParticipantID) {
			return true
		}
	}
	return false
}

func hasIMUser(users []im.User, id, handle, role string) bool {
	for _, user := range users {
		if strings.TrimSpace(user.ID) != id {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(user.Handle), handle) {
			return false
		}
		if !strings.EqualFold(strings.TrimSpace(user.Role), role) {
			return false
		}
		return true
	}
	return false
}

func containsMember(members []string, id string) bool {
	for _, member := range members {
		if strings.TrimSpace(member) == id {
			return true
		}
	}
	return false
}

func managerAgentComplete(state agentStateReader) bool {
	if state == nil {
		return false
	}
	managerAgent, ok := state.Agent(agent.ManagerUserID)
	if !ok {
		return false
	}
	if strings.TrimSpace(managerAgent.ID) != agent.ManagerUserID {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(managerAgent.Name), agent.ManagerName) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(managerAgent.Role), agent.RoleManager)
}

func adminParticipantComplete(items []participant.Participant) bool {
	for _, item := range items {
		if strings.TrimSpace(item.Channel) != participant.ChannelCSGClaw {
			continue
		}
		if strings.TrimSpace(item.ID) != im.AdminUserID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Type), participant.TypeHuman) {
			return false
		}
		if strings.TrimSpace(item.AgentID) != "" {
			return false
		}
		return strings.TrimSpace(item.ChannelUserRef) == im.AdminUserID
	}
	return false
}

func managerParticipantComplete(items []participant.Participant) bool {
	for _, item := range items {
		if strings.TrimSpace(item.Channel) != participant.ChannelCSGClaw {
			continue
		}
		if strings.TrimSpace(item.ID) != agent.ManagerParticipantID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Type), participant.TypeAgent) {
			return false
		}
		if strings.TrimSpace(item.AgentID) != agent.ManagerUserID {
			return false
		}
		return strings.TrimSpace(item.ChannelUserRef) != ""
	}
	return false
}
