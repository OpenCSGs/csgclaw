import { describe, expect, it } from "vitest";
import {
  groupVideoGenerationMessages,
  mergeRoomMetadataUpdateInData,
  roomType,
  type IMMessage,
  type IMConversation,
} from "./conversations";

describe("groupVideoGenerationMessages", () => {
  it("projects a persisted video task under its final manager message", () => {
    const messages: IMMessage[] = [
      {
        id: "video-1",
        content: "prompt",
        metadata: { video_generation: { state: "generating" }, codex: { turn_message_id: "turn-1" } },
      },
      { id: "turn-1", content: "视频正在生成", metadata: { codex: { delivery_kind: "final" } } },
    ];
    const grouped = groupVideoGenerationMessages(messages);
    expect(grouped.childMessageIDs.has("video-1")).toBe(true);
    expect(grouped.childrenByParentID.get("turn-1")).toEqual([messages[0]]);
  });

  it("keeps a video task standalone when its parent is not loaded", () => {
    const message: IMMessage = {
      id: "video-1",
      content: "prompt",
      metadata: { video_generation: { state: "generating" }, codex: { turn_message_id: "missing" } },
    };
    const grouped = groupVideoGenerationMessages([message]);
    expect(grouped.childMessageIDs.size).toBe(0);
    expect(grouped.childrenByParentID.size).toBe(0);
  });
});

describe("autonomous room metadata", () => {
  it("preserves the mode and updates the round status without replacing messages", () => {
    const messages = [{ id: "m", content: "Keep history" }];
    const room: IMConversation = { id: "r", type: "autonomous", members: [], messages };
    const state = { source_id: "m", receipts: ["m:dev"], paused: true };
    const next = mergeRoomMetadataUpdateInData(
      { rooms: [room], users: [] },
      { ...room, messages: [], autonomous_state: state },
    );
    expect(roomType(next.rooms[0])).toBe("autonomous");
    expect(next.rooms[0].messages).toBe(messages);
    expect(next.rooms[0].autonomous_state).toEqual(state);
    expect(roomType({})).toBe("free");
  });
});
