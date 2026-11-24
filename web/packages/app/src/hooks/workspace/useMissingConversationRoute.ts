import { useEffect } from "react";
import { apiErrorCode } from "@/api/client";
import { fetchMessagesRequest } from "@/api/im";
import type { IMConversation } from "@/models/conversations";
import { fetchWorkspaceBootstrapData } from "./workspaceQueries";

export function useMissingConversationRoute(
  roomID: string | undefined,
  bootstrapUpdatedAt: number,
  onMissing: (roomID: string) => void,
  onFound: (room: IMConversation) => void,
) {
  useEffect(() => {
    if (!roomID || !bootstrapUpdatedAt) return;
    let current = true;
    async function reconcile(id: string) {
      // The transcript endpoint distinguishes a missing room from a stale cache.
      try {
        await fetchMessagesRequest(id);
      } catch (error) {
        if (current && apiErrorCode(error) === "room_not_found") onMissing(id);
        return;
      }
      if (!current) return;

      // Only publish the recovered room so a stale result cannot replace other live data.
      const bootstrap = await fetchWorkspaceBootstrapData().catch(() => null);
      const room = bootstrap?.rooms.find((candidate) => candidate.id === id);
      if (current && room) onFound(room);
    }
    void reconcile(roomID);
    return () => {
      current = false;
    };
  }, [roomID, bootstrapUpdatedAt, onMissing, onFound]);
}
