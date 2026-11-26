import { useCallback, useEffect, useMemo, useState, useRef } from "react";
import { useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { apiErrorCode } from "@/api/client";
import { createRoomRequest, deleteRoomRequest, fetchMessagesRequest } from "@/api/im";
import type { CreateRoomPayload } from "@/api/im";
import {
  updateConversationMetadata,
  type ConversationMetadataPatch,
  type TaskConversationPage,
} from "@/api/taskConversations";
import { parseTurnProgress, progressActive } from "@/models/turnProgress";
import { taskListChange, type RefreshScope } from "@/models/taskConversations";
import { isThreadReply, type IMConversation, type IMMessage, type IMServerEvent } from "@/models/conversations";

type Args = {
  currentUserID: string;
  rooms: readonly IMConversation[];
  onUpdate: (room: IMConversation) => void;
  onMessage: (roomID: string, message: IMMessage) => void;
  onDelete: (id: string) => void;
  onOpen: (room: IMConversation) => void;
  isWorking: (id: string) => boolean;
};

export function useTaskEntry({ currentUserID, rooms, onUpdate, onMessage, onDelete, onOpen, isWorking }: Args) {
  const client = useQueryClient();
  const [busyID, setBusyID] = useState("");
  const refreshTimer = useRef<number | null>(null);
  const previousRooms = useRef<readonly IMConversation[] | null>(null);
  const pending = useRef<RefreshScope | null>(null);
  const liveRooms = useMemo(() => new Map(rooms.map((room) => [room.id, room])), [rooms]);
  const scheduleRefresh = useCallback(
    (scope: RefreshScope) => {
      if (pending.current !== "all") pending.current = scope;
      if (refreshTimer.current !== null) return;
      function refresh() {
        const searchOnly = pending.current === "search";
        pending.current = null;
        void client
          .invalidateQueries(
            {
              queryKey: ["task-conversations"],
              predicate: searchOnly ? (query) => Boolean(query.queryKey[2]) : undefined,
            },
            { cancelRefetch: false },
          )
          .then(() => {
            if (refreshTimer.current === null) return;
            refreshTimer.current = pending.current ? window.setTimeout(refresh, 500) : null;
          });
      }
      refreshTimer.current = window.setTimeout(refresh, 500);
    },
    [client],
  );
  useEffect(() => {
    const scope = taskListChange(previousRooms.current, rooms);
    previousRooms.current = rooms;
    if (scope) scheduleRefresh(scope);
  }, [rooms, scheduleRefresh]);

  useEffect(
    () => () => {
      if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current);
      refreshTimer.current = null;
      previousRooms.current = null;
      pending.current = null;
    },
    [],
  );

  function onHistoryEvent(event: IMServerEvent) {
    if (event.type === "connection.open" || event.type === "room.deleted") scheduleRefresh("all");
    // Thread replies are absent from bootstrap.rooms' main transcript.
    if (event.type !== "message.created" || !isThreadReply(event.message)) return;
    const progress = parseTurnProgress(event.message);
    if (!progress || progress.revision <= 1 || !progressActive(progress)) scheduleRefresh("all");
  }

  async function create(payload: Omit<CreateRoomPayload, "creator_id">) {
    const room = await createRoomRequest({ ...payload, creator_id: currentUserID, task_session: true });
    onUpdate(room);
    return room;
  }
  async function patch(id: string, value: ConversationMetadataPatch) {
    setBusyID(id);
    try {
      const room = await updateConversationMetadata(id, value);
      onUpdate(room);
      await client.invalidateQueries({ queryKey: ["task-conversations"] });
    } finally {
      setBusyID("");
    }
  }
  async function remove(id: string) {
    setBusyID(id);
    try {
      try {
        await deleteRoomRequest(id);
      } catch (error) {
        if (apiErrorCode(error) !== "room_not_found") {
          const status = error && typeof error === "object" && "status" in error ? error.status : undefined;
          const ambiguousGatewayResponse = status === 502 || status === 503 || status === 504;
          if (!(error instanceof TypeError) && !ambiguousGatewayResponse) throw error;
          const roomMissing = await fetchMessagesRequest(id).then(
            () => false,
            (cause: unknown) => apiErrorCode(cause) === "room_not_found",
          );
          if (!roomMissing) throw error;
        }
      }

      const historyQueries = { queryKey: ["task-conversations"] };
      await client.cancelQueries(historyQueries);
      client.setQueriesData<InfiniteData<TaskConversationPage>>(
        { ...historyQueries, predicate: (query) => query.queryKey.length === 3 },
        (data) => {
          if (!data?.pages.some((page) => page.rooms.some((room) => room.id === id))) return data;
          return {
            ...data,
            pages: data.pages.map((page) => ({
              ...page,
              rooms: page.rooms.filter((room) => room.id !== id),
              total: Math.max(0, page.total - 1),
            })),
          };
        },
      );
      onDelete(id);
      void client.invalidateQueries(historyQueries);
    } finally {
      setBusyID("");
    }
  }
  return { create, patch, remove, liveRooms, onHistoryEvent, onMessage, open: onOpen, isWorking, busyID };
}

export type TaskEntryController = Omit<ReturnType<typeof useTaskEntry>, "onHistoryEvent">;
