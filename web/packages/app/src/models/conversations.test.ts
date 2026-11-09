import { describe, expect, it } from "vitest";
import { groupVideoGenerationMessages, type IMMessage } from "./conversations";

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
