import { useEffect, useState, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createRoomRequest, deleteRoomRequest } from "@/api/im";
import type { CreateRoomPayload } from "@/api/im";
import { updateConversationMetadata, type ConversationMetadataPatch } from "@/api/taskConversations";
import type { IMConversation, IMMessage } from "@/models/conversations";

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
  useEffect(() => {
    if (refreshTimer.current !== null) return;
    refreshTimer.current = window.setTimeout(() => {
      refreshTimer.current = null;
      void client.invalidateQueries({ queryKey: ["task-conversations"] });
    }, 500);
  }, [client, rooms]);
  useEffect(
    () => () => {
      if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current);
    },
    [],
  );

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
      await deleteRoomRequest(id);
      onDelete(id);
      await client.invalidateQueries({ queryKey: ["task-conversations"] });
    } finally {
      setBusyID("");
    }
  }
  return { create, patch, remove, onMessage, open: onOpen, isWorking, busyID };
}

export type TaskEntryController = ReturnType<typeof useTaskEntry>;
