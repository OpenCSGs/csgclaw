package api

import (
	"context"
	"fmt"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/channel/feishu"
	feishubinding "csgclaw/internal/channel/feishu/binding"
	"csgclaw/internal/participant"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

type nativeTestCredentials struct{ participants *participant.Service }

func (p *nativeTestCredentials) BotConfigForAgent(id string) (string, feishu.AppConfig, bool) {
	items := p.participants.List(participant.ListOptions{Channel: participant.ChannelFeishu, Type: participant.TypeAgent, AgentID: id})
	if len(items) == 0 {
		return "", feishu.AppConfig{}, false
	}
	item := items[0]
	return item.ID, feishu.AppConfig{AppID: channelAppConfigString(item.ChannelAppConfig, "app_id"), AppSecret: channelAppConfigString(item.ChannelAppConfig, "app_secret")}, true
}

type noNativeHostedWorker struct{}

func (noNativeHostedWorker) NewWorker(feishubinding.Resolved) (feishubinding.Worker, error) {
	return nil, fmt.Errorf("sandbox-native channels must not create a hosted worker")
}

type nativeTestBindings struct{ manager *feishubinding.Manager }

func (b nativeTestBindings) RefreshAgentChannel(ctx context.Context, _ agent.Agent, _ string) error {
	return b.manager.Reconcile(ctx)
}

type startedNativeSandbox struct{ *sandboxtest.Runtime }

func (r startedNativeSandbox) Create(ctx context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	instance, err := r.Runtime.Create(ctx, spec)
	if err != nil {
		return nil, err
	}
	if err := instance.Start(ctx); err != nil {
		return nil, err
	}
	return instance, nil
}

// Exercise HTTP, the Engine, real sandbox Runtime adapters and their on-disk
// configuration. Only the container provider is replaced, so no Bot/model calls
// or installed Docker/BoxLite runtime are needed.
