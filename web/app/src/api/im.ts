import { del, get, post } from "@/api/client";
import type { IMConversation, IMMessage, IMUser, MessageRelation, ThreadView } from "@/models/conversations";

export type SendMessagePayload = {
  content: string;
  relates_to?: MessageRelation | null;
  room_id: string;
  sender_id: string;
};

export type StartThreadPayload = {
  root_message_id: string;
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

export function startThreadRequest(roomID: string, payload: StartThreadPayload): Promise<ThreadView> {
  return post(`api/v1/rooms/${encodeURIComponent(roomID)}/threads`, payload);
}

export function fetchThreadRequest(roomID: string, rootMessageID: string): Promise<ThreadView> {
  return get(`api/v1/rooms/${encodeURIComponent(roomID)}/threads/${encodeURIComponent(rootMessageID)}`);
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
