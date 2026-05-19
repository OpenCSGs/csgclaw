import { del, post } from "@/api/client";
import type { IMConversation, IMMessage, IMUser } from "@/models/conversations";

export type SendMessagePayload = {
  content: string;
  room_id: string;
  sender_id: string;
};

export type CreateRoomPayload = {
  creator_id: string;
  description?: string;
  locale?: string;
  member_ids: string[];
  title: string;
};

export type InviteRoomUsersPayload = {
  inviter_id: string;
  locale?: string;
  room_id: string;
  user_ids: string[];
};

export type JoinAgentToRoomPayload = {
  agent_id: string;
  inviter_id: string;
  locale?: string;
  room_id: string;
};

export type CreateUserPayload = Partial<IMUser> & {
  id: string;
  name: string;
};

export function sendMessageRequest(payload: SendMessagePayload): Promise<IMMessage> {
  return post("api/v1/messages", payload);
}

export function createRoomRequest(payload: CreateRoomPayload): Promise<IMConversation> {
  return post("api/v1/rooms", payload);
}

export function inviteRoomUsersRequest(payload: InviteRoomUsersPayload): Promise<IMConversation> {
  return post("api/v1/rooms/invite", payload);
}

export function deleteRoomRequest(roomID: string): Promise<void> {
  return del(`api/v1/rooms/${encodeURIComponent(roomID)}`);
}

export function joinAgentToRoomRequest(payload: JoinAgentToRoomPayload): Promise<IMConversation> {
  return post("api/v1/im/agents/join", payload);
}

export function createUserRequest(payload: CreateUserPayload): Promise<IMUser> {
  return post("api/v1/users", payload);
}
