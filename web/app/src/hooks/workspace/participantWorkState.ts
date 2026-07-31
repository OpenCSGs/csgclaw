import { localIdentitiesMatch, participantIDForLocalIdentity } from "@/models/conversations";
import type { ParticipantWorkStage, ParticipantWorkUpdate } from "@/models/conversations";

export const PARTICIPANT_WORK_CLOSED_RETENTION_MS = 70_000;

export type ActiveParticipantWorkLease = ParticipantWorkUpdate & {
  expiresAt: number;
};

export type ClosedParticipantWorkLease = {
  forgetAt: number;
  lastRevision: number;
  leaseID: string;
  participantID: string;
  roomID: string;
};

export type ParticipantWorkState = {
  activeByRoom: Record<string, Record<string, Record<string, ActiveParticipantWorkLease>>>;
  closedByLease: Record<string, ClosedParticipantWorkLease>;
  registryEpoch: string | null;
};

export type ParticipantWorkAction =
  | { now: number; type: "clock" }
  | { participantIDs: string[]; type: "participantLifecycle" }
  | { memberParticipantIDs?: string[]; roomID: string; type: "roomLifecycle" }
  | { type: "reset" }
  | { now: number; type: "workEvent"; work: ParticipantWorkUpdate };

export function createParticipantWorkState(): ParticipantWorkState {
  return {
    activeByRoom: {},
    closedByLease: {},
    registryEpoch: null,
  };
}

export function participantWorkReducer(
  state: ParticipantWorkState,
  action: ParticipantWorkAction,
): ParticipantWorkState {
  switch (action.type) {
    case "reset":
      return createParticipantWorkState();
    case "clock":
      return applyClock(state, action.now);
    case "participantLifecycle":
      return removeParticipants(state, action.participantIDs);
    case "roomLifecycle":
      return updateRoomLifecycle(state, action.roomID, action.memberParticipantIDs);
    case "workEvent":
      return applyWorkEvent(state, action.work, action.now);
  }
}

export function activeParticipantWorkForRoom(
  state: ParticipantWorkState,
  roomID: string | null | undefined,
): Record<string, Record<string, ActiveParticipantWorkLease>> {
  return state.activeByRoom[String(roomID || "").trim()] ?? {};
}

export const activeWorkLeasesForRoom = activeParticipantWorkForRoom;

export function workLeaseForRequest(
  state: ParticipantWorkState,
  roomIDValue: string | null | undefined,
  participantIDValue: string | null | undefined,
  requestIDValue: string | null | undefined,
): ActiveParticipantWorkLease | null {
  const roomID = String(roomIDValue || "").trim();
  const participantID = String(participantIDValue || "").trim();
  const requestID = String(requestIDValue || "").trim();
  if (!roomID || !participantID || !requestID) {
    return null;
  }
  const byParticipant = state.activeByRoom[roomID] ?? {};
  for (const [candidateID, byLease] of Object.entries(byParticipant)) {
    if (!localIdentitiesMatch(candidateID, participantID)) {
      continue;
    }
    const match = Object.values(byLease).find((lease) => lease.request_id === requestID);
    if (match) {
      return match;
    }
  }
  return null;
}

export function nextParticipantWorkDeadline(state: ParticipantWorkState): number | null {
  let next: number | null = null;
  Object.values(state.activeByRoom).forEach((byParticipant) => {
    Object.values(byParticipant).forEach((byLease) => {
      Object.values(byLease).forEach((lease) => {
        next = next === null ? lease.expiresAt : Math.min(next, lease.expiresAt);
      });
    });
  });
  Object.values(state.closedByLease).forEach((closed) => {
    next = next === null ? closed.forgetAt : Math.min(next, closed.forgetAt);
  });
  return next;
}

