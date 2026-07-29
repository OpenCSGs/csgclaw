import {
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

describe("workspaceAgentsStartupRefetchInterval", () => {
  it("polls every 1.5 seconds until the manager is running", () => {
    expect(workspaceAgentsStartupRefetchInterval(undefined, 0)).toBe(WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS);
    expect(workspaceAgentsStartupRefetchInterval([manager("stopped")], 60_000)).toBe(
      WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS,
    );
  });

  it("stops immediately when the manager is running", () => {
    expect(workspaceAgentsStartupRefetchInterval([manager("running")], 1_000)).toBe(false);
  });

  it("stops after the two-minute startup window", () => {
    expect(workspaceAgentsStartupRefetchInterval([manager("stopped")], WORKSPACE_AGENTS_STARTUP_POLL_WINDOW_MS)).toBe(
      false,
    );
  });
});
