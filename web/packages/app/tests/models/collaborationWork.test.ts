import { describe, expect, it } from "vitest";
import { collaborationWork, collaborationWorkSummary } from "@/models/collaborationWork";
import type { IMMessage } from "@/models/conversations";
import type { TurnProgress } from "@/models/turnProgress";

const lease = { id: "dev", name: "Developer", roomID: "room", requestID: "request", leaseID: "lease", canStop: true };
function message(status = "running", items: TurnProgress["items"] = []): IMMessage {
  return {
    id: "message",
    sender_id: "dev",
    content: "",
    metadata: {
      csgclaw: {
        request_id: "request",
        turn_progress: { id: "turn", revision: 1, status, started_at: "2026-11-11T12:00:00Z", items },
      },
    },
  };
}

describe("collaboration work status", () => {
  it("replaces a pending lease with progress and releases it on completion", () => {
    expect(collaborationWork("room", [], [lease])).toHaveLength(1);
    const running = collaborationWork("room", [message()], [lease]);
    expect(running).toHaveLength(1);
    expect(running[0].participant).toBe(lease);
    expect(collaborationWork("room", [message("succeeded")], [lease])).toEqual([]);
  });
  it("never lends a later request's stop control to an earlier run", () => {
    const nextLease = { ...lease, requestID: "next" };
    const work = collaborationWork("room", [message()], [nextLease]);
    expect(work).toHaveLength(2);
    expect(work[0].participant).toBeUndefined();
    expect(work[1].participant).toBe(nextLease);
  });
  it("reports an unfinished tool, then result processing; never exposes reasoning or shell input", () => {
    const tool = {
      id: "tool",
      kind: "tool" as const,
      tool: { name: "Search", actions: ["search"], status: "running", input: '{"query":"AI news"}' },
    };
    const running = collaborationWork("room", [message("running", [tool])], [lease])[0];
    expect(collaborationWorkSummary(running)).toEqual({ label: "conversationWorkingSearching", detail: "AI news" });
    const finished = { ...tool, tool: { ...tool.tool, status: "completed" } };
    const processing = collaborationWork("room", [message("running", [finished])], [lease])[0];
    expect(collaborationWorkSummary(processing).label).toBe("collaborationWorkReviewing");
    const privateWork = collaborationWork(
      "room",
      [
        message("running", [
          { id: "reason", kind: "reasoning", text: "Private reasoning" },
          {
            ...tool,
            tool: {
              ...tool.tool,
              actions: ["execute"],
              name: "Run shell command",
              input: '{"command":"long private script"}',
            },
          },
        ]),
      ],
      [lease],
    )[0];
    expect(collaborationWorkSummary(privateWork)).toEqual({
      label: "conversationWorkingRunning",
      detail: "Run shell command",
    });
  });
  it("uses current commentary or public updates for context, never a previous round's message", () => {
    const msgs = [
      message(),
      { id: "update", sender_id: "dev", content: "Checking official sources", created_at: "2026-11-11T12:01:00Z" },
    ];
    expect(collaborationWorkSummary(collaborationWork("room", msgs, [lease])[0]).detail).toBe(
      "Checking official sources",
    );
    msgs[1].created_at = "2026-11-11T11:00:00Z";
    expect(collaborationWorkSummary(collaborationWork("room", msgs, [lease])[0]).detail).toBe("");
  });
  it("keeps waiting and errors visible, and retires an old failure after a newer run", () => {
    const failure = message("failed");
    expect(collaborationWorkSummary(collaborationWork("room", [failure], [])[0]).label).toBe("progressStatus_failed");
    const next = message("succeeded");
    (next.metadata!.csgclaw as { turn_progress: TurnProgress }).turn_progress.started_at = "2026-11-11T12:02:00Z";
    expect(collaborationWork("room", [failure, next], [])).toEqual([]);
    expect(collaborationWorkSummary(collaborationWork("room", [message("waiting")], [lease])[0]).label).toBe(
      "progressStatus_waiting",
    );
  });
});

it("distinguishes checking new messages from claiming work", () => {
  const notification = message();
  (notification.metadata!.csgclaw as { turn_progress: TurnProgress }).turn_progress.private_output = true;
  expect(collaborationWorkSummary(collaborationWork("room", [notification], [lease])[0]).label).toBe(
    "collaborationWorkCheckingMessages",
  );
  const reading = message("running", [
    { id: "read", kind: "tool", tool: { name: "mcp__csgclaw__messages_read", actions: ["tool"], status: "running" } },
  ]);
  expect(collaborationWorkSummary(collaborationWork("room", [reading], [lease])[0]).label).toBe(
    "collaborationWorkCheckingMessages",
  );
});
