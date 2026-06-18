import { patch } from "@/api/client";
import type { IMUser } from "@/models/conversations";

export type ParticipantResponse = {
  avatar?: string | null;
  channel_user_ref?: string | null;
  id?: string | null;
  name?: string | null;
  type?: string | null;
};

export function patchParticipantAvatarRequest(participantID: string, avatar: string): Promise<ParticipantResponse> {
  return patch<ParticipantResponse>(`api/v1/channels/csgclaw/participants/${encodeURIComponent(participantID)}`, {
    avatar,
  });
}

export function patchCsgclawUserRequest(
  userID: string,
  payload: { description?: string },
): Promise<IMUser> {
  return patch<IMUser>(`api/v1/channels/csgclaw/users/${encodeURIComponent(userID)}`, payload);
}
