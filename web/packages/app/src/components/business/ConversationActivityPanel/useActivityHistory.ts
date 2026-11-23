import { useEffect, useRef, useState } from "react";
import { fetchMessagePageRequest } from "@/api/im";
import type { IMMessage } from "@/models/conversations";
import { newerProgressMessage } from "@/models/turnProgress";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import { mergeConversationActivityMessages } from "./conversationActivity";

export const activityHistoryPageSize = 100;
const refreshOverlap = 8;
const liveBatchMs = 80;

type History = {
  roomID: string;
  messages: IMMessage[];
  before: string;
  tail: string[];
  loaded: boolean;
  cleared?: boolean;
  loading: boolean;
  loadingOlder: boolean;
  error: unknown;
};

function emptyHistory(roomID: string): History {
  return {
    roomID,
    messages: [],
    before: "",
    tail: [],
    loaded: false,
    loading: false,
    loadingOlder: false,
    error: null,
  };
}

export function useActivityHistory(roomID: string, active: boolean) {
  const [history, setHistory] = useState(() => emptyHistory(roomID));
  const current = useRef(history);
  const actions = useRef<{ refresh(): void; loadOlder(): void } | null>(null);

  useEffect(() => {
    if (!active) return;
    let request: AbortController | null = null;
    let refreshPending = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const live = new Map<string, IMMessage>();
    const duringRequest = new Map<string, IMMessage>();

    const commit = (next: History) => {
      current.current = next;
      setHistory(next);
    };
    if (current.current.roomID !== roomID) commit(emptyHistory(roomID));

    const flush = () => {
      clearTimeout(timer);
      timer = undefined;
      if (live.size === 0) return;
      const currentHistory = current.current;
      commit({
        ...currentHistory,
        messages: mergeConversationActivityMessages(currentHistory.messages, [...live.values()]),
      });
      live.clear();
    };

    async function read(older = false) {
      if (request) {
        if (!older) refreshPending = true;
        return;
      }
      const start = current.current;
      if (older && !start.before) return;
      const controller = new AbortController();
      request = controller;
      duringRequest.clear();
      commit({ ...start, loading: true, loadingOlder: older, error: null });
      const options = { limit: activityHistoryPageSize, signal: controller.signal };

      try {
        let before = older ? start.before : undefined;
        let after = older ? undefined : start.tail[0];
        let page = await fetchMessagePageRequest(roomID, { ...options, before, after });
        let reset = Boolean(page.cursor_missing);
        if (reset) {
          before = undefined;
          after = undefined;
          page = await fetchMessagePageRequest(roomID, options);
        }
        const fetched = page.anchor ? [page.anchor, ...page.messages] : [...page.messages];
        let olderCursor = start.before;
        if (!after) olderCursor = page.has_more ? page.messages[0]?.id || "" : "";

        // SSE does not replay missed events. Catch up from the last HTTP page
        // and reread a small tail for revisions of in-flight tool calls.
        while (after && page.has_more) {
          after = page.messages.at(-1)?.id;
          if (!after) break;
          page = await fetchMessagePageRequest(roomID, { ...options, after });
          if (page.cursor_missing) {
            page = await fetchMessagePageRequest(roomID, options);
            fetched.splice(0, fetched.length, ...page.messages);
            olderCursor = page.has_more ? page.messages[0]?.id || "" : "";
            reset = true;
            break;
          }
          fetched.push(...page.messages);
        }
        if (controller.signal.aborted) return;
        let tail = reset ? [] : start.tail;
        if (!before && fetched.length) {
          tail = fetched.slice(-refreshOverlap).flatMap((message) => (message.id ? [message.id] : []));
        }
        flush();
        const existing = reset ? [] : current.current.messages;
        const pending = [...duringRequest.values()];
        commit({
          roomID,
          messages: before
            ? mergeConversationActivityMessages(fetched, existing, pending)
            : mergeConversationActivityMessages(existing, fetched, pending),
          before: olderCursor,
          tail,
          loaded: true,
          cleared: start.cleared || reset,
          loading: false,
          loadingOlder: false,
          error: null,
        });
      } catch (error) {
        if (!controller.signal.aborted) commit({ ...current.current, loading: false, loadingOlder: false, error });
      } finally {
        if (request === controller) {
          request = null;
          duringRequest.clear();
          if (refreshPending) {
            refreshPending = false;
            void read();
          }
        }
      }
    }

    const unsubscribe = subscribeIMEvents((event) => {
      if (event.type === "connection.open") {
        void read();
        return;
      }
      if (event.room_id !== roomID) return;
      if (event.type === "room.messages_cleared") {
        request?.abort();
        request = null;
        refreshPending = false;
        live.clear();
        commit({ ...emptyHistory(roomID), loaded: true, cleared: true });
        void read();
        return;
      }
      const message = event.message;
      if (event.type !== "message.created" || !message?.id) return;
      const previous = live.get(message.id);
      live.set(message.id, previous ? newerProgressMessage(previous, message) : message);
      if (request) {
        const pending = duringRequest.get(message.id);
        duringRequest.set(message.id, pending ? newerProgressMessage(pending, message) : message);
      }
      timer ??= setTimeout(flush, liveBatchMs);
    });
    actions.current = { refresh: () => void read(), loadOlder: () => void read(true) };
    void read();
    return () => {
      request?.abort();
      request = null;
      clearTimeout(timer);
      unsubscribe();
      actions.current = null;
    };
  }, [active, roomID]);

  return {
    ...(history.roomID === roomID ? history : emptyHistory(roomID)),
    refresh: () => actions.current?.refresh(),
    loadOlder: () => actions.current?.loadOlder(),
  };
}
