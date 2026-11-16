import { describe, expect, it } from "vitest";
import { applyIMEvent, type IMData, type IMMessage } from "./conversations";
import { latestContextUsage, type ContextUsage } from "./modelMetadata";
const old: ContextUsage = {
  session_id: "s",
  model_id: "m",
  context_window: 100000,
  context_source: "user",
  used_tokens: 38000,
  estimated: false,
  compacting: false,
  auto_compact: true,
  compact_threshold: 75000,
  updated_at: "2026-11-16T01:00:00Z",
};
const compacted: ContextUsage = { ...old, used_tokens: 3000, estimated: true, updated_at: "2026-11-16T01:01:00Z" };
const message: IMMessage = { id: "keep", sender_id: "user-admin", content: "Existing transcript" };
const data: IMData = {
  users: [],
  rooms: [
    { id: "r", members: ["user-admin", "user-agent"], messages: [message], context_usage: { "user-agent": old } },
  ],
};
describe("room context usage events", () => {
  it("applies a room update immediately while preserving the live transcript", () => {
    const result = applyIMEvent(data, {
      type: "room.updated",
      room: { ...data.rooms[0], messages: [], context_usage: { "user-agent": compacted } },
    }) as IMData;
    expect(result.rooms[0].context_usage?.["user-agent"]).toEqual(compacted);
    expect(result.rooms[0].messages).toEqual([message]);
    expect(latestContextUsage(old, result.rooms[0].context_usage?.["user-agent"])).toEqual(compacted);
  });
  it("rejects stale values and accepts a cleared room's newer epoch", () => {
    const current: IMData = {
      ...data,
      rooms: [{ ...data.rooms[0], context_usage_epoch: 1, context_usage: { "user-agent": compacted } }],
    };
    const stale = applyIMEvent(current, {
      type: "room.updated",
      room: { ...data.rooms[0], context_usage_epoch: 0 },
    }) as IMData;
    expect(stale.rooms[0].context_usage?.["user-agent"]).toEqual(compacted);
    const cleared = applyIMEvent(current, {
      type: "room.updated",
      room: { ...data.rooms[0], context_usage_epoch: 2, context_usage: {} },
    }) as IMData;
    expect(cleared.rooms[0].context_usage).toEqual({});
  });
});
