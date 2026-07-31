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
  conversationWorkingParticipantsWithActivity,
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

function toolMessage(id: string, createdAt: string, tool: Record<string, unknown>, senderID = "pt-qa"): IMMessage {
  return {
    id,
    created_at: createdAt,
    sender_id: senderID,
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

  it("shows each working agent's latest activity and keeps the newest agent at the bottom", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [
        { id: "dev-reply", created_at: "2026-07-16T10:00:01Z", sender_id: "u-dev", content: "Starting e2e" },
        toolMessage(
          "qa-edit",
          "2026-07-16T10:00:02Z",
          {
            input: { path: "playwright.config.ts" },
            kind: "patch_apply",
            status: "running",
            title: "Apply patch",
            tool_call_id: "call-edit",
          },
          "pt-qa",
        ),
        toolMessage(
          "dev-search",
          "2026-07-16T10:00:03Z",
          {
            input: { query: "release checklist" },
            kind: "web_search",
            status: "running",
            title: "Search the web",
            tool_call_id: "call-search",
          },
          "u-dev",
        ),
      ],
      roomAgents,
      room.members,
    );

    const working = conversationWorkingParticipantsWithActivity(
      [
        { activityAfter: "2026-07-16T10:00:00Z", id: "u-dev", name: "dev" },
        { activityAfter: "2026-07-16T10:00:00Z", id: "u-qa", name: "qa" },
      ],
      roomAgents,
      entries,
    );

    expect(working.map((participant) => participant.name)).toEqual(["qa", "dev"]);
    expect(working[0]?.activity).toMatchObject({
      action: "editing",
      summary: "playwright.config.ts",
    });
    expect(working[1]?.activity).toMatchObject({
      action: "searching",
      summary: "release checklist",
    });
  });

  it("does not reuse an older turn's content before the current request emits activity", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [{ id: "old-reply", created_at: "2026-07-16T09:00:00Z", sender_id: "u-dev", content: "Previous answer" }],
      roomAgents,
      room.members,
    );

    const [working] = conversationWorkingParticipantsWithActivity(
      [{ id: "u-dev", name: "dev", requestID: "new-turn" }],
      roomAgents,
      entries,
    );

    expect(working?.activity).toEqual({ action: "preparing_reply" });
  });

  it("keeps the latest tool command visible while the runtime processes its result", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [
        toolMessage(
          "tool-start",
          "2026-07-16T10:00:00Z",
          {
            input: { command: "csgclaw-cli participant list --channel csgclaw" },
            kind: "exec_command",
            status: "running",
            title: "Run shell command",
            tool_call_id: "call-list",
          },
          "u-dev",
        ),
        toolMessage(
          "tool-end",
          "2026-07-16T10:00:01Z",
          {
            kind: "exec_command",
            output: "3 participants",
            status: "completed",
            title: "Run shell command",
            tool_call_id: "call-list",
          },
          "u-dev",
        ),
      ],
      roomAgents,
      room.members,
    );

    const [working] = conversationWorkingParticipantsWithActivity(
      [{ id: "u-dev", name: "dev", requestID: "turn-7" }],
      roomAgents,
      entries,
    );

    expect(working?.activity).toMatchObject({
      action: "running",
      summary: "csgclaw-cli participant list --channel csgclaw",
      toolName: "exec_command",
    });
  });

  it("uses explicit work stages for model waits and final generation", () => {
    const working = conversationWorkingParticipantsWithActivity(
      [
        { id: "u-dev", name: "prepare", workStage: "preparing_reply" },
        { id: "u-dev", name: "reason", thinkingText: "checking", workStage: "thinking" },
        { id: "u-dev", name: "empty-reason", thinkingText: "", workStage: "thinking" },
        { id: "u-dev", name: "final", workStage: "generating_reply" },
        { id: "u-dev", name: "tool", workStage: "running_tool" },
      ],
      [],
      [],
    );

    expect(working.map((participant) => participant.activity?.action)).toEqual([
      "preparing_reply",
      "thinking",
      "preparing_reply",
      "generating_reply",
      "using_tool",
    ]);
  });

  it("ignores previous replies for legacy work inferred from a newer prompt", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const entries = conversationActivityEntries(
      [
        { id: "old-reply", created_at: "2026-07-16T09:00:00Z", sender_id: "u-dev", content: "Previous answer" },
        { id: "new-reply", created_at: "2026-07-16T10:00:01Z", sender_id: "u-dev", content: "Current answer" },
      ],
      roomAgents,
      room.members,
    );

    const [beforeReply] = conversationWorkingParticipantsWithActivity(
      [{ activityAfter: "2026-07-16T10:00:00Z", id: "u-dev", name: "dev" }],
      roomAgents,
      entries.slice(0, 1),
    );
    const [afterReply] = conversationWorkingParticipantsWithActivity(
      [{ activityAfter: "2026-07-16T10:00:00Z", id: "u-dev", name: "dev" }],
      roomAgents,
      entries,
    );

    expect(beforeReply?.activity).toEqual({ action: "preparing_reply" });
    expect(afterReply?.activity).toMatchObject({ action: "replying", summary: "Current answer" });
  });

  it("unwraps shell launchers and truncates long working summaries", () => {
    const roomAgents = conversationActivityAgents(room, agents);
    const command =
      "printf 'a deliberately long command that should be shortened before it reaches the compact working status row'";
    const entries = conversationActivityEntries(
      [
        toolMessage(
          "dev-command",
          "2026-07-16T10:00:00Z",
          {
            input: { command: `/bin/zsh -lc "${command}"` },
            kind: "exec_command",
            status: "running",
            title: "Run shell command",
            tool_call_id: "call-long-command",
          },
          "u-dev",
        ),
      ],
      roomAgents,
      room.members,
    );

    const [working] = conversationWorkingParticipantsWithActivity(
      [{ activityAfter: "2026-07-16T09:59:59Z", id: "u-dev", name: "dev" }],
      roomAgents,
      entries,
    );

    expect(working?.activity?.summary).not.toContain("/bin/zsh -lc");
    expect(working?.activity?.summary?.length).toBeLessThanOrEqual(72);
    expect(working?.activity?.summary).toMatch(/…$/);
  });
});
