package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestTurnModelCompactionUsesLoadedModelForCurrentTurn(t *testing.T) {
	for _, overridden := range []bool{false, true} {
		t.Run(fmt.Sprint(overridden), func(t *testing.T) {
			profile := agentruntime.Profile{Provider: "api", ModelID: "small", ReasoningEffort: "high", ModelMetadata: modelcap.Resolved{ContextWindow: 32000}}
			live := &liveSession{spec: SessionSpec{AgentID: canonicalRuntimeAgentID("alice"), RuntimeID: "runtime", Profile: profile, ModelCatalog: []codexmodel.Profile{{Provider: "api", ModelID: "large", Metadata: modelcap.Resolved{ContextWindow: 64000}}, {Provider: "other", ModelID: "other-provider"}}}, session: &Session{SessionID: "thread"}, threadModels: map[string]contract.TurnModel{"thread": {ModelID: "large", ReasoningEffort: "high"}}}
			manager := newAppServerManager(managerDeps{})
			manager.sessions["runtime"] = live
			rt := New(Dependencies{Manager: manager})
			waiter, err := live.registerAppServerTurnWaiterWithModel("thread", turnModelFromProfile(profile), overridden)
			if err != nil {
				t.Fatal(err)
			}
			waiter.setTurnID("turn")
			compaction := map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]any{"id": "compact", "type": "contextCompaction"}}
			manager.handleRawAppServerNotification("runtime", live, "item/started", compaction)
			got, ok, err := rt.TurnModel("alice", "thread", "turn", "large")
			if err != nil || !ok || got.ModelID != "large" || got.Metadata.ContextWindow != 64000 || got.ReasoningEffort != "high" {
				t.Fatalf("compaction model=%+v override=%t error=%v", got, ok, err)
			}
			for _, model := range []string{"missing", "other-provider"} {
				if _, _, err := rt.TurnModel("alice", "thread", "turn", model); err == nil {
					t.Fatalf("accepted compaction model %q", model)
				}
			}
			if _, ok, err := rt.TurnModel("alice", "thread", "old-turn", "large"); err != nil || ok {
				t.Fatalf("stale turn override=%t error=%v", ok, err)
			}
			manager.handleRawAppServerNotification("runtime", live, "item/completed", compaction)
			got, ok, err = rt.TurnModel("alice", "thread", "turn", "small")
			if err != nil || ok != overridden || got.ModelID != "small" {
				t.Fatalf("ordinary model=%+v override=%t error=%v", got, ok, err)
			}
			manager.handleRawAppServerNotification("runtime", live, "item/started", compaction)
			live.removeAppServerTurnWaiter("thread", waiter)
			next, err := live.registerAppServerTurnWaiter("thread")
			if err != nil {
				t.Fatal(err)
			}
			next.setTurnID("next-turn")
			manager.handleRawAppServerNotification("runtime", live, "item/started", compaction)
			if next.compacting {
				t.Fatal("previous compaction changed the next turn")
			}
		})
	}
}

