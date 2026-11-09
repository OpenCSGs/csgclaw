import { describe, expect, it } from "vitest";
import type { ParticipantWorkUpdate } from "@/models/conversations";
import {
  activeParticipantWorkForRoom,
  createParticipantWorkState,
  nextParticipantWorkDeadline,
  participantWorkReducer,
  workLeaseForRequest,
} from "@/hooks/workspace/participantWorkState";

const now = Date.parse("2026-07-14T12:00:00Z");

function work(overrides: Partial<ParticipantWorkUpdate> = {}): ParticipantWorkUpdate {
  return {
    expires_at: "2026-07-14T12:00:15Z",
    kind: "agent_turn",
    lease_id: "lease-1",
    participant_id: "pt-worker",
    reason: "started",
    registry_epoch: "epoch-1",
    request_id: "message-1",
    revision: 1,
    room_id: "room-1",
    state: "working",
    user_id: "user-worker",
    ...overrides,
  };
}

describe("participantWorkReducer", () => {
  it("handles start, renew, release, and out-of-order events", () => {
    let state = participantWorkReducer(createParticipantWorkState(), { now, type: "workEvent", work: work() });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]?.["lease-1"]?.revision).toBe(1);

    state = participantWorkReducer(state, {
      now: now + 5_000,
      type: "workEvent",
      work: work({ expires_at: "2026-07-14T12:00:20Z", reason: "renewed", revision: 2 }),
    });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]?.["lease-1"]?.revision).toBe(2);

    state = participantWorkReducer(state, {
      now: now + 6_000,
      type: "workEvent",
      work: work({ reason: "released", revision: 3, state: "idle" }),
    });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]).toBeUndefined();

    state = participantWorkReducer(state, {
      now: now + 7_000,
      type: "workEvent",
      work: work({ reason: "renewed", revision: 2 }),
    });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]).toBeUndefined();
  });

  it("keeps concurrent leases independent and expires them with one clock action", () => {
    let state = participantWorkReducer(createParticipantWorkState(), { now, type: "workEvent", work: work() });
    state = participantWorkReducer(state, {
      now,
      type: "workEvent",
      work: work({ lease_id: "lease-2", request_id: "message-2" }),
    });
    expect(Object.keys(activeParticipantWorkForRoom(state, "room-1")["pt-worker"] ?? {})).toHaveLength(2);

    state = participantWorkReducer(state, {
      now: now + 1_000,
      type: "workEvent",
      work: work({ reason: "released", revision: 2, state: "idle" }),
    });
    expect(Object.keys(activeParticipantWorkForRoom(state, "room-1")["pt-worker"] ?? {})).toEqual(["lease-2"]);
    expect(nextParticipantWorkDeadline(state)).toBe(Date.parse("2026-07-14T12:00:15Z"));

    state = participantWorkReducer(state, { now: now + 15_000, type: "clock" });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]).toBeUndefined();

    state = participantWorkReducer(state, {
      now: now + 16_000,
      type: "workEvent",
      work: work({
        expires_at: "2026-07-14T12:00:30Z",
        lease_id: "lease-2",
        reason: "renewed",
        revision: 2,
      }),
    });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]?.["lease-2"]?.revision).toBe(2);
  });

  it("switches epochs and rejects attempts to move a lease", () => {
    let state = participantWorkReducer(createParticipantWorkState(), { now, type: "workEvent", work: work() });
    state = participantWorkReducer(state, {
      now,
      type: "workEvent",
      work: work({ lease_id: "lease-move", request_id: "message-move" }),
    });
    const unchanged = participantWorkReducer(state, {
      now,
      type: "workEvent",
      work: work({ lease_id: "lease-move", participant_id: "pt-other", revision: 2, user_id: "user-other" }),
    });
    expect(unchanged).toBe(state);

    state = participantWorkReducer(state, {
      now,
      type: "workEvent",
      work: work({ lease_id: "lease-new", registry_epoch: "epoch-2" }),
    });
    expect(state.registryEpoch).toBe("epoch-2");
    expect(Object.keys(activeParticipantWorkForRoom(state, "room-1")["pt-worker"] ?? {})).toEqual(["lease-new"]);
  });

  it("cleans participant and room lifecycle state", () => {
    let state = participantWorkReducer(createParticipantWorkState(), { now, type: "workEvent", work: work() });
    state = participantWorkReducer(state, {
      participantIDs: ["u-worker"],
      type: "participantLifecycle",
    });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]).toBeUndefined();

    state = participantWorkReducer(state, { now, type: "workEvent", work: work() });
    state = participantWorkReducer(state, { roomID: "room-1", type: "roomLifecycle" });
    expect(activeParticipantWorkForRoom(state, "room-1")).toEqual({});
  });

  it("preserves canonical participant ids while matching display aliases", () => {
    const canonicalID = "pt-agent-worker-d59735ad";
    const state = participantWorkReducer(createParticipantWorkState(), {
      now,
      type: "workEvent",
      work: work({ participant_id: canonicalID, user_id: "user-agent-worker-d59735ad" }),
    });
    expect(activeParticipantWorkForRoom(state, "room-1")[canonicalID]?.["lease-1"]).toBeDefined();
  });

  it("replaces thinking snapshots, preserves stopping leases, and selects exact turns", () => {
    let state = participantWorkReducer(createParticipantWorkState(), {
      now,
      type: "workEvent",
      work: work({
        capabilities: ["thinking_status_v1", "turn_stop_v1", "work_stage_v1"],
        reason: "status_updated",
        revision: 2,
        status: {
          phase: "thinking",
          sequence: 1,
          stage: "thinking",
          thinking: { format: "plain_text", text: "checking config", truncated: false },
        },
      }),
    });
    expect(workLeaseForRequest(state, "room-1", "user-worker", "message-1")?.status?.thinking?.text).toBe(
      "checking config",
    );
    expect(workLeaseForRequest(state, "room-1", "user-worker", "message-1")?.status?.stage).toBe("thinking");

    state = participantWorkReducer(state, {
      now,
      type: "workEvent",
      work: work({
        capabilities: ["thinking_status_v1", "turn_stop_v1", "work_stage_v1"],
        reason: "stop_requested",
        revision: 3,
        status: {
          phase: "thinking",
          sequence: 1,
          stage: "thinking",
          thinking: { format: "plain_text", text: "checking config", truncated: false },
        },
        stop_requested_at: "2026-07-14T12:00:01Z",
        stop_state: "stop_requested",
      }),
    });
    expect(activeParticipantWorkForRoom(state, "room-1")["pt-worker"]?.["lease-1"]?.stop_requested_at).toBe(
      "2026-07-14T12:00:01Z",
    );

    state = participantWorkReducer(state, {
      now,
      type: "workEvent",
      work: work({ reason: "stopped", revision: 4, state: "idle" }),
    });
    expect(workLeaseForRequest(state, "room-1", "pt-worker", "message-1")).toBeNull();
  });
});
