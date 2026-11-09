import type { IMConversation } from "./conversations";

// Mirror the persisted job/reply identities used by the cloud task worker.
export function hasPendingCloudTask(room: IMConversation | null | undefined): boolean {
  if (!room?.cloud_agent) return false;
  const senderID = room.cloud_agent.user_id;
  const creatorID = room.created_by || room.members.find((id) => id !== senderID);
  const replied = new Set(
    room.messages.filter((message) => message.sender_id === senderID).map((message) => message.client_message_id),
  );
  return room.messages.some(
    (message) =>
      message.id &&
      message.kind !== "event" &&
      message.sender_id === creatorID &&
      !replied.has(`relay-reply:${message.id}`),
  );
}