func TestTurnModelCompactionPreservesAcceptedEffort(t *testing.T) {
	for _, test := range []struct {
		name, previousModel, previousEffort, wantEffort string
		profileModel, rejectPrevious                    bool
	}{
		{name: "previous_override", previousModel: "large", previousEffort: "high", wantEffort: "high"},
		{name: "previous_auto", previousModel: "large", previousEffort: "auto", wantEffort: "auto"},
		{name: "previous_default", previousModel: "large", wantEffort: ""},
		{name: "previous_profile_override", previousModel: "large", previousEffort: "high", wantEffort: "high", profileModel: true},
		{name: "rejected_override", previousModel: "large", previousEffort: "high", wantEffort: "high", rejectPrevious: true},
		{name: "no_previous_selection", wantEffort: "auto"},
		{name: "different_previous_model", previousModel: "third", previousEffort: "max", wantEffort: "auto"},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := agentruntime.Profile{Provider: "api", ModelID: "small", ReasoningEffort: "none", ModelMetadata: modelcap.Resolved{ContextWindow: 32000}}
			large := contract.TurnModel{ModelID: "large", ReasoningEffort: "auto", Metadata: modelcap.Resolved{ContextWindow: 64000}}
			if test.profileModel {
				profile.ModelID, profile.ReasoningEffort, profile.ModelMetadata = large.ModelID, "low", large.Metadata
			}
			live := &liveSession{spec: SessionSpec{AgentID: canonicalRuntimeAgentID("alice"), Profile: profile, ModelCatalog: []codexmodel.Profile{{Provider: "api", ModelID: large.ModelID, Metadata: large.Metadata}}}}
			manager := newAppServerManager(managerDeps{})
			manager.sessions["runtime"] = live
			rt := New(Dependencies{Manager: manager})
			if test.previousModel != "" {
				previous := large
				previous.ModelID, previous.ReasoningEffort = test.previousModel, test.previousEffort
				waiter, err := live.registerAppServerTurnWaiterWithModel("thread", previous, true)
				if err != nil {
					t.Fatal(err)
				}
				waiter.setTurnID("previous")
				live.removeAppServerTurnWaiter("thread", waiter)
			}
			if test.rejectPrevious {
				rejected := large
				rejected.ReasoningEffort = "none"
				waiter, err := live.registerAppServerTurnWaiterWithModel("thread", rejected, true)
				if err != nil {
					t.Fatal(err)
				}
				live.removeAppServerTurnWaiter("thread", waiter)
			}
			other, err := live.registerAppServerTurnWaiterWithModel("other-thread", contract.TurnModel{ModelID: "large", ReasoningEffort: "max"}, true)
			if err != nil {
				t.Fatal(err)
			}
			other.setTurnID("other-turn")
			live.removeAppServerTurnWaiter("other-thread", other)
			selected := contract.TurnModel{ModelID: "small", ReasoningEffort: "none", Metadata: modelcap.Resolved{ContextWindow: 32000}}
			waiter, err := live.registerAppServerTurnWaiterWithModel("thread", selected, true)
			if err != nil {
				t.Fatal(err)
			}
			waiter.setTurnID("current")
			live.setTurnCompacting("thread", "current", "item/started")
			got, overridden, err := rt.TurnModel("alice", "thread", "current", "large")
			if err != nil || !overridden || got.ModelID != large.ModelID || got.Metadata != large.Metadata.Normalized() || got.ReasoningEffort != test.wantEffort {
				t.Fatalf("compaction selection = %+v, overridden = %t, error = %v; want effort %q", got, overridden, err, test.wantEffort)
			}
			live.setTurnCompacting("thread", "current", "item/completed")
			got, overridden, err = rt.TurnModel("alice", "thread", "current", "small")
			if err != nil || !overridden || got != selected || waiter.model != selected {
				t.Fatalf("answer selection changed: %+v, overridden = %t, error = %v", got, overridden, err)
			}
		})
	}
}

