import { useEffect, useRef, useState, type RefObject } from "react";
import { fetchMessagePageRequest } from "@/api/im";
import type { IMMessage } from "@/models/conversations";
import { newerProgressMessage, parseTurnProgress, progressActive } from "@/models/turnProgress";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import { mergeConversationActivityMessages } from "./conversationActivity";

export const activityHistoryPageSize = 100;
const maxRefreshIDs = 100;
const liveBatchMs = 80;

type History = {
  roomID: string;
  messages: IMMessage[];
  sources: IMMessage[];
  before: string;
  latest: IMMessage[];
  loaded: boolean;
  loading: boolean;
  loadingOlder: boolean;
  error: unknown;
};

function emptyHistory(roomID: string): History {
  return {
    roomID,
    messages: [],
    sources: [],
    before: "",
    latest: [],
    loaded: false,
    loading: false,
    loadingOlder: false,
    error: null,
  };
}

function refreshMessageIDs(targetIDs: string[], inspected: readonly IMMessage[], messages: IMMessage[]): string[] {
  const inspectedID = inspected.at(-1)?.id;
  const ids = new Set(inspectedID ? [inspectedID, ...targetIDs] : targetIDs);
  for (let index = messages.length - 1; index >= 0 && ids.size < maxRefreshIDs; index--) {
    const message = messages[index];
    if (!message.id) continue;
    const progress = parseTurnProgress(message);
    if (progress && progressActive(progress)) ids.add(message.id);
  }
  for (const message of inspected) {
    if (ids.size >= maxRefreshIDs) break;
    if (message.id) ids.add(message.id);
  }
  return [...ids].slice(0, maxRefreshIDs);
}

export function useActivityHistory(
  roomID: string,
  active: boolean,
  targetKey: string,
  inspected?: RefObject<readonly IMMessage[] | undefined>,
) {
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
      flush();
      const start = current.current;
      if (older && !start.before) return;
      const controller = new AbortController();
      request = controller;
      duringRequest.clear();
      commit({ ...start, loading: true, loadingOlder: older, error: null });
      const options = { limit: activityHistoryPageSize, signal: controller.signal };

      try {
        let before = older ? start.before : undefined;
        const inspecting = start.messages.length ? (inspected?.current ?? []) : [];
        let requestedIDs: string[] = [];
        const loadLatest = () => {
          requestedIDs = refreshMessageIDs(JSON.parse(targetKey) as string[], inspecting, start.messages);
          return fetchMessagePageRequest(roomID, { ...options, refreshIDs: requestedIDs });
        };
        let page = await (before ? fetchMessagePageRequest(roomID, { ...options, before }) : loadLatest());
        const missingCursor = page.cursor_missing;
        if (missingCursor) {
          before = undefined;
          page = await loadLatest();
        }
        if (controller.signal.aborted) return;
        const first = page.messages[0];
        // Final replies can move to the end of the transcript. Keep earlier
        // pages only when the new window starts on an unchanged HTTP record.
        const keepEarlier =
          !missingCursor &&
          (Boolean(before) ||
            (page.has_more &&
              first?.id &&
              start.latest.some((message) => message.id === first.id && message.created_at === first.created_at)));
        const fetched = [...page.messages, ...(page.refreshed ?? [])];
        let existing = keepEarlier ? current.current.messages : [];
        if (!keepEarlier && inspecting.length > 0) {
          const fetchedIDs = new Set(fetched.map((message) => message.id));
          if (inspecting.some((message) => message.id && fetchedIDs.has(message.id))) {
            // Long legacy turns can span more records than one refresh request.
            // Keep their loaded remainder after the server confirms the turn still exists.
            const checked = new Set(requestedIDs);
            existing = inspecting.filter((message) => !message.id || !checked.has(message.id));
          }
        }
        const pending = [...duringRequest.values()];
        clearTimeout(timer);
        timer = undefined;
        live.clear();
        commit({
          roomID,
          messages: before
            ? mergeConversationActivityMessages(fetched, existing, pending)
            : mergeConversationActivityMessages(existing, fetched, pending),
          sources: mergeConversationActivityMessages(keepEarlier ? start.sources : [], page.sources),
          before: !before && keepEarlier ? start.before : page.has_more ? first?.id || "" : "",
          latest: before ? start.latest : page.messages,
          loaded: true,
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
        commit({ ...emptyHistory(roomID), loaded: true });
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
  }, [active, roomID, targetKey, inspected]);

  return {
    ...(history.roomID === roomID ? history : emptyHistory(roomID)),
    refresh: () => actions.current?.refresh(),
    loadOlder: () => actions.current?.loadOlder(),
  };
}
