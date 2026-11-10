import { useCallback, useMemo, useSyncExternalStore } from "react";
import {
  closeChatSideTab,
  replaceChatSideTab,
  openChatSideTab,
  restoreChatSideState,
  serializeChatSideState,
  type ChatSideState,
  type ChatSideTab,
} from "@/models/chatSidePanel";
import { CHAT_SIDE_PANEL_STORAGE_KEY } from "@/shared/storage/keys";

const snapshots = new Map<string, ChatSideState>();
const listeners = new Map<string, Set<() => void>>();

export function chatSidePanelKey(userID: string, roomID: string): string {
  return `${CHAT_SIDE_PANEL_STORAGE_KEY}:${window.location.origin}${window.location.pathname}:${encodeURIComponent(userID)}:${encodeURIComponent(roomID)}`;
}

function read(key: string): ChatSideState {
  let snapshot = snapshots.get(key);
  if (!snapshot) {
    try {
      snapshot = restoreChatSideState(localStorage.getItem(key));
    } catch {
      snapshot = restoreChatSideState(null);
    }
    snapshots.set(key, snapshot);
  }
  return snapshot;
}

export function useChatSidePanel(userID: string, roomID: string) {
  const key = chatSidePanelKey(userID, roomID);
  const subscribe = useCallback(
    (listener: () => void) => {
      const group = listeners.get(key) ?? new Set();
      group.add(listener);
      listeners.set(key, group);
      return () => {
        group.delete(listener);
        if (!group.size) {
          snapshots.delete(key);
          listeners.delete(key);
        }
      };
    },
    [key],
  );
  const getSnapshot = useCallback(() => read(key), [key]);
  const state = useSyncExternalStore(subscribe, getSnapshot);
  const actions = useMemo(() => {
    const update = (updater: (current: ChatSideState) => ChatSideState) => {
      const next = updater(read(key));
      snapshots.set(key, next);
      try {
        localStorage.setItem(key, serializeChatSideState(next));
      } catch {
        /* In-memory tabs work without storage. */
      }
      listeners.get(key)?.forEach((listener) => listener());
    };
    return {
      openTab: (tab: ChatSideTab) => update((current) => openChatSideTab(current, tab)),
      closeTab: (id: string) => update((current) => closeChatSideTab(current, id)),
      patch: (patch: Partial<ChatSideState>) => update((current) => ({ ...current, ...patch })),
      replaceTab: (tab: ChatSideTab, previousID = tab.id) =>
        update((current) => replaceChatSideTab(current, previousID, tab)),
    };
  }, [key]);
  return { state, ...actions };
}

export type ChatSidePanelController = ReturnType<typeof useChatSidePanel>;