function applyWorkEvent(current: ParticipantWorkState, work: ParticipantWorkUpdate, now: number): ParticipantWorkState {
  const normalized = normalizeWorkEvent(work);
  if (!normalized || !Number.isFinite(now)) {
    return current;
  }
  let state = current;
  if (state.registryEpoch !== normalized.registry_epoch) {
    state = { ...createParticipantWorkState(), registryEpoch: normalized.registry_epoch };
  }

  const existingLocation = findLeaseLocation(state, normalized.lease_id);
  if (
    existingLocation &&
    (existingLocation.participantID !== normalized.participant_id || existingLocation.roomID !== normalized.room_id)
  ) {
    return state;
  }
  const key = closedLeaseKey(normalized.participant_id, normalized.lease_id);
  const active = state.activeByRoom[normalized.room_id]?.[normalized.participant_id]?.[normalized.lease_id];
  const lastRevision = active?.revision ?? state.closedByLease[key]?.lastRevision ?? 0;
  if (normalized.revision <= lastRevision) {
    return state;
  }

  if (normalized.state === "working") {
    const expiresAt = Date.parse(normalized.expires_at);
    if (expiresAt <= now) {
      return closeLease(
        state,
        normalized.room_id,
        normalized.participant_id,
        normalized.lease_id,
        normalized.revision,
        now,
      );
    }
    const activeByRoom = cloneActiveWithLease(state.activeByRoom, normalized.room_id, normalized.participant_id);
    activeByRoom[normalized.room_id][normalized.participant_id][normalized.lease_id] = {
      ...normalized,
      expiresAt,
    };
    const closedByLease = { ...state.closedByLease };
    delete closedByLease[key];
    return {
      ...state,
      activeByRoom,
      closedByLease,
    };
  }

  return closeLease(
    state,
    normalized.room_id,
    normalized.participant_id,
    normalized.lease_id,
    normalized.revision,
    now,
  );
}

function applyClock(state: ParticipantWorkState, now: number): ParticipantWorkState {
  if (!Number.isFinite(now)) {
    return state;
  }
  let next = state;
  Object.entries(state.activeByRoom).forEach(([roomID, byParticipant]) => {
    Object.entries(byParticipant).forEach(([participantID, byLease]) => {
      Object.entries(byLease).forEach(([leaseID, lease]) => {
        if (lease.expiresAt <= now) {
          next = closeLease(next, roomID, participantID, leaseID, lease.revision, now);
        }
      });
    });
  });
  const retainedClosed = Object.fromEntries(
    Object.entries(next.closedByLease).filter(([, closed]) => closed.forgetAt > now),
  );
  if (Object.keys(retainedClosed).length !== Object.keys(next.closedByLease).length) {
    next = { ...next, closedByLease: retainedClosed };
  }
  return next;
}

function closeLease(
  state: ParticipantWorkState,
  roomID: string,
  participantID: string,
  leaseID: string,
  revision: number,
  now: number,
): ParticipantWorkState {
  const activeByRoom = cloneActiveWithoutLease(state.activeByRoom, roomID, participantID, leaseID);
  return {
    ...state,
    activeByRoom,
    closedByLease: {
      ...state.closedByLease,
      [closedLeaseKey(participantID, leaseID)]: {
        forgetAt: now + PARTICIPANT_WORK_CLOSED_RETENTION_MS,
        lastRevision: revision,
        leaseID,
        participantID,
        roomID,
      },
    },
  };
}

function removeParticipants(state: ParticipantWorkState, participantIDs: readonly string[]): ParticipantWorkState {
  const targets = participantIDs.map((value) => String(value || "").trim()).filter(Boolean);
  if (!targets.length) {
    return state;
  }
  const matchesTarget = (participantID: string) =>
    targets.some((target) => localIdentitiesMatch(target, participantID));
  const activeByRoom: ParticipantWorkState["activeByRoom"] = {};
  Object.entries(state.activeByRoom).forEach(([roomID, byParticipant]) => {
    const retained = Object.fromEntries(
      Object.entries(byParticipant).filter(([participantID]) => !matchesTarget(participantID)),
    );
    if (Object.keys(retained).length) {
      activeByRoom[roomID] = retained;
    }
  });
  const closedByLease = Object.fromEntries(
    Object.entries(state.closedByLease).filter(([, closed]) => !matchesTarget(closed.participantID)),
  );
  return { ...state, activeByRoom, closedByLease };
}

