import { describe, expect, it } from "vitest";
import { WorkspacePaneTypes } from "@/models/routing";
import { agentSelectionAfterDelete, shouldReturnToAgentOverviewAfterAgentMissing } from "./useAgentController";

describe("shouldReturnToAgentOverviewAfterAgentMissing", () => {
  it("keeps the workspace on the agent overview when an agent disappears", () => {
    expect(
      shouldReturnToAgentOverviewAfterAgentMissing({
        type: WorkspacePaneTypes.agent,
        id: "u-test-agent",
      }),
    ).toBe(true);
  });

  it("does not redirect non-agent panes", () => {
    expect(
      shouldReturnToAgentOverviewAfterAgentMissing({
        type: WorkspacePaneTypes.conversation,
        id: "room-1",
      }),
    ).toBe(false);
  });
});

describe("agentSelectionAfterDelete", () => {
  const manager = { id: "u-manager", name: "Manager", role: "manager" };
  const alpha = { id: "u-alpha", name: "Alpha", role: "worker" };
  const beta = { id: "u-beta", name: "Beta", role: "worker" };

  it("selects the next ordinary agent", () => {
    expect(agentSelectionAfterDelete([manager, alpha, beta], alpha.id)?.id).toBe(beta.id);
  });

  it("selects the previous ordinary agent when the deleted agent was last", () => {
    expect(agentSelectionAfterDelete([manager, alpha, beta], beta.id)?.id).toBe(alpha.id);
  });

  it("falls back to Manager when no ordinary agent remains", () => {
    expect(agentSelectionAfterDelete([manager, alpha], alpha.id)?.id).toBe(manager.id);
  });
});
