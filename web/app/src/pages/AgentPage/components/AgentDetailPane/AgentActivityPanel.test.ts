import { describe, expect, it } from "vitest";
import type { IMConversation, IMMessage } from "@/models/conversations";
import { CSGCLAW_AGENT_ACTIVITY_TYPE, AgentActivityMsgTypes } from "@/shared/constants/messages";
import { activityEntriesFromRooms } from "./AgentActivityPanel";

const room: IMConversation = {
  id: "room-1",
  members: ["agent-1"],
  messages: [],
  title: "Agent room",
};

function activityMessage(id: string, createdAt: string, tool: Record<string, unknown>, body = "Command"): IMMessage {
  return {
    id,
    content: JSON.stringify({
      channel: "openclaw",
      content: {
        body,
        msgtype: AgentActivityMsgTypes.tool,
        tool,
      },
      event_id: id,
      origin_server_ts: Date.parse(createdAt),
      room_id: room.id,
      sender: "openclaw",
      type: CSGCLAW_AGENT_ACTIVITY_TYPE,
      version: 1,
    }),
    created_at: createdAt,
    metadata: {
      openclaw: {
        request_id: "msg-user-1",
        source_message_id: "msg-user-1",
      },
    },
    sender_id: "agent-1",
  };
}

function openClawToolMessage(
  id: string,
  createdAt: string,
  toolCallID: string,
  content: string,
  requestID = "msg-user-1",
): IMMessage {
  return {
    id,
    content,
    created_at: createdAt,
    metadata: {
      openclaw: {
        delivery_info: { kind: "tool", toolCallId: toolCallID },
        delivery_kind: "tool",
        request_id: requestID,
        tool_call_id: toolCallID,
      },
    },
    sender_id: "agent-1",
  };
}

function entries(messages: IMMessage[]) {
  return activityEntriesFromRooms([{ room, messages }], ["agent-1"]);
}

describe("activityEntriesFromRooms", () => {
  it("merges command lifecycle and output by canonical item id", () => {
    const result = entries([
      activityMessage("activity-start", "2026-07-10T10:00:00.000Z", {
        id: "command:call-7",
        input_summary: JSON.stringify({ command: "csgclaw-cli --version" }),
        item_id: "command:call-7",
        kind: "command",
        phase: "start",
        status: "running",
        title: "Command",
      }),
      activityMessage("activity-end", "2026-07-10T10:00:01.000Z", {
        command: "csgclaw-cli --version",
        id: "call-7",
        item_id: "command:call-7",
        kind: "exec",
        output: "csgclaw-cli v0.3.16",
        output_summary: "csgclaw-cli v0.3.16",
        phase: "end",
        status: "completed",
        title: "csgclaw-cli --version",
        tool_call_id: "call-7",
      }),
    ]);

    expect(result).toHaveLength(1);
    expect(result[0]?.activity?.content.tool).toMatchObject({
      command: "csgclaw-cli --version",
      output: "csgclaw-cli v0.3.16",
      phase: "end",
      status: "completed",
    });
  });

  it("coalesces previously separate item and tool-call rows when a bridge event contains both ids", () => {
    const result = entries([
      activityMessage("item-start", "2026-07-10T10:00:00.000Z", {
        id: "item-7",
        input_summary: JSON.stringify({ command: "csgclaw-cli --help" }),
        item_id: "item-7",
        kind: "command",
        phase: "start",
        status: "running",
        title: "Command",
      }),
      openClawToolMessage(
        "plain-output",
        "2026-07-10T10:00:00.500Z",
        "call-7",
        "csgclaw-cli --help\nusage: csgclaw-cli <command>",
      ),
      activityMessage("command-output", "2026-07-10T10:00:01.000Z", {
        command: "csgclaw-cli --help",
        id: "item-7",
        item_id: "item-7",
        kind: "exec",
        output: "usage: csgclaw-cli <command>",
        output_summary: "usage: csgclaw-cli <command>",
        phase: "end",
        status: "completed",
        title: "csgclaw-cli --help",
        tool_call_id: "call-7",
      }),
    ]);

    expect(result).toHaveLength(1);
    expect(result[0]?.activity?.content.tool).toMatchObject({
      command: "csgclaw-cli --help",
      item_id: "item-7",
      output: "usage: csgclaw-cli <command>",
      tool_call_id: "call-7",
    });
  });

  it("keeps concrete command output when a later lifecycle event only reports status", () => {
    const result = entries([
      activityMessage("command-output", "2026-07-10T10:00:00.000Z", {
        command: "csgclaw-cli --version",
        id: "item-7",
        item_id: "item-7",
        kind: "exec",
        output: "csgclaw-cli v0.3.16",
        output_summary: "csgclaw-cli v0.3.16",
        phase: "end",
        status: "completed",
        title: "csgclaw-cli --version",
        tool_call_id: "call-7",
      }),
      activityMessage("item-end", "2026-07-10T10:00:01.000Z", {
        id: "item-7",
        item_id: "item-7",
        kind: "command",
        output_summary: "Command completed",
        phase: "end",
        status: "completed",
        title: "Command",
      }),
    ]);

    expect(result).toHaveLength(1);
    expect(result[0]?.activity?.content.tool).toMatchObject({
      output: "csgclaw-cli v0.3.16",
      output_summary: "csgclaw-cli v0.3.16",
    });
  });

  it("keeps repeated commands separate when OpenClaw provides different tool-call ids", () => {
    const result = entries([
      openClawToolMessage("call-one", "2026-07-10T10:00:00.000Z", "call-1", "csgclaw-cli --version"),
      openClawToolMessage("call-two", "2026-07-10T10:00:01.000Z", "call-2", "csgclaw-cli --version"),
    ]);

    expect(result).toHaveLength(2);
    expect(result.map((entry) => entry.command?.command)).toEqual(["csgclaw-cli --version", "csgclaw-cli --version"]);
  });

  it("scopes reused tool-call ids to their source request", () => {
    const result = entries([
      openClawToolMessage("first-request", "2026-07-10T10:00:00.000Z", "call-1", "csgclaw-cli --version", "msg-user-1"),
      openClawToolMessage(
        "second-request",
        "2026-07-10T10:01:00.000Z",
        "call-1",
        "csgclaw-cli --version",
        "msg-user-2",
      ),
    ]);

    expect(result).toHaveLength(2);
  });
});
