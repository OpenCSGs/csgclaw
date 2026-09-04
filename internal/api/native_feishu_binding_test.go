package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/channel/feishu"
	feishubinding "csgclaw/internal/channel/feishu/binding"
	"csgclaw/internal/participant"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
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
func TestNativeFeishuBindingsReconcileRuntimeConfigurationThroughHTTP(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindOpenClawSandbox, agent.RuntimeKindPicoClawSandbox} {
		t.Run(kind, func(t *testing.T) {
			target := completeWorkerAgent("agent-native", "native")
			target.RuntimeKind = kind
			credentials := &nativeTestCredentials{}
			adapter := runtimewiring.WithOpenClawSandboxRuntime(credentials)
			if kind == agent.RuntimeKindPicoClawSandbox {
				adapter = runtimewiring.WithPicoClawSandboxRuntime(credentials)
			}
			containers := sandboxtest.NewProvider()
			nativeRuntime := startedNativeSandbox{sandboxtest.NewRuntime()}
			containers.OpenFunc = func(context.Context, string) (sandbox.Runtime, error) { return nativeRuntime, nil }
			controller, statePath := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{target}, agent.WithSandboxProvider(containers), adapter)
			t.Cleanup(func() { _ = controller.Close() })
			engine := agentengine.New(controller)
			credentials.participants = participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(engine))
			manager, err := feishubinding.NewManager(feishubinding.ManagerOptions{Agents: engine.Agents(), Provider: credentials, Workers: noNativeHostedWorker{}})
			if err != nil {
				t.Fatal(err)
			}
			h := NewHandler(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, nil, nil, nil, nil, nil)
			h.SetParticipantService(credentials.participants)
			h.SetChannelBindingReconciler(nativeTestBindings{manager})
			router := h.Routes()
			home := filepath.Join(filepath.Dir(statePath), "agents", target.ID)
			configPath := filepath.Join(openclawsandbox.Root(home), openclawsandbox.HostConfig)
			if kind == agent.RuntimeKindPicoClawSandbox {
				configPath = filepath.Join(picoclawsandbox.Root(home), picoclawsandbox.HostConfig)
			}
			request := func(method, path, body string, want int) {
				t.Helper()
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
				if w.Code != want {
					t.Fatalf("%s %s: HTTP %d, want %d: %s", method, path, w.Code, want, w.Body)
				}
			}
			checkConfig := func(present, absent string) {
				t.Helper()
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatal(err)
				}
				if present != "" && !strings.Contains(string(data), present) {
					t.Fatal("Runtime configuration did not receive the current Bot")
				}
				if absent != "" && strings.Contains(string(data), absent) {
					t.Fatal("Runtime configuration retained the previous Bot")
				}
				item, err := engine.Agents().Get(context.Background(), target.ID, agentengine.AgentGetOptions{ProbeRuntime: true})
				if err != nil || item.Status.State != agentengine.AgentStateRunning {
					t.Fatalf("Runtime was not restored: %+v %v", item.Status, err)
				}
			}
			request(http.MethodPost, "/api/v1/channels/feishu/participants", `{"id":"pt-native","type":"agent","name":"native","channel_user":{"kind":"app_id"},"agent_binding":{"mode":"reuse","agent_id":"agent-native"},"channel_app_config":{"app_id":"cli_first","app_secret":"first-test-secret"}}`, http.StatusCreated)
			checkConfig("cli_first", "cli_second")
			request(http.MethodPatch, "/api/v1/channels/feishu/participants/pt-native", `{"channel_app_config":{"app_id":"cli_second","app_secret":"second-test-secret"}}`, http.StatusOK)
			checkConfig("cli_second", "cli_first")
			request(http.MethodDelete, "/api/v1/channels/feishu/participants/pt-native", "", http.StatusNoContent)
			checkConfig("", "cli_second")
		})
	}
}