function updateRoomLifecycle(
  state: ParticipantWorkState,
  roomIDValue: string,
  memberParticipantIDs?: readonly string[],
): ParticipantWorkState {
  const roomID = String(roomIDValue || "").trim();
  if (!roomID) {
    return state;
  }
  if (!memberParticipantIDs) {
    const activeByRoom = { ...state.activeByRoom };
    delete activeByRoom[roomID];
    const closedByLease = Object.fromEntries(
      Object.entries(state.closedByLease).filter(([, closed]) => closed.roomID !== roomID),
    );
    return { ...state, activeByRoom, closedByLease };
  }
  const members = memberParticipantIDs.map((value) => String(value || "").trim()).filter(Boolean);
  const isMember = (participantID: string) => members.some((memberID) => localIdentitiesMatch(memberID, participantID));
  const activeByRoom = { ...state.activeByRoom };
  const retainedActive = Object.fromEntries(
    Object.entries(state.activeByRoom[roomID] ?? {}).filter(([participantID]) => isMember(participantID)),
  );
  if (Object.keys(retainedActive).length) activeByRoom[roomID] = retainedActive;
  else delete activeByRoom[roomID];
  const closedByLease = Object.fromEntries(
    Object.entries(state.closedByLease).filter(
      ([, closed]) => closed.roomID !== roomID || isMember(closed.participantID),
    ),
  );
  return { ...state, activeByRoom, closedByLease };
}

function normalizeWorkEvent(work: ParticipantWorkUpdate): ParticipantWorkUpdate | null {
  const registryEpoch = String(work?.registry_epoch || "").trim();
  const leaseID = String(work?.lease_id || "").trim();
  const rawParticipantID = String(work?.participant_id || "").trim();
  const participantID = rawParticipantID.startsWith("pt-")
    ? rawParticipantID
    : participantIDForLocalIdentity(rawParticipantID);
  const userID = String(work?.user_id || "").trim();
  const roomID = String(work?.room_id || "").trim();
  const requestID = String(work?.request_id || "").trim();
  const expiresAt = Date.parse(String(work?.expires_at || ""));
  const revision = Number(work?.revision);
  if (
    !registryEpoch ||
    !leaseID ||
    !participantID ||
    !userID ||
    !roomID ||
    !requestID ||
    work?.kind !== "agent_turn" ||
    (work?.state !== "working" && work?.state !== "idle") ||
    ![
      "started",
      "renewed",
      "status_updated",
      "stop_requested",
      "stop_failed",
      "stop_timed_out",
      "released",
      "stopped",
      "expired",
    ].includes(work?.reason) ||
    !Number.isInteger(revision) ||
    revision <= 0 ||
    !Number.isFinite(expiresAt)
  ) {
    return null;
  }
  const capabilities = Array.isArray(work.capabilities)
    ? work.capabilities.filter(
        (value): value is "thinking_status_v1" | "turn_stop_v1" | "work_stage_v1" =>
          value === "thinking_status_v1" || value === "turn_stop_v1" || value === "work_stage_v1",
      )
    : undefined;
  const status = normalizeParticipantWorkStatus(work.status);
  if (work.status && !status) {
    return null;
  }
  const stopRequestedAt = String(work.stop_requested_at || "").trim();
  if (stopRequestedAt && !Number.isFinite(Date.parse(stopRequestedAt))) {
    return null;
  }
  const stopState = String(work.stop_state || "").trim();
  if (stopState && !["stop_requested", "stop_failed", "stop_timed_out", "stopped"].includes(stopState)) {
    return null;
  }
  return {
    ...work,
    capabilities,
    lease_id: leaseID,
    participant_id: participantID,
    registry_epoch: registryEpoch,
    request_id: requestID,
    revision,
    room_id: roomID,
    status,
    stop_error: String(work.stop_error || "").trim() || undefined,
    stop_requested_at: stopRequestedAt || undefined,
    stop_state: (stopState || undefined) as ParticipantWorkUpdate["stop_state"],
    thread_root_id: String(work.thread_root_id || "").trim() || undefined,
    user_id: userID,
  };
}

