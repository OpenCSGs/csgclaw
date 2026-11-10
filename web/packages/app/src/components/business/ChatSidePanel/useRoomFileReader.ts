import { useCallback, useContext } from "react";
import { QueryClientContext } from "@tanstack/react-query";
import { fetchRoomFiles, fetchRoomFileMessages, roomFilePage, transcriptRoomFiles } from "@/api/chatFiles";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import type { ChatFileScope } from "./types";

export function useRoomFileReader(scope?: ChatFileScope) {
  const client = useContext(QueryClientContext);
  const roomID = scope?.roomID ?? "";
  const retained = scope?.retainedMessages;
  const desktop = Boolean(getDesktopBridge());
  const readTranscript = useCallback(async () => {
    if (!roomID) return [];
    const messages = client
      ? await client.fetchQuery({
          queryKey: ["chat-file-transcript", roomID],
          queryFn: ({ signal }) => fetchRoomFileMessages(roomID, signal),
          staleTime: 10_000,
        })
      : await fetchRoomFileMessages(roomID);
    return transcriptRoomFiles([...messages, ...(retained ?? [])]);
  }, [client, roomID, retained]);
  const read = useCallback(
    async (query = "", from = 0, signal?: AbortSignal) => {
      if (desktop) return fetchRoomFiles(roomID, query, from, signal);
      return roomFilePage(await readTranscript(), query, from);
    },
    [desktop, roomID, readTranscript],
  );
  const resolve = useCallback(
    async (id: string) => (await readTranscript()).find((file) => file.id === id),
    [readTranscript],
  );
  const refresh = useCallback(
    () => client?.invalidateQueries({ queryKey: ["chat-file-transcript", roomID] }),
    [client, roomID],
  );
  return { read, resolve, refresh, desktop };
}
