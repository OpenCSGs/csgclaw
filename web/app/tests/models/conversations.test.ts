import {
  agentMatchesUser,
  applyIMEvent,
  appendMessageToData,
  conversationThreadViews,
  formatConversationPreview,
  formatEventMessage,
  formatMessageTimestampParts,
  formatMessagePreviewText,
  splitMessagePreviewText,
  formatTime,
  feishuHumanParticipant,
  hasConnectedHumanChannel,
  humanConnectedChannels,
  isAgentRosterEvent,
  isToolCallMessage,
  latestAt,
  removeUserFromData,
  sortConversations,
  THREAD_RELATION_TYPE,
  userDisplayName,
} from "@/models/conversations";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import type { IMConversation, IMMessage } from "@/models/conversations";
import { vi } from "vitest";

const t = (key: string) => key;

function message(id: string, createdAt: string, senderID = "u-1"): IMMessage {
  return {
    content: `message ${id}`,
    created_at: createdAt,
    id,
    sender_id: senderID,
  };
}

function room(id: string, createdAt: string, extra: Partial<IMConversation> = {}): IMConversation {
  return {
    description: "",
    id,
    is_direct: false,
    members: ["u-1", "u-2"],
    messages: [message(`${id}-message`, createdAt)],
    title: id,
    ...extra,
  };
}

