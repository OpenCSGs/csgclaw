import { describe, expect, it } from "vitest";
import { WorkspacePaneTypes } from "@/models/routing";
import {
  agentSelectionAfterDelete,
  feishuRegistrationFinalizeNotice,
  larkCLIInitErrorKind,
  shouldReturnToAgentOverviewAfterAgentMissing,
} from "./useAgentController";

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

describe("larkCLIInitErrorKind", () => {
  it("maps actionable backend errors to focused user guidance", () => {
    expect(larkCLIInitErrorKind("feishu_bot_not_configured")).toBe("missing_bot");
    expect(larkCLIInitErrorKind("lark_cli_unavailable")).toBe("install");
    expect(larkCLIInitErrorKind("feishu_bot_app_id_conflict")).toBe("app_conflict");
    expect(larkCLIInitErrorKind("lark_cli_source_unavailable")).toBe("source_unavailable");
    expect(larkCLIInitErrorKind("lark_cli_bind_failed")).toBe("bind_failed");
    expect(larkCLIInitErrorKind("unexpected")).toBe("generic");
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

describe("feishuRegistrationFinalizeNotice", () => {
  it("surfaces structured lark-cli failures as warnings", () => {
    expect(
      feishuRegistrationFinalizeNotice({
        lark_cli_status: "error",
        lark_cli_error: { code: "lark_cli_bind_failed" },
      }),
    ).toEqual({ kind: "bind_failed", warnings: [] });
  });

  it("keeps backward-compatible warnings visible", () => {
    expect(feishuRegistrationFinalizeNotice({ warnings: [" first ", "", "second"] })).toEqual({
      kind: "warnings",
      warnings: ["first", "second"],
    });
  });
});
