import { useCallback, useEffect, useRef } from "react";
import { fetchMessagesRequest } from "@/api/im";
import { mergeConversationMessageSnapshot, type IMConversation, type IMData } from "@/models/conversations";
import type { WorkspaceQuerySetter } from "./types";

// SSE is the fast path. Reconcile the open transcript as well: events are not
// replayed when a tab subscribes late or the server's event buffer overflows.
export function useConversationMessageSync(
  roomID: string,
  setData: WorkspaceQuerySetter<IMData | null>,
  conversation?: IMConversation | null,
) {
  const generations = useRef(new Map<string, number>());
  const globalGeneration = useRef(0);
  const invalidate = useCallback((id?: string) => {
    if (id) generations.current.set(id, (generations.current.get(id) ?? 0) + 1);
    else globalGeneration.current += 1;
  }, []);
  const conversationRef = useRef(conversation);
  useEffect(() => {
    conversationRef.current = conversation;
  }, [conversation]);
  const setDataRef = useRef(setData);
  useEffect(() => {
    setDataRef.current = setData;
  }, [setData]);
  useEffect(() => {
    if (!roomID) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function refresh() {
      const generation = generations.current.get(roomID) ?? 0;
      const global = globalGeneration.current;
      const baseline = conversationRef.current?.id === roomID ? conversationRef.current.messages : undefined;
      try {
        const messages = await fetchMessagesRequest(roomID);
        if (!disposed) {
          setDataRef.current((current) => {
            if (
              disposed ||
              global !== globalGeneration.current ||
              generation !== (generations.current.get(roomID) ?? 0)
            )
              return current;
            return mergeConversationMessageSnapshot(current, roomID, messages, baseline);
          });
        }
      } catch {
        // Keep the live transcript intact and retry on the next interval.
      } finally {
        if (!disposed) timer = setTimeout(() => void refresh(), 2000);
      }
    }
    void refresh();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [roomID]);
  return invalidate;
}
