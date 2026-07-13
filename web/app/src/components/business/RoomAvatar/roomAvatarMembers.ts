import { localIdentitiesMatch, resolveUserByLocalIdentity } from "@/models/conversations";
import type { IMConversation, IMUser, UsersById } from "@/models/conversations";

export type RoomAvatarMember = Pick<IMUser, "accent_hex" | "avatar" | "id" | "name">;

export function resolveRoomAvatarMembers(
  conversation: IMConversation | null | undefined,
  usersById: UsersById,
  currentUserID?: string | null,
): RoomAvatarMember[] {
  return (conversation?.members || [])
    .filter((memberID) => !localIdentitiesMatch(memberID, currentUserID))
    .map((memberID) => resolveUserByLocalIdentity(memberID, usersById))
    .filter((member): member is IMUser => Boolean(member))
    .map((member) => ({
      id: member.id,
      name: member.name || member.id,
      avatar: member.avatar,
      accent_hex: member.accent_hex,
    }));
}
