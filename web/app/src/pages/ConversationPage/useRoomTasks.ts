import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchRoomTasks, fetchRoomTaskEvents } from "@/api/tasks";
import { isOnDemandConversation, type IMConversation } from "@/models/conversations";

export function useRoomTasks(conversation: IMConversation) {
  const client = useQueryClient();
  const enabled = isOnDemandConversation(conversation) && !conversation.is_direct;
  const revision = (conversation.messages ?? [])
    .filter((message) => message.metadata?.task_id)
    .map((message) => message.id)
    .join("|");
  const previous = useRef({ roomID: conversation.id, revision });
  useEffect(() => {
    const changed = previous.current.roomID === conversation.id && previous.current.revision !== revision;
    previous.current = { roomID: conversation.id, revision };
    if (enabled && changed) {
      void client.invalidateQueries({ queryKey: ["workspace", "room-tasks", conversation.id] });
    }
  }, [client, conversation.id, enabled, revision]);
  return useQuery({
    queryKey: ["workspace", "room-tasks", conversation.id],
    queryFn: ({ signal }) => fetchRoomTasks(conversation.id, signal),
    enabled,
    // Poll while the room is visible, even before a root exists. Task persistence
    // and IM projection can succeed independently; messages are not the task store.
    refetchInterval: 3000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  });
}

export function useRoomTaskEvents(roomID: string, taskID: string) {
  return useQuery({
    queryKey: ["workspace", "room-task-events", roomID, taskID],
    queryFn: ({ signal }) => fetchRoomTaskEvents(roomID, taskID, signal),
    enabled: Boolean(taskID),
    refetchInterval: 3000,
    refetchIntervalInBackground: false,
  });
}
