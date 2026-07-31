import {
  WORKSPACE_AGENTS_SETTLE_POLL_INTERVAL_MS,
  WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS,
  WORKSPACE_AGENTS_STARTUP_POLL_WINDOW_MS,
  workspaceAgentsStartupRefetchInterval,
} from "@/hooks/workspace/workspaceQueries";
import type { AgentLike } from "@/models/agents";

const manager = (status: string): AgentLike => ({
  id: "u-manager",
  name: "manager",
  role: "manager",
  runtime_kind: "codex",
  status,
});

const worker = (status: string): AgentLike => ({
  id: "u-worker",
  name: "worker",
  role: "worker",
  runtime_kind: "picoclaw_sandbox",
  status,
});

describe("workspaceAgentsStartupRefetchInterval", () => {
  it("polls every 1.5 seconds until the manager is running", () => {
    expect(workspaceAgentsStartupRefetchInterval(undefined, 0)).toBe(WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS);
    expect(workspaceAgentsStartupRefetchInterval([manager("stopped")], 60_000)).toBe(
      WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS,
    );
  });

  it("keeps refreshing the complete agent list after the manager starts", () => {
    expect(workspaceAgentsStartupRefetchInterval([manager("running"), worker("stopped")], 1_000)).toBe(
      WORKSPACE_AGENTS_SETTLE_POLL_INTERVAL_MS,
    );
  });

  it("stops after the two-minute startup window", () => {
    expect(workspaceAgentsStartupRefetchInterval([manager("stopped")], WORKSPACE_AGENTS_STARTUP_POLL_WINDOW_MS)).toBe(
      false,
    );
  });
});
