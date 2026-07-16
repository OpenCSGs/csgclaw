import { describe, expect, it } from "vitest";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMMessage } from "@/models/conversations";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import {
  conversationActivityAgents,
  conversationActivityDensitySegments,
  conversationActivityEntries,
  conversationActivityEntryDetails,
  conversationActivityEntrySummary,
  mergeConversationActivityMessages,
} from "./conversationActivity";

const agents: AgentLike[] = [
  { id: "dev", name: "dev", participants: [{ id: "pt-dev", channel: "csgclaw", channel_user_ref: "u-dev" }] },
  { id: "qa", name: "qa", participants: [{ id: "pt-qa", channel: "csgclaw", channel_user_ref: "u-qa" }] },
  { id: "outside", name: "outside" },
];

const room: IMConversation = {
  id: "room-release",
  is_direct: false,
  members: ["u-admin", "u-dev", "u-qa"],
  messages: [],
  title: "release-war-room",
};

function toolMessage(id: string, createdAt: string, tool: Record<string, unknown>): IMMessage {
  return {
    id,
    created_at: createdAt,
    sender_id: "pt-qa",
    metadata: { codex: { request_id: "turn-7" } },
    content: JSON.stringify({
      type: CSGCLAW_AGENT_ACTIVITY_TYPE,
      content: {
        msgtype: AgentActivityMsgTypes.tool,
        body: "Run tests",
        tool,
      },
    }),
  };
}

describe("conversation activity model", () => {
  it("scopes the timeline to room agents and merges tool lifecycle events", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [
        { id: "human", created_at: "2026-07-16T10:00:00Z", sender_id: "u-admin", content: "ship it" },
        { id: "dev-reply", created_at: "2026-07-16T10:00:01Z", sender_id: "u-dev", content: "Starting e2e" },
        toolMessage("tool-start", "2026-07-16T10:00:02Z", {
          item_id: "command:item-1",
          input: { command: "npm run test:e2e" },
          kind: "exec_command",
          status: "running",
          title: "Run shell command",
          tool_call_id: "call-7",
        }),
        toolMessage("tool-end", "2026-07-16T10:00:05Z", {
          kind: "exec_command",
          output: "2 failed",
          status: "completed",
          title: "Run shell command",
          tool_call_id: "call-7",
        }),
        { id: "outside", created_at: "2026-07-16T10:00:06Z", sender_id: "outside", content: "ignore me" },
      ],
      roomAgents,
      room.members,
    );

    expect(roomAgents.map((agent) => agent.id)).toEqual(["dev", "qa"]);
    expect(entries).toHaveLength(3);
    expect(entries.map((entry) => entry.source)).toEqual(["user", "agent", "agent"]);
    expect(entries.map((entry) => entry.eventType)).toEqual(["message", "message", "exec_command"]);
    expect(conversationActivityEntrySummary(entries[0]!)).toBe("ship it");
    expect(entries[2]).toMatchObject({ eventType: "exec_command", index: 3, updatedAt: "2026-07-16T10:00:05Z" });
    expect(conversationActivityEntrySummary(entries[2]!)).toBe("npm run test:e2e");
    expect(conversationActivityEntryDetails(entries[2]!)).toEqual([
      { kind: "command", value: "npm run test:e2e" },
      { kind: "result", value: "2 failed" },
    ]);
  });

  it("upserts SSE messages by message id without losing chronological order", () => {
    const initial: IMMessage = {
      id: "message-1",
      created_at: "2026-07-16T10:00:02Z",
      sender_id: "u-dev",
      content: "old",
    };
    const updated = { ...initial, content: "updated" };
    const earlier: IMMessage = {
      id: "message-0",
      created_at: "2026-07-16T10:00:01Z",
      sender_id: "u-dev",
      content: "earlier",
    };

    expect(mergeConversationActivityMessages([initial], [updated, earlier])).toEqual([earlier, updated]);
  });

  it("compresses idle gaps while sizing density segments by active duration", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [
        toolMessage("tool-start", "2026-07-16T10:00:00Z", {
          kind: "exec_command",
          status: "running",
          title: "Run shell command",
          tool_call_id: "call-density",
        }),
        toolMessage("tool-end", "2026-07-16T10:00:30Z", {
          kind: "exec_command",
          status: "completed",
          title: "Run shell command",
          tool_call_id: "call-density",
        }),
        { id: "later", created_at: "2026-07-16T10:01:40Z", sender_id: "u-dev", content: "done" },
      ],
      roomAgents,
      room.members,
    );

    const segments = conversationActivityDensitySegments(entries);
    expect(segments).toHaveLength(2);
    expect(segments[0]?.startPercent).toBe(0);
    expect(segments[0]?.durationPercent).toBeCloseTo((30 / 31) * 100);
    expect(segments[1]?.startPercent).toBeCloseTo((30 / 31) * 100);
    expect(segments[1]?.durationPercent).toBeCloseTo((1 / 31) * 100);
    expect((segments[0]?.durationPercent || 0) + (segments[1]?.durationPercent || 0)).toBeCloseTo(100);
  });

  it("fills the density track evenly when every event is instantaneous", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [
        { id: "first", created_at: "2026-07-16T10:00:00Z", sender_id: "u-dev", content: "first" },
        { id: "near-start", created_at: "2026-07-16T10:00:10Z", sender_id: "u-qa", content: "second" },
        { id: "last", created_at: "2026-07-16T10:01:40Z", sender_id: "u-dev", content: "last" },
      ],
      roomAgents,
      room.members,
    );

    const segments = conversationActivityDensitySegments(entries);
    expect(segments).toHaveLength(3);
    segments.forEach((segment) => expect(segment.durationPercent).toBeCloseTo(100 / 3));
    expect(segments[0]?.startPercent).toBe(0);
    expect(segments[1]?.startPercent).toBeCloseTo(100 / 3);
    expect(segments[2]?.startPercent).toBeCloseTo(200 / 3);
  });
});
