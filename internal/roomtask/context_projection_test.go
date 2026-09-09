package roomtask

import (
	"strings"
	"testing"
)

func projectedContext(role TurnRole, scope, snapshot, turn string) PrivateTurnContext {
	return PrivateTurnContext{
		Role: role, PolicyID: TurnPolicyID(role),
		ScopeJSON: scope, SnapshotJSON: snapshot, TurnJSON: turn,
	}
}

func TestContextProjectorSendsFullThenOnlyChanges(t *testing.T) {
	projector := NewContextProjector()
	current := projectedContext(TurnRoleManager, `{"room_id":"room-a"}`, `{"tasks":[]}`, `{"source_message_id":"one"}`)

	first, err := projector.Project("conversation-a", current)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`mode="full"`, `room_type="on_demand"`, `role="manager"`, `policy_id="on-demand-manager/v1"`, `<turn-directive source="server" required="true">`, `ON-DEMAND ROOM MANAGER MODE IS ACTIVE`, `task submit, task plan, and task dispatch`, `kind="room-scope"`, `kind="task-snapshot"`, `"source_message_id":"one"`} {
		if !strings.Contains(first, want) {
			t.Fatalf("full projection missing %q:\n%s", want, first)
		}
	}

	current.TurnJSON = `{"source_message_id":"two"}`
	steady, err := projector.Project("conversation-a", current)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`mode="steady"`, `<turn-directive source="server" required="true">`, `ON-DEMAND ROOM MANAGER MODE IS ACTIVE`, `task submit, task plan, and task dispatch`, `kind="current-turn"`, `"source_message_id":"two"`} {
		if !strings.Contains(steady, want) {
			t.Fatalf("steady projection missing %q:\n%s", want, steady)
		}
	}
	for _, unwanted := range []string{"<policy-ref", "<policy>\n", "task review --task", `kind="room-scope"`, `kind="task-snapshot"`, `"source_message_id":"one"`} {
		if strings.Contains(steady, unwanted) {
			t.Fatalf("steady projection repeated %q:\n%s", unwanted, steady)
		}
	}

	current.SnapshotJSON = `{"tasks":[{"id":"task-1"}]}`
	current.TurnJSON = `{"source_message_id":"three"}`
	delta, err := projector.Project("conversation-a", current)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(delta, `mode="delta"`) || !strings.Contains(delta, `kind="task-snapshot"`) || strings.Contains(delta, `kind="room-scope"`) || strings.Contains(delta, "<policy>\n") {
		t.Fatalf("task delta = %s", delta)
	}
}

func TestContextProjectorResetIsolationAndInactiveScope(t *testing.T) {
	projector := NewContextProjector()
	current := projectedContext(TurnRoleWorker, `{"room_id":"room-a"}`, `{"task":{"id":"task-1"}}`, `{"source_message_id":"one"}`)
	if _, err := projector.Project("conversation-a", current); err != nil {
		t.Fatal(err)
	}
	current.TurnJSON = `{"source_message_id":"two"}`
	if got, _ := projector.Project("conversation-a", current); !strings.Contains(got, `mode="steady"`) {
		t.Fatal(got)
	}
	if got, _ := projector.Project("conversation-b", current); !strings.Contains(got, `mode="full"`) {
		t.Fatalf("independent conversation did not receive full context: %s", got)
	}

	projector.Reset("conversation-a")
	if got, _ := projector.Project("conversation-a", current); !strings.Contains(got, `mode="full"`) {
		t.Fatalf("reset did not restore full context: %s", got)
	}
	if got, err := projector.Project("conversation-a", PrivateTurnContext{}); err != nil || got != "" {
		t.Fatalf("inactive context = %q, %v", got, err)
	}
	if got, _ := projector.Project("conversation-a", current); !strings.Contains(got, `mode="full"`) {
		t.Fatalf("reactivated context did not receive full context: %s", got)
	}
}

func TestContextProjectorPeriodicallyRefreshesStableFacts(t *testing.T) {
	projector := NewContextProjector()
	projector.refreshTurns = 2
	current := projectedContext(TurnRoleManager, `{}`, `{}`, `{"turn":1}`)
	first, _ := projector.Project("conversation", current)
	current.TurnJSON = `{"turn":2}`
	second, _ := projector.Project("conversation", current)
	current.TurnJSON = `{"turn":3}`
	third, _ := projector.Project("conversation", current)
	if !strings.Contains(first, `mode="full"`) || !strings.Contains(second, `mode="steady"`) || !strings.Contains(third, `mode="full"`) {
		t.Fatalf("periodic refresh modes:\nfirst=%s\nsecond=%s\nthird=%s", first, second, third)
	}
}
