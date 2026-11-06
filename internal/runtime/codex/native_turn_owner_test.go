package codex

import (
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelusage"
)

func TestNativeTurnModelAndUsageFindOwningRuntime(t *testing.T) {
	manager := newAppServerManager(managerDeps{EventSink: &recordingSink{}})
	rt := New(Dependencies{Manager: manager})
	add := func(runtimeID, agentID, threadID, turnID, modelID string) *appServerTurnWaiter {
		t.Helper()
		live := &liveSession{spec: SessionSpec{RuntimeID: runtimeID, AgentID: canonicalRuntimeAgentID(agentID)}}
		waiter, err := live.registerAppServerTurnWaiterWithModel(threadID, contract.TurnModel{ModelID: modelID}, true)
		if err != nil {
			t.Fatal(err)
		}
		waiter.setTurnID(turnID)
		manager.sessions[runtimeID] = live
		return waiter
	}
	first := add("runtime-one", "alice", "thread-one", "turn-one", "model-one")
	second := add("runtime-two", "alice", "thread-two", "turn-two", "model-two")
	foreign := add("runtime-other-agent", "bob", "thread-two", "turn-two", "model-other")
	for _, request := range []struct{ thread, turn, model string }{
		{"thread-one", "turn-one", "model-one"},
		{"thread-two", "turn-two", "model-two"},
	} {
		got, ok, err := rt.TurnModel("alice", request.thread, request.turn, request.model)
		if err != nil || !ok || got.ModelID != request.model {
			t.Fatalf("model for %s/%s = %+v, %t, %v", request.thread, request.turn, got, ok, err)
		}
		rt.ObserveModelResponseUsage("alice", request.thread, request.turn, &modelusage.Usage{Known: true, Input: 10, Output: 1, Total: 11})
	}
	for _, waiter := range []*appServerTurnWaiter{first, second} {
		if waiter.tokenUsage.Total != 11 || waiter.tokenUsage.ModelID != waiter.model.ModelID {
			t.Fatalf("owner usage=%+v", waiter.tokenUsage)
		}
	}
	if foreign.tokenUsage.Total != 0 {
		t.Fatalf("another Agent received usage: %+v", foreign.tokenUsage)
	}
	for _, identity := range [][3]string{{"alice", "thread-two", "stale"}, {"alice", "missing", "turn-two"}, {"missing-agent", "thread-two", "turn-two"}} {
		if _, ok, err := rt.TurnModel(identity[0], identity[1], identity[2], "model-two"); err != nil || ok {
			t.Fatalf("unowned model lookup %v = %t, %v", identity, ok, err)
		}
		rt.ObserveModelResponseUsage(identity[0], identity[1], identity[2], &modelusage.Usage{Known: true, Input: 100, Total: 100})
	}
	if first.tokenUsage.Total != 11 || second.tokenUsage.Total != 11 || foreign.tokenUsage.Total != 0 {
		t.Fatal("unowned observations changed usage")
	}
}

func TestNativeTurnOwnerRejectsAmbiguousRuntimes(t *testing.T) {
	for _, secondTurn := range []string{"turn", ""} {
		t.Run("second_turn_"+secondTurn, func(t *testing.T) {
			manager := newAppServerManager(managerDeps{})
			rt := New(Dependencies{Manager: manager})
			var waiters []*appServerTurnWaiter
			for index, runtimeID := range []string{"runtime-one", "runtime-two"} {
				live := &liveSession{spec: SessionSpec{RuntimeID: runtimeID, AgentID: canonicalRuntimeAgentID("alice")}}
				waiter, err := live.registerAppServerTurnWaiterWithModel("thread", contract.TurnModel{ModelID: runtimeID}, true)
				if err != nil {
					t.Fatal(err)
				}
				turnID := "turn"
				if index == 1 {
					turnID = secondTurn
				}
				waiter.setTurnID(turnID)
				waiters = append(waiters, waiter)
				manager.sessions[runtimeID] = live
			}
			if _, ok, err := rt.TurnModel("alice", "thread", "turn", "runtime-one"); err != nil || ok {
				t.Fatalf("ambiguous lookup=%t, %v", ok, err)
			}
			rt.ObserveModelResponseUsage("alice", "thread", "turn", &modelusage.Usage{Known: true, Input: 10, Total: 10})
			for _, waiter := range waiters {
				if waiter.tokenUsage.Total != 0 || len(waiter.pendingUsage) != 0 {
					t.Fatalf("ambiguous usage=%+v pending=%+v", waiter.tokenUsage, waiter.pendingUsage)
				}
			}
			waiters[1].setTurnID("other-turn")
			got, ok, err := rt.TurnModel("alice", "thread", "turn", "runtime-one")
			if err != nil || !ok || got.ModelID != "runtime-one" {
				t.Fatalf("confirmed owner=%+v, %t, %v", got, ok, err)
			}
		})
	}
}

func TestNativeTurnOwnerPreservesPendingStart(t *testing.T) {
	manager := newAppServerManager(managerDeps{})
	rt := New(Dependencies{Manager: manager})
	manager.sessions["other-runtime"] = &liveSession{spec: SessionSpec{AgentID: canonicalRuntimeAgentID("alice")}}
	live := &liveSession{spec: SessionSpec{AgentID: canonicalRuntimeAgentID("alice")}}
	manager.sessions["owner"] = live
	waiter, err := live.registerAppServerTurnWaiterWithModel("thread", contract.TurnModel{ModelID: "selected"}, true)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := rt.TurnModel("alice", "thread", "native-turn", "selected")
	if err != nil || !ok || got.ModelID != "selected" || waiter.currentTurnID() != "" {
		t.Fatalf("pending model=%+v, %t, %v", got, ok, err)
	}
	rt.ObserveModelResponseUsage("alice", "thread", "native-turn", &modelusage.Usage{Known: true, Input: 10, Total: 10})
	if waiter.tokenUsage.Total != 0 || waiter.pendingUsage["native-turn"].Total != 10 || waiter.currentTurnID() != "" {
		t.Fatalf("pending usage=%+v, buffered=%+v", waiter.tokenUsage, waiter.pendingUsage)
	}
}
