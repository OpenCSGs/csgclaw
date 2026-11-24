import { useEffect } from "react";
import { apiErrorCode } from "@/api/client";
import { fetchMessagesRequest } from "@/api/im";

export function useMissingConversationRoute(
  roomID: string | undefined,
  bootstrapUpdatedAt: number,
  onMissing: (roomID: string) => void,
) {
  useEffect(() => {
    if (!roomID || !bootstrapUpdatedAt) return;
    let current = true;
    // The bootstrap cache can be stale, so confirm absence against the room endpoint.
    void fetchMessagesRequest(roomID).catch((error: unknown) => {
      if (current && apiErrorCode(error) === "room_not_found") onMissing(roomID);
    });
    return () => {
      current = false;
    };
  }, [roomID, bootstrapUpdatedAt, onMissing]);
}
