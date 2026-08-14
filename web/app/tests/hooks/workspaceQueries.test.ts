import {
  WORKSPACE_AGENTS_SETTLE_POLL_INTERVAL_MS,
  WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS,
  WORKSPACE_AGENTS_STARTUP_POLL_WINDOW_MS,
  workspaceAgentsAvailabilityRefetchInterval,
  workspaceAgentsRefetchInterval,
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

  it("keeps polling while the server reports a configured runtime restore", () => {
    expect(
      workspaceAgentsStartupRefetchInterval(
        [manager("running"), { id: "u-alice", runtime: { startup_pending: true } }],
        1_000,
      ),
    ).toBe(WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS);
  });

  it("stops after the two-minute startup window", () => {
    expect(workspaceAgentsStartupRefetchInterval([manager("stopped")], WORKSPACE_AGENTS_STARTUP_POLL_WINDOW_MS)).toBe(
      false,
    );
  });
});

describe("workspaceAgentsAvailabilityRefetchInterval", () => {
  const availabilityExpiresAt = "2026-07-30T04:30:00Z";
  const runningGateway: AgentLike = {
    ...manager("running"),
    runtime: { availability: { state: "degraded", expires_at: availabilityExpiresAt } },
  };

  it("refreshes when the most recent availability observation expires", () => {
    expect(workspaceAgentsAvailabilityRefetchInterval([runningGateway], Date.parse("2026-07-30T04:29:00Z"))).toBe(
      60_000,
    );
    expect(workspaceAgentsAvailabilityRefetchInterval([runningGateway], Date.parse("2026-07-30T04:30:01Z"))).toBe(
      1_000,
    );
  });

  it("continues polling ready and unknown observations, but not unsupported runtimes", () => {
    expect(
      workspaceAgentsAvailabilityRefetchInterval(
        [
          { ...runningGateway, runtime: { availability: { state: "ready", expires_at: availabilityExpiresAt } } },
          { ...runningGateway, runtime: { availability: { state: "unknown", expires_at: availabilityExpiresAt } } },
          {
            ...runningGateway,
            runtime: { availability: { state: "not_applicable", expires_at: availabilityExpiresAt } },
          },
        ],
        Date.parse("2026-07-30T04:29:00Z"),
      ),
    ).toBe(60_000);
    expect(
      workspaceAgentsAvailabilityRefetchInterval(
        [
          {
            ...runningGateway,
            runtime: { availability: { state: "not_applicable", expires_at: availabilityExpiresAt } },
          },
        ],
        Date.parse("2026-07-30T04:29:00Z"),
      ),
    ).toBe(false);
  });

  it("keeps polling a pending unknown observation without an expiry", () => {
    expect(
      workspaceAgentsAvailabilityRefetchInterval([
        { ...runningGateway, runtime: { availability: { state: "unknown" } } },
      ]),
    ).toBe(1_000);
  });

  it("combines availability expiry with startup polling without increasing the poll interval", () => {
    const startingGateway = { ...runningGateway, status: "stopped" };
    expect(workspaceAgentsRefetchInterval([startingGateway], 1_000, Date.parse("2026-07-30T04:29:59Z"))).toBe(1_000);
    expect(workspaceAgentsRefetchInterval([startingGateway], 1_000, Date.parse("2026-07-30T04:29:00Z"))).toBe(
      WORKSPACE_AGENTS_STARTUP_POLL_INTERVAL_MS,
    );
  });
});
