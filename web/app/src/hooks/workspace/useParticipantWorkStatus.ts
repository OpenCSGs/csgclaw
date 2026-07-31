import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { errorMessage } from "@/api/client";
import { stopParticipantWorkRequest } from "@/api/im";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import type { AgentLike } from "@/models/agents";
import {
  agentMatchesUser,
  buildUsersById,
  localIdentitiesMatch,
  resolveUserByLocalIdentity,
} from "@/models/conversations";
import type { IMServerEvent, IMUser } from "@/models/conversations";
import {
  activeParticipantWorkForRoom,
  createParticipantWorkState,
  nextParticipantWorkDeadline,
  participantWorkReducer,
} from "./participantWorkState";

type UseParticipantWorkStatusArgs = {
  agents: readonly AgentLike[];
  users: readonly IMUser[];
};

export type ParticipantWorkStatus = {
  handleRealtimeEvent: (event: IMServerEvent) => void;
  hasObservedWorkLease: (participantID: string | null | undefined) => boolean;
  stopWorkingTurn: (participant: ConversationWorkingParticipant) => Promise<void>;
  workingParticipantsForRoom: (roomID: string | null | undefined) => ConversationWorkingParticipant[];
};

export function useParticipantWorkStatus({ agents, users }: UseParticipantWorkStatusArgs): ParticipantWorkStatus {
  const [state, dispatch] = useReducer(participantWorkReducer, undefined, createParticipantWorkState);
  const [stopRequests, setStopRequests] = useState<Record<string, { error?: string; state: "sending" | "accepted" }>>(
    {},
  );
  const observedLeaseIdentitiesRef = useRef(new Set<string>());
  const usersById = useMemo(() => buildUsersById(users), [users]);

  useEffect(() => {
    const deadline = nextParticipantWorkDeadline(state);
    if (deadline === null) {
      return;
    }
    const timer = window.setTimeout(
      () => {
        dispatch({ now: Date.now(), type: "clock" });
      },
      Math.max(0, deadline - Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [state]);

  const handleRealtimeEvent = useCallback((event: IMServerEvent) => {
    if (event?.type === "participant.work.updated" && event.work) {
      [event.work.participant_id, event.work.user_id].forEach((value) => {
        const id = String(value || "").trim();
        if (id) {
          observedLeaseIdentitiesRef.current.add(id);
        }
      });
      dispatch({ now: Date.now(), type: "workEvent", work: event.work });
      return;
    }
    if ((event?.type === "participant.updated" || event?.type === "participant.deleted") && event.participant) {
      const participantIDs = [
        event.participant.id,
        event.participant.channel_user_ref,
        event.participant.user_id,
      ].filter((value): value is string => Boolean(value));
      participantIDs.forEach((id) => observedLeaseIdentitiesRef.current.delete(id));
      dispatch({ participantIDs, type: "participantLifecycle" });
      return;
    }
    if (event?.type === "room.deleted") {
      dispatch({ roomID: String(event.room_id || event.room?.id || ""), type: "roomLifecycle" });
      return;
    }
    if (event?.type === "room.members_removed" && event.room?.id && Array.isArray(event.room.members)) {
      dispatch({
        memberParticipantIDs: event.room.members,
        roomID: event.room.id,
        type: "roomLifecycle",
      });
    }
  }, []);

  const hasObservedWorkLease = useCallback((participantID: string | null | undefined): boolean => {
    const id = String(participantID || "").trim();
    return Boolean(id && observedLeaseIdentitiesRef.current.has(id));
  }, []);

  useEffect(() => {
    const activeLeaseIDs = new Set<string>();
    Object.values(state.activeByRoom).forEach((byParticipant) => {
      Object.values(byParticipant).forEach((byLease) => {
        Object.keys(byLease).forEach((leaseID) => activeLeaseIDs.add(leaseID));
      });
    });
    setStopRequests((current) => {
      const retained = Object.fromEntries(Object.entries(current).filter(([leaseID]) => activeLeaseIDs.has(leaseID)));
      return Object.keys(retained).length === Object.keys(current).length ? current : retained;
    });
  }, [state.activeByRoom, stopRequests]);

  const stopWorkingTurn = useCallback(async (participant: ConversationWorkingParticipant) => {
    const participantID = String(participant.participantID || "").trim();
    const leaseID = String(participant.leaseID || "").trim();
    const roomID = String(participant.roomID || "").trim();
    const requestID = String(participant.requestID || "").trim();
    if (!participantID || !leaseID || !roomID || !requestID) {
      return;
    }
    setStopRequests((current) => ({ ...current, [leaseID]: { state: "sending" } }));
    try {
      await stopParticipantWorkRequest(participantID, {
        lease_id: leaseID,
        request_id: requestID,
        room_id: roomID,
      });
      setStopRequests((current) => ({ ...current, [leaseID]: { state: "accepted" } }));
    } catch (error) {
      setStopRequests((current) => ({
        ...current,
        [leaseID]: {
          error: errorMessage(error, "Failed to request turn stop"),
          state: "sending",
        },
      }));
    }
  }, []);

  const workingParticipantsForRoom = useCallback(
    (roomIDValue: string | null | undefined): ConversationWorkingParticipant[] => {
      const roomID = String(roomIDValue || "").trim();
      if (!roomID) {
        return [];
      }
      const result: ConversationWorkingParticipant[] = [];
      Object.entries(activeParticipantWorkForRoom(state, roomID)).forEach(([participantID, byLease]) => {
        Object.values(byLease).forEach((lease) => {
          const user =
            resolveUserByLocalIdentity(lease.user_id, usersById) ??
            resolveUserByLocalIdentity(participantID, usersById);
          const agent = agents.find(
            (candidate) =>
              localIdentitiesMatch(candidate.id, participantID) ||
              localIdentitiesMatch(candidate.user_id, lease.user_id) ||
              (user ? agentMatchesUser(candidate, user) : false),
          );
          const id = String(user?.id || lease.user_id).trim();
          const name = String(agent?.name || user?.name || id).trim();
          if (!id || !name) {
            return;
          }
          const stopRequest = stopRequests[lease.lease_id];
          const thinking = lease.status?.phase === "thinking" ? (lease.status.thinking?.text ?? "") : undefined;
          result.push({
            canStop: lease.capabilities?.includes("turn_stop_v1") === true,
            id,
            leaseID: lease.lease_id,
            name,
            participantID: lease.participant_id,
            requestID: lease.request_id,
            roomID: lease.room_id,
            stopError: stopRequest?.error || lease.stop_error || undefined,
            stopSending: stopRequest?.state === "sending" && !stopRequest.error,
            stopping: Boolean(
              lease.stop_state === "stop_requested" ||
              (!lease.stop_state && lease.stop_requested_at) ||
              stopRequest?.state === "accepted",
            ),
            thinkingText: thinking,
            thinkingTruncated: lease.status?.thinking?.truncated === true,
            workStage: lease.status?.stage ?? undefined,
          });
        });
      });
      return result.sort(
        (left, right) =>
          left.name.localeCompare(right.name) ||
          String(left.requestID || "").localeCompare(String(right.requestID || "")),
      );
    },
    [agents, state, stopRequests, usersById],
  );

  return { handleRealtimeEvent, hasObservedWorkLease, stopWorkingTurn, workingParticipantsForRoom };
}