describe("conversation model helpers", () => {
  it("formats structured event messages in English and Chinese", () => {
    const usersById = new Map([
      ["u-1", { id: "u-1", name: "Alice" }],
      ["u-2", { handle: "bob", id: "u-2" }],
    ]);

    expect(
      formatEventMessage(
        {
          content: "Alice created the room",
          event: { actor_id: "u-1", key: "room_created", title: "Ops" },
          sender_id: "u-1",
        },
        usersById,
        "en",
      ),
    ).toBe("Alice created the room");

    expect(
      formatEventMessage(
        {
          content: "Alice 邀请 @bob 加入了房间",
          event: { actor_id: "u-1", key: "room_members_added", target_ids: ["u-2"] },
          mentions: [],
          sender_id: "u-1",
        },
        usersById,
        "zh",
      ),
    ).toBe("Alice 邀请 @bob 加入了房间");
  });

  it("uses message content or conversation subtitle for previews", () => {
    const usersById = new Map();
    expect(
      formatConversationPreview({ content: 'Hello <at user_id="u-1">Alice</at>' }, null, "u-1", usersById, "en", t),
    ).toBe("Hello @Alice");
    expect(formatConversationPreview(null, room("general", "2026-05-15T00:00:00Z"), "u-1", usersById, "en", t)).toBe(
      "",
    );
  });

  it("strips markdown code fences from compact message previews", () => {
    expect(formatMessagePreviewText("```text\nthread title should be plain\n```")).toBe("thread title should be plain");
    expect(formatMessagePreviewText("```text thread title should be plain ```")).toBe("thread title should be plain");
    expect(formatMessagePreviewText("``` thread title should stay plain ```")).toBe("thread title should stay plain");
    expect(formatMessagePreviewText('Hi <at user_id="u-1">Alice</at>')).toBe("Hi @Alice");
    expect(
      formatMessagePreviewText('<slash-command name="use-skill" arg="skill-creator"></slash-command> create README'),
    ).toBe("/skill-creator create README");
    expect(formatMessagePreviewText('<slash-command name="use-skill" arg="skill-creator" />')).toBe("/skill-creator");
    expect(
      formatMessagePreviewText('<slash-command name="use-skill" arg="skill-creator"><b>bad</b></slash-command>'),
    ).toBe('<slash-command name="use-skill" arg="skill-creator"><b>bad</b></slash-command>');
  });

  it("keeps path-like slash segments in plain text previews", () => {
    expect(splitMessagePreviewText("Open /tmp/build logs")).toEqual([{ text: "Open /tmp/build logs", type: "text" }]);
    expect(splitMessagePreviewText("/skill-creator run tests")).toEqual([
      { text: "/skill-creator", type: "slash" },
      { text: " run tests", type: "text" },
    ]);
  });

  it("formats message timestamps with the browser local timezone", () => {
    const spy = vi.spyOn(Date.prototype, "toLocaleTimeString").mockReturnValue("local time");
    try {
      expect(formatTime("2026-05-26T07:44:00Z", "en")).toBe("local time");
      expect(formatTime("2026-05-26T07:44:00Z", "zh")).toBe("local time");

      expect(spy).toHaveBeenNthCalledWith(1, "en-US", {
        hour: "2-digit",
        minute: "2-digit",
      });
      expect(spy).toHaveBeenNthCalledWith(2, "zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
      });
    } finally {
      spy.mockRestore();
    }
  });

  it("formats layered message timestamps and hover details", () => {
    const enT = (key: string) =>
      key === "timestampToday" ? "Today" : key === "timestampYesterday" ? "Yesterday" : key;
    const zhT = (key: string) => (key === "timestampToday" ? "今天" : key === "timestampYesterday" ? "昨天" : key);
    const now = new Date("2026-06-03T12:00:00+08:00");

    expect(formatMessageTimestampParts("2026-06-03T14:32:45+08:00", "en", enT, now)).toEqual({
      dateTime: "2026-06-03T06:32:45.000Z",
      dividerLabel: "Today",
      label: "14:32",
      shortLabel: "14:32",
      tooltip: "2026-06-03 14:32:45",
    });
    expect(formatMessageTimestampParts("2026-06-02T14:32:45+08:00", "en", enT, now)).toEqual({
      dateTime: "2026-06-02T06:32:45.000Z",
      dividerLabel: "Yesterday",
      label: "Yesterday 14:32",
      shortLabel: "14:32",
      tooltip: "2026-06-02 14:32:45",
    });
    expect(formatMessageTimestampParts("2026-06-01T14:32:45+08:00", "zh", zhT, now)).toEqual({
      dateTime: "2026-06-01T06:32:45.000Z",
      dividerLabel: "周一",
      label: "周一 14:32",
      shortLabel: "14:32",
      tooltip: "2026-06-01 14:32:45",
    });
    expect(formatMessageTimestampParts("2026-05-25T14:32:45+08:00", "zh", zhT, now)).toEqual({
      dateTime: "2026-05-25T06:32:45.000Z",
      dividerLabel: "5月25日",
      label: "5月25日 14:32",
      tooltip: "2026-05-25 14:32:45",
      shortLabel: "14:32",
    });
    expect(formatMessageTimestampParts("2026-05-11T10:25:00+08:00", "en", enT, now)).toEqual({
      dateTime: "2026-05-11T02:25:00.000Z",
      dividerLabel: "05-11",
      label: "05-11 10:25",
      shortLabel: "10:25",
      tooltip: "2026-05-11 10:25:00",
    });
    expect(formatMessageTimestampParts("2025-05-31T13:21:45+08:00", "zh", zhT, now)).toEqual({
      dateTime: "2025-05-31T05:21:45.000Z",
      dividerLabel: "2025年5月31日",
      label: "2025年5月31日 13:21",
      shortLabel: "13:21",
      tooltip: "2025-05-31 13:21:45",
    });
  });

  it("resolves display names and agent/user matches defensively", () => {
    const usersById = new Map([
      ["u-1", { id: "u-1", name: "Alice" }],
      ["u-2", { handle: "worker", id: "u-2" }],
    ]);

    expect(userDisplayName("u-1", usersById)).toBe("Alice");
    expect(userDisplayName("u-2", usersById)).toBe("@worker");
    expect(userDisplayName("missing", usersById)).toBe("missing");
    expect(agentMatchesUser({ handle: "Worker" }, { handle: "worker", id: "u-2" })).toBe(true);
    expect(agentMatchesUser({ name: "Manager" }, { id: "u-3", name: "manager" })).toBe(true);
    expect(agentMatchesUser(null, { id: "u-1" })).toBe(false);
  });

  it("detects Feishu-bound human channels from participants", () => {
    const user = {
      id: "admin",
      name: "Admin",
      participants: [
        {
          channel: "feishu",
          channel_user_kind: "open_id",
          channel_user_ref: "ou_admin",
          id: "admin",
          name: "龙韵",
          type: "human",
        },
        {
          agent_id: "u-dev",
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "dev",
          type: "agent",
        },
      ],
    };

    expect(feishuHumanParticipant(user)?.channel_user_ref).toBe("ou_admin");
    expect(humanConnectedChannels(user)).toEqual([
      {
        id: "feishu",
        name: "Feishu",
        participantID: "admin",
        channelUserName: "龙韵",
        channelUserRef: "ou_admin",
      },
    ]);
    expect(hasConnectedHumanChannel(user, "feishu")).toBe(true);
  });

  it("applies IM events without duplicating messages and keeps rooms sorted by latest activity", () => {
    const current = {
      rooms: [room("old", "2026-05-14T00:00:00Z"), room("new", "2026-05-15T00:00:00Z")],
      users: [],
    };

    const next = applyIMEvent(current, {
      message: message("old-new-message", "2026-05-16T00:00:00Z"),
      room_id: "old",
      type: "message.created",
    });

    expect(next.rooms.map((item) => item.id)).toEqual(["old", "new"]);
    expect(next.rooms[0].messages.map((item) => item.id)).toContain("old-new-message");

    const duplicate = applyIMEvent(next, {
      message: {
        ...message("old-new-message", "2026-05-16T00:00:00Z"),
        content: "updated message content",
      },
      room_id: "old",
      type: "message.created",
    });
    expect(duplicate.rooms[0].messages.filter((item) => item.id === "old-new-message")).toHaveLength(1);
    expect(duplicate.rooms[0].messages.find((item) => item.id === "old-new-message")?.content).toBe(
      "updated message content",
    );
  });

  it("applies user update events to existing users", () => {
    const current = {
      rooms: [],
      users: [{ avatar: "avatar/3D-1.png", id: "u-alice", name: "alice" }],
    };

    const next = applyIMEvent(current, {
      type: "user.updated",
      user: { avatar: "avatar/cartoon-4.png", id: "u-alice", name: "alice" },
    });

    expect(next.users).toEqual([{ avatar: "avatar/cartoon-4.png", id: "u-alice", name: "alice" }]);
  });

  it("keeps thread replies out of the main timeline", () => {
    const current = {
      rooms: [room("general", "2026-05-15T00:00:00Z")],
      users: [],
    };

    const next = appendMessageToData(current, "general", {
      ...message("reply-1", "2026-05-15T00:01:00Z"),
      relates_to: {
        rel_type: THREAD_RELATION_TYPE,
        event_id: "general-message",
      },
    });

    expect(next.rooms[0].messages.map((item) => item.id)).toEqual(["general-message"]);
  });

  it("applies room messages cleared events as authoritative room updates", () => {
    const current = {
      rooms: [
        room("general", "2026-05-15T00:00:00Z", {
          messages: [message("old-root", "2026-05-15T00:00:00Z")],
          threads: [{ root_message_id: "old-root" }],
        }),
        room("other", "2026-05-15T00:02:00Z", {
          messages: [message("other-message", "2026-05-15T00:02:00Z")],
        }),
      ],
      users: [],
    };

    const next = applyIMEvent(current, {
      room: { ...current.rooms[0], messages: [], threads: [] },
      room_id: "general",
      type: "room.messages_cleared",
    });

    expect(next.rooms.find((item) => item.id === "general")?.messages).toEqual([]);
    expect(next.rooms.find((item) => item.id === "general")?.threads).toEqual([]);
    expect(next.rooms.find((item) => item.id === "other")?.messages.map((item) => item.id)).toEqual(["other-message"]);
  });

  it("applies thread event summaries to root messages and exposes thread views", () => {
    const root = message("root-1", "2026-05-15T00:00:00Z");
    const current = {
      rooms: [
        room("general", "2026-05-15T00:00:00Z", {
          messages: [root],
        }),
      ],
      users: [],
    };

    const next = applyIMEvent(current, {
      room_id: "general",
      thread: {
        room_id: "general",
        root,
        replies: [message("reply-1", "2026-05-15T00:01:00Z")],
        summary: {
          context_summary: { root_excerpt: "Root excerpt", message_count: 1 },
          reply_count: 1,
          root_id: "root-1",
        },
      },
      type: "thread.updated",
    });

    expect(next.rooms[0].messages[0].thread?.reply_count).toBe(1);
    expect(next.rooms[0].threads?.[0].root_message_id).toBe("root-1");
    const views = conversationThreadViews(next.rooms[0]);
    expect(views).toHaveLength(1);
    expect(views[0].summary?.context_summary?.root_excerpt).toBe("Root excerpt");
  });

  it("removes deleted users from users, room members, and their messages", () => {
    const current = {
      rooms: [
        room("kept", "2026-05-15T00:00:00Z", {
          members: ["u-1", "u-2", "u-3"],
          messages: [
            message("from-1", "2026-05-15T00:00:00Z", "u-1"),
            message("from-2", "2026-05-15T00:01:00Z", "u-2"),
          ],
        }),
        room("removed", "2026-05-15T00:00:00Z", {
          members: ["u-1", "u-2"],
          messages: [message("from-2-only", "2026-05-15T00:00:00Z", "u-2")],
        }),
      ],
      users: [
        { id: "u-1", name: "Alice" },
        { id: "u-2", name: "Bob" },
      ],
    };

    const next = removeUserFromData(current, "u-2");

    expect(next.users).toEqual([{ id: "u-1", name: "Alice" }]);
    expect(next.rooms.map((item) => item.id)).toEqual(["kept"]);
    expect(next.rooms[0].members).toEqual(["u-1", "u-3"]);
    expect(next.rooms[0].messages.map((item) => item.id)).toEqual(["from-1"]);
  });

  it("classifies agent roster events and latest timestamps", () => {
    expect(isAgentRosterEvent({ type: "user.created" })).toBe(true);
    expect(isAgentRosterEvent({ type: "user.updated" })).toBe(true);
    expect(isAgentRosterEvent({ room: { is_direct: true }, type: "room.created" })).toBe(true);
    expect(isAgentRosterEvent({ room: { is_direct: false }, type: "room.created" })).toBe(false);
    expect(latestAt({ messages: [] })).toBe(0);
    expect(
      sortConversations([room("old", "2026-05-14T00:00:00Z"), room("new", "2026-05-15T00:00:00Z")]).map(
        (item) => item.id,
      ),
    ).toEqual(["new", "old"]);
  });

  it("classifies legacy and structured tool-call messages", () => {
    expect(isToolCallMessage("🔧 Running tool")).toBe(true);
    expect(
      isToolCallMessage({
        content: JSON.stringify({
          type: CSGCLAW_AGENT_ACTIVITY_TYPE,
          content: {
            msgtype: AgentActivityMsgTypes.tool,
            body: "Running tool",
            tool: { id: "tool-1", status: "running", title: "Run shell command" },
          },
        }),
      }),
    ).toBe(true);
    expect(
      isToolCallMessage({
        content: JSON.stringify({
          type: CSGCLAW_AGENT_ACTIVITY_TYPE,
          content: {
            msgtype: AgentActivityMsgTypes.action,
            body: "Codex wants permission",
            action: { id: "perm-1", status: "pending", title: "Run shell command" },
          },
        }),
      }),
    ).toBe(false);
  });
});
