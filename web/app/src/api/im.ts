import { del, get, patch, post, resolveRequestPath, type ApiError } from "@/api/client";
import type {
  IMConversation,
  IMMessage,
  IMUser,
  MessageRelation,
  ParticipantWorkUpdate,
  ThreadView,
} from "@/models/conversations";

export type SendMessagePayload = {
  attachments?: File[];
  content: string;
  locale?: string;
  relates_to?: MessageRelation | null;
  room_id: string;
  sender_id: string;
};

export type SendMessageRequestOptions = {
  onUploadProgress?: (progress: number) => void;
  signal?: AbortSignal;
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

export type UpdateRoomPayload = {
  notify_all_agents: boolean;
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

export type StopParticipantWorkPayload = {
  lease_id: string;
  request_id: string;
  room_id: string;
};

export type StopParticipantWorkResponse = {
  accepted: true;
  lease_id: string;
  participant_id: string;
  registry_epoch: string;
  request_id: string;
  requested_at: string;
  room_id: string;
  state: "stop_requested";
};

export type CreateUserPayload = Partial<IMUser> & {
  id: string;
  name: string;
};

export function sendMessageRequest(
  payload: SendMessagePayload,
  options: SendMessageRequestOptions = {},
): Promise<IMMessage> {
  if (payload.attachments?.length) {
    const formData = new FormData();
    const { attachments, ...messagePayload } = payload;
    formData.set("payload", JSON.stringify(messagePayload));
    attachments.forEach((file) => {
      formData.append("files", file, file.name);
    });
    if (options.onUploadProgress) {
      return postMessageFormDataWithProgress(formData, options);
    }
    return post("api/v1/messages", undefined, { body: formData, signal: options.signal });
  }
  return post("api/v1/messages", payload, { signal: options.signal });
}

function postMessageFormDataWithProgress(formData: FormData, options: SendMessageRequestOptions): Promise<IMMessage> {
  return new Promise((resolve, reject) => {
    if (options.signal?.aborted) {
      reject(new DOMException("The request was aborted.", "AbortError"));
      return;
    }

    const request = new XMLHttpRequest();
    const abortRequest = () => request.abort();
    request.open("POST", resolveRequestPath("api/v1/messages"));
    request.setRequestHeader("Accept", "application/json");
    request.upload.addEventListener("progress", (event) => {
      if (!event.lengthComputable || event.total <= 0) {
        return;
      }
      options.onUploadProgress?.(Math.min(100, Math.round((event.loaded / event.total) * 100)));
    });
    request.addEventListener("load", () => {
      options.signal?.removeEventListener("abort", abortRequest);
      if (request.status < 200 || request.status >= 300) {
        reject({
          status: request.status,
          message: request.responseText.trim() || request.statusText,
        } satisfies ApiError);
        return;
      }
      try {
        resolve(JSON.parse(request.responseText) as IMMessage);
      } catch {
        reject({ status: request.status, message: "Invalid message response." } satisfies ApiError);
      }
    });
    request.addEventListener("error", () => {
      options.signal?.removeEventListener("abort", abortRequest);
      reject({ status: request.status, message: request.statusText || "Network request failed." } satisfies ApiError);
    });
    request.addEventListener("abort", () => {
      options.signal?.removeEventListener("abort", abortRequest);
      reject(new DOMException("The request was aborted.", "AbortError"));
    });
    options.signal?.addEventListener("abort", abortRequest, { once: true });
    options.onUploadProgress?.(0);
    request.send(formData);
  });
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

export function updateRoomRequest(roomID: string, payload: UpdateRoomPayload): Promise<IMConversation> {
  return patch(`api/v1/rooms/${encodeURIComponent(roomID)}`, payload);
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

export function stopParticipantWorkRequest(
  participantID: ParticipantWorkUpdate["participant_id"],
  payload: StopParticipantWorkPayload,
): Promise<StopParticipantWorkResponse> {
  return post(`api/v1/channels/csgclaw/participants/${encodeURIComponent(participantID)}/work:stop`, payload);
}
