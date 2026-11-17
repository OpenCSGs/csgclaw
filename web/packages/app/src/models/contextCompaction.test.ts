import { describe, it, expect } from "vitest";
import { applyIMEvent, mergeConversationMessageSnapshot, type IMData, type IMMessage } from "./conversations";
import { conversationCompacting, type CompactionOperation } from "./contextCompaction";
const operation: CompactionOperation = {
  id: "compact",
  epoch: 0,
  running: true,
  started_at: "2026-11-16T00:00:00Z",
  targets: [{ agent_id: "a", user_id: "user-a", name: "Agent", status: "pending" }],
};
const card = (state: CompactionOperation): IMMessage => ({
  id: "card",
  sender_id: "user-admin",
  kind: "event",
  content: "Context compaction",
  metadata: { context_compaction: state },
});
const data = (message: IMMessage): IMData => ({
  users: [],
  rooms: [{ id: "room", members: ["user-admin", "user-a"], messages: [message] }],
});
describe("compaction snapshots", () => {
  it("keeps SSE completion when an earlier poll returns running", () => {
    const pending = card(operation);
    const finished = card({
      ...operation,
      running: false,
      finished_at: "2026-11-16T00:00:10Z",
      targets: [{ ...operation.targets[0], status: "completed" }],
    });
    const live = applyIMEvent(data(pending), { type: "message.created", room_id: "room", message: finished }) as IMData;
    const merged = mergeConversationMessageSnapshot(live, "room", [pending], [pending])!;
    expect(merged.rooms[0].messages[0]).toEqual(finished);
    expect(conversationCompacting(merged.rooms[0])).toBe(false);
  });
  it("does not regress partial progress or late usage measurements", () => {
    const latest = card({ ...operation, revision: 4, targets: [{ ...operation.targets[0], status: "running" }] });
    const stale = card({ ...operation, revision: 2 });
    expect(mergeConversationMessageSnapshot(data(latest), "room", [stale])?.rooms[0].messages[0]).toEqual(latest);
  });
});

it("retains a later usage measurement on a completed card", () => {
  const after = {
    session_id: "s",
    model_id: "m",
    used_tokens: 2000,
    context_window: 100000,
    context_source: "user",
    auto_compact: true,
    compact_threshold: 75000,
    compacting: false,
    estimated: true,
    updated_at: "2026-11-16T00:00:20Z",
  };
  const latest = card({
    ...operation,
    revision: 6,
    running: false,
    targets: [{ ...operation.targets[0], status: "completed", after }],
  });
  const stale = card({
    ...operation,
    revision: 5,
    running: false,
    targets: [{ ...operation.targets[0], status: "completed", after: { ...after, used_tokens: null } }],
  });
  expect(mergeConversationMessageSnapshot(data(latest), "room", [stale])?.rooms[0].messages[0]).toEqual(latest);
  const lateEvent = applyIMEvent(data(latest), {
    type: "message.created",
    room_id: "room",
    message: card(operation),
  }) as IMData;
  expect(conversationCompacting(lateEvent.rooms[0])).toBe(false);
});
