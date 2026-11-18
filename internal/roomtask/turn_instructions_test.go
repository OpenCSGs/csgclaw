package roomtask

import (
	"strings"
	"testing"
)

func TestOnDemandPolicyInstructionsAreRoleSpecific(t *testing.T) {
	manager := OnDemandPolicyInstructions(TurnRoleManager, "csgclaw-cli")
	worker := OnDemandPolicyInstructions(TurnRoleWorker, "csgclaw-cli")

	for _, want := range []string{`room_type="on_demand", role="manager", and policy_id="on-demand-manager/v1"`, "MUST create or continue tracked room work and dispatch it", "independent of domain, size, apparent simplicity", "Never silently switch to direct execution", "Before dispatch, do not use domain tools", "task submit --room <room_id>", "task plan --task", "task dispatch --task", "task review --task", "task report --task", "Split independent requested items", "csgclaw-cli"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager instructions missing %q", want)
		}
	}
	for _, unwanted := range []string{"task claim --task", "task update and status"} {
		if strings.Contains(manager, unwanted) {
			t.Fatalf("manager instructions contain Worker-only rule %q", unwanted)
		}
	}
	for _, want := range []string{"named independent reviewers", "explicit Worker children with dependencies", "Manager acceptance does not replace a guide-required independent review", "not just the children already created", "Do not invent additional mandatory reviews"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager guide handling missing %q", want)
		}
	}
	for _, want := range []string{"Independent goals in the same room may start immediately", "same Worker can execute independent children concurrently in separate task sessions", "adjust the existing plan in this turn", "room_task_dispatch with assigned_to", "If the user stopped the parent"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager replanning instructions missing %q", want)
		}
	}

	for _, want := range []string{`room_type="on_demand", role="worker", and policy_id="on-demand-worker/v1"`, "task claim --task", "task update --task", "task message --task", "pending Manager review", "independent review required by the guide is separate Worker work", "csgclaw-cli"} {
		if !strings.Contains(worker, want) {
			t.Fatalf("worker instructions missing %q", want)
		}
	}
	for _, unwanted := range []string{"task submit", "task plan", "Review the exact", "Report the parent"} {
		if strings.Contains(worker, unwanted) {
			t.Fatalf("worker instructions contain Manager-only rule %q", unwanted)
		}
	}
	for _, unwanted := range []string{"task claim --task <task_id> --actor-id", "task update --task <task_id> --actor-id", "task message --task <task_id> --actor-id", "task message --task <task_id> --target"} {
		if strings.Contains(worker, unwanted) {
			t.Fatalf("worker instructions contain redundant caller parameter %q", unwanted)
		}
	}
	if strings.Contains(manager, "task submit --room <room_id> --source-message <source_message_id> --actor-id") {
		t.Fatal("manager submit instructions should derive the requester from the source message")
	}
}

func TestOnDemandPolicyInstructionsDoNotEnableOtherCoordinationScopes(t *testing.T) {
	for _, role := range []TurnRole{TurnRoleManager, TurnRoleWorker} {
		got := OnDemandPolicyInstructions(role, "csgclaw-cli")
		for _, unwanted := range []string{"agent-teams", "participant list", "member list", "task list --room"} {
			if strings.Contains(got, unwanted) {
				t.Fatalf("%s instructions contain out-of-scope discovery %q", role, unwanted)
			}
		}
	}
	if got := OnDemandPolicyInstructions("unknown", "csgclaw-cli"); got != "" {
		t.Fatalf("unknown role instructions = %q, want empty", got)
	}
	if TurnPolicyID(TurnRoleManager) == TurnPolicyID(TurnRoleWorker) || TurnPolicyID("unknown") != "" {
		t.Fatal("role policy IDs must be distinct and reject unknown roles")
	}
	if got := OnDemandPolicySection(TurnRoleManager, "/safe/csgclaw-cli"); !strings.Contains(got, ManagerPolicyID) || !strings.Contains(got, `/safe/csgclaw-cli task submit`) {
		t.Fatalf("manager policy section did not bind policy ID and companion CLI: %s", got)
	}
}

func TestConversationInstructionsScopeTeamTasksToWorkerDispatch(t *testing.T) {
	manager := ConversationInstructions(TurnRoleManager, "csgclaw-cli")
	worker := ConversationInstructions(TurnRoleWorker, "csgclaw-cli")
	for _, want := range []string{"addressed server task_assigned event", `source_message_id starts with "team:"`, "team task claim --team", "team task update --team"} {
		if !strings.Contains(worker, want) || strings.Contains(manager, want) {
			t.Errorf("Team workflow scope for %q is incorrect", want)
		}
	}
	for _, want := range []string{"only way to schedule Worker participation", "Never use ordinary message_send", "already dispatched task"} {
		if !strings.Contains(manager, want) {
			t.Errorf("on-demand policy missing %q", want)
		}
	}
}

func TestAutonomousPolicyCoordinatesWorkThroughPublicMessages(t *testing.T) {
	for _, role := range []TurnRole{TurnRoleManager, TurnRoleWorker} {
		policy := ConversationInstructions(role, "csgclaw-cli")
		_, afterHeading, found := strings.Cut(policy, "### Autonomous room")
		scene, _, done := strings.Cut(afterHeading, "### Public room messages")
		if !found || !done {
			t.Fatal("autonomous policy section is missing")
		}
		for _, rule := range []string{
			"use message_send to post one short public message",
			"end this turn immediately with exactly <csgclaw-silent/>",
			"Do not send a waiting update",
			"Batch new unread notices and read them at a natural break",
			"defer to the first clear commitment in room message order",
			"yield the overlapping part at the next safe step",
		} {
			if !strings.Contains(scene, rule) {
				t.Errorf("%s policy is missing %q", role, rule)
			}
		}
		for _, tool := range []string{"autonomous_task", "task claim", "task submit", "room_task_dispatch"} {
			if strings.Contains(scene, tool) {
				t.Errorf("%s autonomous policy directs the agent to use %q", role, tool)
			}
		}
	}
}
