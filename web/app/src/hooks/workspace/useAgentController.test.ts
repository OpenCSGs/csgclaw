import { describe, expect, it } from "vitest";
import { WorkspacePaneTypes } from "@/models/routing";
import { shouldReturnToAgentOverviewAfterAgentMissing } from "./useAgentController";

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
