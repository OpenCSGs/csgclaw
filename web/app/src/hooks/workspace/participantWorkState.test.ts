import { describe, expect, it } from "vitest";
import type { ParticipantWorkUpdate } from "@/models/conversations";
import {
  activeParticipantWorkForRoom,
  createParticipantWorkState,
  participantWorkReducer,
} from "./participantWorkState";

function workUpdate(overrides: Partial<ParticipantWorkUpdate> = {}): ParticipantWorkUpdate {
  return {
    capabilities: ["turn_stop_v1"],
    expires_at: "2026-09-28T02:00:00.000Z",
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
  it("keeps a locally stopped lease closed when a delayed stop_requested update arrives", () => {
    const started = participantWorkReducer(createParticipantWorkState(), {
      now: Date.parse("2026-09-28T01:00:00.000Z"),
      type: "workEvent",
      work: workUpdate(),
    });

    const startedWorker = activeParticipantWorkForRoom(started, "room-1")["pt-worker"];
    expect(startedWorker?.["lease-1"]).toBeTruthy();

    const closed = participantWorkReducer(started, {
      leaseID: "lease-1",
      participantID: "pt-worker",
      roomID: "room-1",
      type: "closeLeaseLocally",
    });

    const closedWorker = activeParticipantWorkForRoom(closed, "room-1")["pt-worker"];
    expect(closedWorker?.["lease-1"]).toBeUndefined();

    const delayedStopRequested = participantWorkReducer(closed, {
      now: Date.parse("2026-09-28T01:00:01.000Z"),
      type: "workEvent",
      work: workUpdate({
        reason: "stop_requested",
        revision: 2,
        stop_requested_at: "2026-09-28T01:00:01.000Z",
        stop_state: "stop_requested",
      }),
    });

    const delayedWorker = activeParticipantWorkForRoom(delayedStopRequested, "room-1")["pt-worker"];
    expect(delayedWorker?.["lease-1"]).toBeUndefined();
  });
});
