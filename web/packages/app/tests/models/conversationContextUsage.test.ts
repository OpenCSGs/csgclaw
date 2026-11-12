import { describe, it, expect } from "vitest";
import {
  applyIMEvent,
  normalizeRoom,
  upsertConversationInData,
  type IMData,
  type IMServerEvent,
} from "@/models/conversations";
import type { ContextUsage } from "@/models/modelMetadata";
const usage: ContextUsage = {
  session_id: "s",
  model_id: "m",
  used_tokens: 25000,
  context_window: 100000,
  context_source: "runtime",
  auto_compact: true,
  compact_threshold: 75000,
  compacting: false,
  estimated: false,
  updated_at: "2026-11-11T00:00:00Z",
};
const data = { rooms: [{ id: "r", members: ["u-worker"], messages: [] }], users: [] } as IMData;
const event = {
  type: "participant.work.updated",
  work: { reason: "status_updated", room_id: "r", user_id: "user-worker", status: { context_usage: usage } },
} as IMServerEvent;
describe("room context snapshots", () => {
  it("retains a runtime report in conversation data after work ends", () => {
    const updated = applyIMEvent(data, event) as IMData;
    const idle = applyIMEvent(updated, {
      ...event,
      work: { ...event.work!, state: "idle", status: undefined },
    }) as IMData;
    expect(idle.rooms[0].context_usage?.["user-worker"]).toEqual(usage);
  });
  it("rejects thread reports and non-members and normalizes saved snapshots", () => {
    expect(applyIMEvent(data, { ...event, work: { ...event.work!, thread_root_id: "thread" } })).toBe(data);
    expect(
      (applyIMEvent(data, { ...event, work: { ...event.work!, user_id: "outsider" } }) as IMData).rooms[0]
        .context_usage,
    ).toBeUndefined();
    const room = normalizeRoom({
      ...data.rooms[0],
      context_usage: { "user-worker": usage, outsider: usage, invalid: {} as ContextUsage },
    });
    expect(Object.keys(room.context_usage!)).toEqual(["user-worker"]);
  });
  it("does not restore usage from a released lease after the room was cleared", () => {
    const updated = applyIMEvent(data, event) as IMData;
    const cleared = applyIMEvent(updated, {
      type: "room.messages_cleared",
      room: { ...data.rooms[0], context_usage_epoch: 1 },
    } as unknown as IMServerEvent) as IMData;
    const released = applyIMEvent(cleared, {
      ...event,
      work: { ...event.work!, state: "idle", reason: "released" },
    }) as IMData;
    expect(Object.keys(released.rooms[0].context_usage ?? {})).toHaveLength(0);
  });
  it("accepts only the new usage epoch after clearing, including status-only updates", () => {
    const cleared = { ...data, rooms: [{ ...data.rooms[0], context_usage_epoch: 1 }] } as IMData;
    const stale = applyIMEvent(cleared, event) as IMData;
    expect(stale.rooms[0].context_usage).toBeUndefined();
    const fresh = applyIMEvent(cleared, {
      ...event,
      work: { ...event.work!, status: { ...event.work!.status!, context_usage_epoch: 1 } },
    }) as IMData;
    expect(fresh.rooms[0].context_usage?.["user-worker"]).toEqual(usage);
  });
  it("retains a fresh post-clear report when its queue is delivered before the clear event", () => {
    const fresh = { ...event, work: { ...event.work!, status: { ...event.work!.status!, context_usage_epoch: 1 } } };
    const updated = applyIMEvent(data, fresh) as IMData;
    const cleared = applyIMEvent(updated, {
      type: "room.messages_cleared",
      room: { ...data.rooms[0], context_usage_epoch: 1 },
    } as IMServerEvent) as IMData;
    expect(cleared.rooms[0].context_usage?.["user-worker"]).toEqual(usage);
    const stale = applyIMEvent(cleared, event) as IMData;
    expect(stale.rooms[0].context_usage?.["user-worker"]).toEqual(usage);
  });
  it("does not let the clear HTTP response overwrite a fresh event from the same epoch", () => {
    const fresh = applyIMEvent(data, {
      ...event,
      work: { ...event.work!, status: { ...event.work!.status!, context_usage_epoch: 1 } },
    }) as IMData;
    const result = upsertConversationInData(fresh, { ...data.rooms[0], context_usage_epoch: 1 }) as IMData;
    expect(result.rooms[0].context_usage?.["user-worker"]).toEqual(usage);
  });
});
