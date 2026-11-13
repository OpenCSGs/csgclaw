import { useCallback, useEffect, useState } from "react";
import { fetchBrowserSessions, type BrowserSession } from "@/api/browser";

// Keep the live browser inventory scoped to its mounted chat. Browser sessions
// are ephemeral and should not be restored from a stale query or localStorage.
export function useRoomBrowsers(roomID: string) {
  const [snapshot, setSnapshot] = useState<{ roomID: string; data?: BrowserSession[]; error?: boolean }>({ roomID });
  const [retry, setRetry] = useState(0);
  const refetch = useCallback(() => setRetry((value) => value + 1), []);
  useEffect(() => {
    if (!roomID) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const data = await fetchBrowserSessions(roomID, controller.signal);
        if (!Array.isArray(data)) throw new Error("Invalid browser inventory");
        if (!controller.signal.aborted) setSnapshot({ roomID, data });
      } catch {
        if (!controller.signal.aborted) setSnapshot((current) => ({ ...current, roomID, error: true }));
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 2000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [roomID, retry]);
  return {
    data: snapshot.roomID === roomID ? snapshot.data : undefined,
    error: snapshot.roomID === roomID && snapshot.error,
    refetch,
  };
}
