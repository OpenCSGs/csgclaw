import { describe, it, expect } from "vitest";
import { applyIMEvent, normalizeRoom, type IMData, type IMServerEvent } from "@/models/conversations";
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
  work: { room_id: "r", user_id: "user-worker", status: { context_usage: usage } },
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
});
