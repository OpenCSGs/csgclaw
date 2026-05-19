import {
  agentMatchesUser,
  applyIMEvent,
  formatConversationPreview,
  formatEventMessage,
  isAgentRosterEvent,
  latestAt,
  removeUserFromData,
  sortConversations,
  userDisplayName,
} from "@/models/conversations";

const t = (key: string) => key;

function message(id: string, createdAt: string, senderID = "u-1") {
  return {
    content: `message ${id}`,
    created_at: createdAt,
    id,
    sender_id: senderID,
  };
}

function room(id: string, createdAt: string, extra: Record<string, unknown> = {}) {
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
          content: "",
          event: { actor_id: "u-1", key: "room_created", title: "Ops" },
          sender_id: "u-1",
        },
        usersById,
        "en",
      ),
    ).toBe('Alice created the room "Ops"');

    expect(
      formatEventMessage(
        {
          content: "",
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
      message: message("old-new-message", "2026-05-16T00:00:00Z"),
      room_id: "old",
      type: "message.created",
    });
    expect(duplicate.rooms[0].messages.filter((item) => item.id === "old-new-message")).toHaveLength(1);
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
    expect(isAgentRosterEvent({ room: { is_direct: true }, type: "room.created" })).toBe(true);
    expect(isAgentRosterEvent({ room: { is_direct: false }, type: "room.created" })).toBe(false);
    expect(latestAt({ messages: [] })).toBe(0);
    expect(
      sortConversations([room("old", "2026-05-14T00:00:00Z"), room("new", "2026-05-15T00:00:00Z")]).map(
        (item) => item.id,
      ),
    ).toEqual(["new", "old"]);
  });
});
