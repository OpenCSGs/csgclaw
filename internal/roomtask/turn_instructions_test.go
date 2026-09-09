package roomtask

import (
	"strings"
	"testing"
)

func TestOnDemandPolicyInstructionsAreRoleSpecific(t *testing.T) {
	manager := OnDemandPolicyInstructions(TurnRoleManager, "csgclaw-cli")
	worker := OnDemandPolicyInstructions(TurnRoleWorker, "csgclaw-cli")

	for _, want := range []string{`room_type="on_demand", role="manager", and policy_id="on-demand-manager/v1"`, "MUST create or continue tracked room work and dispatch it", "independent of domain, size, apparent simplicity", "Never silently switch to direct execution", "Before dispatch, do not use domain tools", "task submit", "task plan", "Split independent requested items", "dispatch", "Review", "task report", "csgclaw-cli"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager instructions missing %q", want)
		}
	}
	for _, unwanted := range []string{"task claim --task", "task update and status"} {
		if strings.Contains(manager, unwanted) {
			t.Fatalf("manager instructions contain Worker-only rule %q", unwanted)
		}
	}

	for _, want := range []string{`room_type="on_demand", role="worker", and policy_id="on-demand-worker/v1"`, "task claim --task", "task update", "pending Manager review", "csgclaw-cli"} {
		if !strings.Contains(worker, want) {
			t.Fatalf("worker instructions missing %q", want)
		}
	}
	for _, unwanted := range []string{"task submit", "task plan", "Review the exact", "Report the parent"} {
		if strings.Contains(worker, unwanted) {
			t.Fatalf("worker instructions contain Manager-only rule %q", unwanted)
		}
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

func TestOnDemandTurnDirectiveRepeatsTheImmediateRoleGate(t *testing.T) {
	manager := OnDemandTurnDirective(TurnRoleManager)
	for _, want := range []string{"ON-DEMAND ROOM MANAGER MODE IS ACTIVE", "not the direct executor", "task submit, task plan, and task dispatch", "Do not bypass this workflow", "Only greetings"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("Manager turn directive missing %q in %q", want, manager)
		}
	}
	if strings.Contains(manager, "GitHub") || strings.Contains(manager, "code review") {
		t.Fatalf("Manager turn directive contains domain-specific routing: %q", manager)
	}
	worker := OnDemandTurnDirective(TurnRoleWorker)
	if !strings.Contains(worker, "ON-DEMAND ROOM WORKER MODE IS ACTIVE") || !strings.Contains(worker, "Claim it before doing the work") {
		t.Fatalf("Worker turn directive is incomplete: %q", worker)
	}
	if got := OnDemandTurnDirective("unknown"); got != "" {
		t.Fatalf("unknown role turn directive = %q, want empty", got)
	}
}