function normalizeParticipantWorkStatus(
  status: ParticipantWorkUpdate["status"],
): ParticipantWorkUpdate["status"] | undefined {
  if (!status) {
    return undefined;
  }
  const sequence = Number(status.sequence);
  if (!Number.isInteger(sequence) || sequence <= 0 || (status.phase !== "working" && status.phase !== "thinking")) {
    return undefined;
  }
  const stage = normalizeParticipantWorkStage(status.stage, status.phase);
  if (status.phase === "working") {
    return { phase: "working", sequence, stage };
  }
  if (!status.thinking) {
    return { phase: "thinking", sequence, stage };
  }
  if (
    status.thinking.format !== "plain_text" ||
    typeof status.thinking.text !== "string" ||
    typeof status.thinking.truncated !== "boolean"
  ) {
    return undefined;
  }
  return {
    phase: "thinking",
    sequence,
    stage,
    thinking: {
      format: "plain_text",
      text: status.thinking.text,
      truncated: status.thinking.truncated,
    },
  };
}

function normalizeParticipantWorkStage(
  stage: ParticipantWorkStage | null | undefined,
  phase: "working" | "thinking",
): ParticipantWorkStage | undefined {
  if (phase === "working" && (stage === "running_tool" || stage === "generating_reply")) {
    return stage;
  }
  if (phase === "thinking" && (stage === "preparing_reply" || stage === "thinking")) {
    return stage;
  }
  return undefined;
}

function findLeaseLocation(
  state: ParticipantWorkState,
  leaseID: string,
): { participantID: string; roomID: string } | null {
  for (const [roomID, byParticipant] of Object.entries(state.activeByRoom)) {
    for (const [participantID, byLease] of Object.entries(byParticipant)) {
      if (byLease[leaseID]) {
        return { participantID, roomID };
      }
    }
  }
  const closed = Object.values(state.closedByLease).find((item) => item.leaseID === leaseID);
  return closed ? { participantID: closed.participantID, roomID: closed.roomID } : null;
}

function cloneActiveWithLease(
  source: ParticipantWorkState["activeByRoom"],
  roomID: string,
  participantID: string,
): ParticipantWorkState["activeByRoom"] {
  return {
    ...source,
    [roomID]: {
      ...(source[roomID] ?? {}),
      [participantID]: { ...(source[roomID]?.[participantID] ?? {}) },
    },
  };
}

function cloneActiveWithoutLease(
  source: ParticipantWorkState["activeByRoom"],
  roomID: string,
  participantID: string,
  leaseID: string,
): ParticipantWorkState["activeByRoom"] {
  const byLease = { ...(source[roomID]?.[participantID] ?? {}) };
  if (!byLease[leaseID]) {
    return source;
  }
  delete byLease[leaseID];
  const byParticipant = { ...(source[roomID] ?? {}) };
  if (Object.keys(byLease).length) byParticipant[participantID] = byLease;
  else delete byParticipant[participantID];
  const activeByRoom = { ...source };
  if (Object.keys(byParticipant).length) activeByRoom[roomID] = byParticipant;
  else delete activeByRoom[roomID];
  return activeByRoom;
}

function closedLeaseKey(participantID: string, leaseID: string): string {
  return `${participantID}\u0000${leaseID}`;
}
