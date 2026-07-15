import { del, get, post } from "@/api/client";
import type { IMConversation, IMMessage, IMUser, MessageRelation, ThreadView } from "@/models/conversations";

export type SendMessagePayload = {
  attachments?: File[];
  content: string;
  relates_to?: MessageRelation | null;
  room_id: string;
  sender_id: string;
};

export type FetchMessagesOptions = {
  includeThreadReplies?: boolean;
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

export type RemoveRoomUserPayload = {
  inviter_id: string;
  locale?: string;
  member_id: string;
  room_id: string;
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
  if (payload.attachments?.length) {
    const formData = new FormData();
    const { attachments, ...messagePayload } = payload;
    formData.set("payload", JSON.stringify(messagePayload));
    attachments.forEach((file) => {
      formData.append("files", file, file.name);
    });
    return post("api/v1/messages", undefined, { body: formData });
  }
  return post("api/v1/messages", payload);
}

export function fetchMessagesRequest(roomID: string, options: FetchMessagesOptions = {}): Promise<IMMessage[]> {
  const params = new URLSearchParams({ room_id: roomID });
  if (options.includeThreadReplies) {
    params.set("include_thread_replies", "true");
  }
  return get(`api/v1/messages?${params.toString()}`);
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

export function removeRoomUserRequest(payload: RemoveRoomUserPayload): Promise<IMConversation> {
  return del(`api/v1/rooms/${encodeURIComponent(payload.room_id)}/members/${encodeURIComponent(payload.member_id)}`, {
    json: { inviter_id: payload.inviter_id, locale: payload.locale },
  });
}

export function deleteRoomRequest(roomID: string): Promise<void> {
  return del(`api/v1/rooms/${encodeURIComponent(roomID)}`);
}

export function clearRoomMessagesRequest(roomID: string): Promise<IMConversation> {
  return post(`api/v1/rooms/${encodeURIComponent(roomID)}:clearMessages`, {});
}

export function joinAgentToRoomRequest(payload: JoinAgentToRoomPayload): Promise<IMConversation> {
  return inviteRoomUsersRequest({
    room_id: payload.room_id,
    inviter_id: payload.inviter_id,
    user_ids: [payload.agent_id],
    locale: payload.locale,
  });
}

export function createUserRequest(payload: CreateUserPayload): Promise<IMUser> {
  return post("api/v1/channels/csgclaw/users", payload);
}
