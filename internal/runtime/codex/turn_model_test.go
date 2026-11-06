package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelusage"
	agentruntime "csgclaw/internal/runtime"
)

func TestTurnModelIsScopedToNativeThreadAndTurn(t *testing.T) {
	profile := agentruntime.Profile{Provider: "api", ModelID: "default-model", ReasoningEffort: "auto", ModelMetadata: modelcap.Resolved{ContextWindow: 32000, ContextSource: "provider"}}
	selected := contract.TurnModel{ModelID: "request-model", ReasoningEffort: "high", Metadata: modelcap.Resolved{ContextWindow: 64000, ContextSource: "provider"}}
	live := &liveSession{spec: SessionSpec{AgentID: canonicalRuntimeAgentID("alice"), RuntimeID: "runtime", Profile: profile, ModelCatalog: []codexmodel.Profile{{Provider: profile.Provider, ModelID: selected.ModelID, Metadata: selected.Metadata}}}}
	manager := newAppServerManager(managerDeps{EventSink: &recordingSink{}})
	manager.sessions["runtime"] = live
	rt := New(Dependencies{Manager: manager})
	resolved, err := live.resolveTurnModel(&selected)
	if err != nil || !reflect.DeepEqual(resolved, selected) {
		t.Fatalf("resolved selection = %+v, %v", resolved, err)
	}
	waiter, err := live.registerAppServerTurnWaiterWithModel("thread-one", resolved, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := rt.TurnModel("alice", "thread-one", "first-native-turn"); !ok || !reflect.DeepEqual(got, selected) {
		t.Fatalf("early model request = %+v, %v", got, ok)
	}
	if waiter.currentTurnID() != "" {
		t.Fatal("HTTP lookup established native turn identity")
	}
	waiter.setTurnID("first-native-turn")
	for _, identity := range [][3]string{{"other-agent", "thread-one", "first-native-turn"}, {"alice", "other-thread", "first-native-turn"}, {"alice", "thread-one", "old-turn"}} {
		if _, ok := rt.TurnModel(identity[0], identity[1], identity[2]); ok {
			t.Fatalf("selection escaped its native identity: %v", identity)
		}
	}
	other, err := live.registerAppServerTurnWaiter("thread-two")
	if err != nil {
		t.Fatal(err)
	}
	other.setTurnID("second-native-turn")
	if _, ok := rt.TurnModel("alice", "thread-two", "second-native-turn"); ok {
		t.Fatal("default turn supplied a proxy override")
	}
	manager.observeModelResponseUsage(live, "thread-one", "first-native-turn", &modelusage.Usage{Known: true, ModelID: "upstream-model", Input: 100, Output: 20, Total: 120})
	if waiter.tokenUsage.ModelID != selected.ModelID || waiter.tokenUsage.Total != 120 {
		t.Fatalf("request usage = %+v", waiter.tokenUsage)
	}
	manager.publishContextUsage("runtime", live, "thread-one", nil, "snapshot")
	manager.publishContextUsage("runtime", live, "thread-two", nil, "snapshot")
	if got := live.contextUsage["thread-one"]; got.ModelID != selected.ModelID || got.ContextWindow != 64000 {
		t.Fatalf("override context usage = %+v", got)
	}
	if got := live.contextUsage["thread-two"]; got.ModelID != profile.ModelID || got.ContextWindow != 32000 {
		t.Fatalf("default context usage = %+v", got)
	}
	live.removeAppServerTurnWaiter("thread-one", waiter)
	if _, ok := rt.TurnModel("alice", "thread-one", "first-native-turn"); ok {
		t.Fatal("completed turn still supplies an override")
	}
	manager.publishContextUsage("runtime", live, "thread-one", nil, "snapshot")
	if live.contextUsage["thread-one"].ModelID != selected.ModelID {
		t.Fatal("idle snapshot lost the completed turn's model")
	}
	next, err := live.registerAppServerTurnWaiter("thread-one")
	if err != nil {
		t.Fatal(err)
	}
	if next.model.ModelID != profile.ModelID || next.model.ReasoningEffort != "auto" || !reflect.DeepEqual(live.spec.Profile, profile) {
		t.Fatal("request selection changed Profile defaults")
	}
	manager.publishContextUsage("runtime", live, "thread-one", nil, "snapshot")
	if live.contextUsage["thread-one"].ContextWindow != 32000 {
		t.Fatal("next default turn retained the override context window")
	}
}

func TestTurnModelRequiresLoadedCatalogMetadata(t *testing.T) {
	live := &liveSession{spec: SessionSpec{Profile: agentruntime.Profile{ModelID: "model", ModelMetadata: modelcap.Resolved{ContextWindow: 32000}}}}
	for _, selection := range []contract.TurnModel{
		{ModelID: "missing", Metadata: modelcap.Resolved{ContextWindow: 32000}},
		{ModelID: "model", Metadata: modelcap.Resolved{ContextWindow: 64000}},
	} {
		if _, err := live.resolveTurnModel(&selection); err == nil {
			t.Fatalf("accepted unavailable metadata: %+v", selection)
		}
	}
}

func TestSessionModelCatalogPreservesDefaultAndOtherModelMetadata(t *testing.T) {
	profile := agentruntime.Profile{Provider: "api", ModelID: "default-model", ReasoningEffort: "auto", ModelMetadata: modelcap.Resolved{ContextWindow: 32000}}
	rt := New(Dependencies{ModelCatalog: func(agentID string) []codexmodel.Profile {
		if agentID != "alice" {
			t.Fatalf("catalog agent = %q", agentID)
		}
		return []codexmodel.Profile{{ModelID: "default-model", Metadata: modelcap.Resolved{ContextWindow: 999}}, {ModelID: "request-model", Metadata: modelcap.Resolved{ContextWindow: 64000}}, {ModelID: ""}}
	}})
	catalog := rt.sessionModelCatalog("alice", profile)
	root := t.TempDir()
	if err := rt.writeModelCatalog(root, profile, catalog...); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, modelCatalogFileName))
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Models []struct {
			Slug    string `json:"slug"`
			Window  int64  `json:"context_window"`
			Compact int64  `json:"auto_compact_token_limit"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Models) != 2 || stored.Models[0].Window != 32000 || stored.Models[1].Window != 64000 || stored.Models[1].Compact != 48000 {
		t.Fatalf("model catalog = %+v", stored)
	}
	profile.BaseURL = "http://127.0.0.1:1/v1"
	config := configureCodexHomeConfig("model_context_window = 1000\nmodel_auto_compact_token_limit = 750\n", profile, nil)
	parsed := parseProviderTestConfig(t, config)
	if parsed["model_context_window"] != nil || parsed["model_auto_compact_token_limit"] != nil {
		t.Fatal("process configuration overrides per-model catalog metadata")
	}
}
