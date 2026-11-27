import {
  localIdentitiesMatch,
  resolveUserByLocalIdentity,
  userIDForLocalIdentity,
  type IMConversation,
  type TranslateFn,
  type UsersById,
} from "./conversations";

export const collaborationConversationOwner = "collaboration";

export type ConversationFilters = { search: string; owner: string; directory: string };
export const emptyConversationFilters: ConversationFilters = { search: "", owner: "", directory: "" };

type ConversationFilterRoom = Pick<IMConversation, "members" | "is_direct" | "configuration"> & {
  cloud_agent?: Pick<NonNullable<IMConversation["cloud_agent"]>, "user_id">;
  count?: number;
};

export function conversationPeerID(room: ConversationFilterRoom, currentUserID: string): string {
  return room.is_direct ? (room.members.find((id) => id && !localIdentitiesMatch(id, currentUserID)) ?? "") : "";
}

export function conversationOwner(room: ConversationFilterRoom, currentUserID: string): string {
  if (room.cloud_agent) return room.cloud_agent.user_id ? `cloud:${room.cloud_agent.user_id}` : "";
  if (!room.is_direct) return collaborationConversationOwner;
  const peer = userIDForLocalIdentity(conversationPeerID(room, currentUserID));
  return peer ? `direct:${peer}` : "";
}

export function conversationDirectory(
  room: ConversationFilterRoom,
  currentUserID: string,
  directoriesByPeer: ReadonlyMap<string, string>,
): string {
  if (room.configuration) return room.configuration.workspace_dir || "";
  if (room.cloud_agent) return "";
  return directoriesByPeer.get(userIDForLocalIdentity(conversationPeerID(room, currentUserID))) || "";
}

export function conversationFilterOptions(
  rooms: readonly ConversationFilterRoom[],
  filters: Pick<ConversationFilters, "owner" | "directory">,
  currentUserID: string,
  usersById: UsersById,
  directoriesByPeer: ReadonlyMap<string, string>,
  t: TranslateFn,
) {
  const owners = new Map<string, { value: string; label: string; avatar?: string | null; count: number }>();
  const directories = new Map<string, number>();
  let ownerTotal = 0;
  let directoryTotal = 0;
  for (const room of rooms) {
    const count = room.count ?? 1;
    const value = conversationOwner(room, currentUserID);
    const path = conversationDirectory(room, currentUserID, directoriesByPeer);
    const matchesDirectory = !filters.directory || filters.directory === path;
    const matchesOwner = !filters.owner || filters.owner === value;
    if (matchesDirectory) ownerTotal += count;
    if (matchesOwner) directoryTotal += count;
    if (value && (matchesDirectory || value === filters.owner)) {
      const existing = owners.get(value);
      if (existing) {
        existing.count += matchesDirectory ? count : 0;
      } else {
        const peer = room.cloud_agent?.user_id || conversationPeerID(room, currentUserID);
        const user = room.cloud_agent ? usersById.get(peer) : resolveUserByLocalIdentity(peer, usersById);
        owners.set(value, {
          value,
          label: value === collaborationConversationOwner ? t("iaRoomsTab") : user?.name || peer,
          avatar: user?.avatar,
          count: matchesDirectory ? count : 0,
        });
      }
    }
    if (path) directories.set(path, (directories.get(path) ?? 0) + (matchesOwner ? count : 0));
  }
  return {
    ownerTotal,
    directoryTotal,
    owners: [...owners.values()].sort((a, b) => b.count - a.count || a.label.localeCompare(b.label)),
    directories: [...directories]
      .map(([path, count]) => ({ path, count }))
      .sort((a, b) => a.path.localeCompare(b.path)),
  };
}
