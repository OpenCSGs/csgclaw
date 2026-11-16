import { describe, expect, it } from "vitest";
import { collaborationTurns, collaborationTurnMatches } from "@/models/collaborationTurns";
import { mergeConversationActivityMessages } from "@/components/business/ConversationActivityPanel/conversationActivity";
import type { IMMessage } from "@/models/conversations";
function execution(id: string, actor = "dev", source = "human", thread = "", revision = 1): IMMessage {
  return {
    id: `${actor}-${id}-${thread}`,
    sender_id: actor,
    content: "",
    created_at: "2026-11-12T01:00:00Z",
    relates_to: thread ? { rel_type: "m.thread", event_id: thread } : undefined,
    metadata: {
      codex: { request_id: source },
      csgclaw: { turn_progress: { id, revision, status: "succeeded", started_at: "2026-11-12T01:00:00Z", items: [] } },
    },
  };
}
describe("collaboration turns", () => {
  it("isolates interleaved actors and threads even when their turn IDs and timestamps match", () => {
    const human = {
      id: "human",
      sender_id: "admin",
      content: "Find sources",
      metadata: { diagnostics: { source_id: "human" } },
    };
    const result = collaborationTurns([
      execution("one"),
      execution("one", "qa"),
      human,
      execution("one", "dev", "human", "thread"),
    ]);
    expect(result.turns).toHaveLength(3);
    expect(result.turns.every((turn) => turn.source === human && turn.messages.length === 1)).toBe(true);
    expect(result.unassigned).toEqual([human]);
  });
  it("joins request-only records only when a single exact turn exists", () => {
    const legacy = { id: "legacy", sender_id: "dev", content: "tool", metadata: { codex: { request_id: "human" } } };
    const one = collaborationTurns([legacy, execution("one")]);
    expect(one.turns[0].messages).toHaveLength(2);
    const ambiguous = collaborationTurns([execution("one"), legacy, execution("two")]);
    expect(ambiguous.turns).toHaveLength(2);
    expect(ambiguous.unassigned).toEqual([legacy]);
    expect(collaborationTurns([legacy]).turns[0].status).toBe("unknown");
  });
  it("keeps silent turns and does not regress a live revision during refresh", () => {
    const older = execution("one");
    const newer = execution("one", "dev", "human", "", 9);
    expect(mergeConversationActivityMessages([newer], [older])).toEqual([newer]);
    expect(collaborationTurns([newer, older]).turns[0].progress?.revision).toBe(9);
  });
  it("searches triggers and tool details but excludes hidden reasoning", () => {
    const message = execution("one");
    (message.metadata!.csgclaw as { turn_progress: { items: unknown[] } }).turn_progress.items = [
      { id: "r", kind: "reasoning", text: "private-thought" },
      {
        id: "t",
        kind: "tool",
        tool: { name: "search", status: "completed", actions: [], input: "fact-check", output: "verified-source" },
      },
    ];
    const turn = collaborationTurns([message, { id: "human", content: "News today" }]).turns[0];
    expect(collaborationTurnMatches(turn, "news verified-source")).toBe(true);
    expect(collaborationTurnMatches(turn, "private-thought")).toBe(false);
  });
});
