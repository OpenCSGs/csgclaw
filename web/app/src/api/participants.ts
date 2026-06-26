import { patch } from "@/api/client";
import type { IMUser } from "@/models/conversations";

export type ParticipantResponse = {
  channel_user_ref?: string | null;
  id?: string | null;
  name?: string | null;
  type?: string | null;
};

export function patchCsgclawUserRequest(
  userID: string,
  payload: { avatar?: string; description?: string },
): Promise<IMUser> {
  return patch<IMUser>(`api/v1/channels/csgclaw/users/${encodeURIComponent(userID)}`, payload);
}
