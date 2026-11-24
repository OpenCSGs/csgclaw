import { useCallback, useEffect, useRef } from "react";
import { apiErrorCode } from "@/api/client";
import { fetchMessagesRequest } from "@/api/im";
import type { IMConversation } from "@/models/conversations";
import { fetchWorkspaceBootstrapData } from "./workspaceQueries";

const RECOVERY_RETRY_DELAY_MS = 500;

function isTransientRecoveryError(error: unknown): boolean {
  const status = error && typeof error === "object" && "status" in error ? error.status : undefined;
  return error instanceof TypeError || status === 408 || (typeof status === "number" && status >= 500 && status < 600);
}

export function useMissingConversationRoute(
  roomID: string | undefined,
  onMissing: (roomID: string) => void,
  onFound: (room: IMConversation) => void,
) {
  const retryRef = useRef<(() => void) | null>(null);
  useEffect(() => {
    if (!roomID) return;
    let current = true;
    let inFlight = false;
    let retryPending = false;
    let settled = false;
    let automaticRetryUsed = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    async function reconcile(id: string) {
      // The transcript endpoint distinguishes a missing room from a stale cache.
      try {
        await fetchMessagesRequest(id);
      } catch (error) {
        if (apiErrorCode(error) !== "room_not_found") throw error;
        if (current) {
          settled = true;
          onMissing(id);
        }
        return;
      }
      if (!current) return;

      // Only publish the recovered room so a stale result cannot replace other live data.
      const bootstrap = await fetchWorkspaceBootstrapData();
      const room = bootstrap.rooms.find((candidate) => candidate.id === id);
      if (current && room) {
        settled = true;
        onFound(room);
      }
    }
    function retry(id: string) {
      if (!current || settled) return;
      if (inFlight) {
        retryPending = true;
        return;
      }
      clearTimeout(retryTimer);
      inFlight = true;
      retryPending = false;
      void reconcile(id)
        .catch((error: unknown) => {
          if (!current || retryPending || automaticRetryUsed || !isTransientRecoveryError(error)) return;
          automaticRetryUsed = true;
          retryTimer = setTimeout(() => retry(id), RECOVERY_RETRY_DELAY_MS);
        })
        .finally(() => {
          inFlight = false;
          if (retryPending) retry(id);
        });
    }
    retryRef.current = () => retry(roomID);
    retry(roomID);
    return () => {
      current = false;
      clearTimeout(retryTimer);
      retryRef.current = null;
    };
  }, [roomID, onMissing, onFound]);

  return useCallback(() => retryRef.current?.(), []);
}