func TestTurnModelSynchronizesPendingCompactionNotification(t *testing.T) {
	for _, outcome := range []string{"compaction", "unknown_compaction", "ordinary", "unknown_ordinary", "canceled", "replaced", "read_error"} {
		t.Run(outcome, func(t *testing.T) {
			profile := agentruntime.Profile{Provider: "api", ModelID: "small", ReasoningEffort: "high"}
			live := &liveSession{spec: SessionSpec{AgentID: canonicalRuntimeAgentID("alice"), Profile: profile, ModelCatalog: []codexmodel.Profile{{Provider: "api", ModelID: "large", Metadata: modelcap.Resolved{ContextWindow: 64000}}}}}
			manager := newAppServerManager(managerDeps{})
			manager.sessions["runtime"] = live
			rt := New(Dependencies{Manager: manager})
			waiter, err := live.registerAppServerTurnWaiter("thread")
			if err != nil {
				t.Fatal(err)
			}
			waiter.setTurnID("turn")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			live.setAppServerTurnContext("thread", waiter, ctx)
			requests := make(chan map[string]any, 1)
			live.appClient = newAppServerClient(turnModelWriter(func(data []byte) (int, error) {
				var request map[string]any
				if err := json.Unmarshal(data, &request); err != nil {
					return 0, err
				}
				requests <- request
				return len(data), nil
			}), nil)
			type result struct {
				model contract.TurnModel
				ok    bool
				err   error
			}
			done := make(chan result, 1)
			go func() {
				requested := "large"
				if outcome == "unknown_compaction" || outcome == "unknown_ordinary" {
					requested = "missing"
				}
				model, ok, err := rt.TurnModel("alice", "thread", "turn", requested)
				done <- result{model, ok, err}
			}()
			var request map[string]any
			select {
			case request = <-requests:
			case got := <-done:
				t.Fatalf("model resolved before read: %+v", got)
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
			if request["method"] != "thread/read" {
				t.Fatalf("synchronization method=%v", request["method"])
			}
			select {
			case got := <-done:
				t.Fatalf("resolved model before pending notification: %+v", got)
			default:
			}
			switch outcome {
			case "compaction", "unknown_compaction":
				live.setTurnCompacting("thread", "turn", "item/started")
			case "canceled":
				cancel()
			case "replaced":
				live.removeAppServerTurnWaiter("thread", waiter)
				next, err := live.registerAppServerTurnWaiter("thread")
				if err != nil {
					t.Fatal(err)
				}
				next.setTurnID("next-turn")
			}
			if outcome == "read_error" {
				live.appClient.handleLine(fmt.Sprintf(`{"id":%v,"error":{"code":-32000,"message":"read failed"}}`, request["id"]))
			} else if outcome != "canceled" {
				live.appClient.handleLine(fmt.Sprintf(`{"id":%v,"result":{}}`, request["id"]))
			}
			got := <-done
			switch outcome {
			case "compaction":
				if got.err != nil || !got.ok || got.model.ModelID != "large" {
					t.Fatalf("compaction result=%+v", got)
				}
			case "canceled":
				if !errors.Is(got.err, context.Canceled) {
					t.Fatalf("cancellation result=%+v", got)
				}
			case "read_error", "unknown_compaction":
				if got.err == nil {
					t.Fatal("read failure selected a model")
				}
			default:
				if got.err != nil || got.ok {
					t.Fatalf("ordinary or replaced turn result=%+v", got)
				}
				if outcome != "replaced" && got.model.ModelID != "small" {
					t.Fatalf("ordinary request changed selected model: %+v", got)
				}
			}
		})
	}
}

type turnModelWriter func([]byte) (int, error)

func (write turnModelWriter) Write(data []byte) (int, error) { return write(data) }

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
	if got, ok, err := rt.TurnModel("alice", "thread-one", "first-native-turn", selected.ModelID); err != nil || !ok || !reflect.DeepEqual(got, selected) {
		t.Fatalf("early model request = %+v, %v", got, ok)
	}
	if waiter.currentTurnID() != "" {
		t.Fatal("HTTP lookup established native turn identity")
	}
	waiter.setTurnID("first-native-turn")
	for _, identity := range [][3]string{{"other-agent", "thread-one", "first-native-turn"}, {"alice", "other-thread", "first-native-turn"}, {"alice", "thread-one", "old-turn"}} {
		if _, ok, err := rt.TurnModel(identity[0], identity[1], identity[2], selected.ModelID); err != nil || ok {
			t.Fatalf("selection escaped its native identity: %v", identity)
		}
	}
	other, err := live.registerAppServerTurnWaiter("thread-two")
	if err != nil {
		t.Fatal(err)
	}
	other.setTurnID("second-native-turn")
	if _, ok, err := rt.TurnModel("alice", "thread-two", "second-native-turn", profile.ModelID); err != nil || ok {
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
	if _, ok, err := rt.TurnModel("alice", "thread-one", "first-native-turn", selected.ModelID); err != nil || ok {
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
