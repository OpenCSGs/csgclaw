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

export function conversationPeerID(room: IMConversation, currentUserID: string): string {
  return room.is_direct ? (room.members.find((id) => id && !localIdentitiesMatch(id, currentUserID)) ?? "") : "";
}

export function conversationOwner(room: IMConversation, currentUserID: string): string {
  if (room.cloud_agent) return room.cloud_agent.user_id ? `cloud:${room.cloud_agent.user_id}` : "";
  if (!room.is_direct) return collaborationConversationOwner;
  const peer = userIDForLocalIdentity(conversationPeerID(room, currentUserID));
  return peer ? `direct:${peer}` : "";
}

export function conversationDirectory(
  room: IMConversation,
  currentUserID: string,
  directoriesByPeer: ReadonlyMap<string, string>,
): string {
  if (room.configuration) return room.configuration.workspace_dir || "";
  if (room.cloud_agent) return "";
  return directoriesByPeer.get(userIDForLocalIdentity(conversationPeerID(room, currentUserID))) || "";
}

export function conversationFilterOptions(
  rooms: readonly IMConversation[],
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
    const value = conversationOwner(room, currentUserID);
    const path = conversationDirectory(room, currentUserID, directoriesByPeer);
    const matchesDirectory = !filters.directory || filters.directory === path;
    const matchesOwner = !filters.owner || filters.owner === value;
    if (matchesDirectory) ownerTotal += 1;
    if (matchesOwner) directoryTotal += 1;
    if (value && (matchesDirectory || value === filters.owner)) {
      const existing = owners.get(value);
      if (existing) {
        existing.count += Number(matchesDirectory);
      } else {
        const peer = room.cloud_agent?.user_id || conversationPeerID(room, currentUserID);
        const user = room.cloud_agent ? usersById.get(peer) : resolveUserByLocalIdentity(peer, usersById);
        owners.set(value, {
          value,
          label: value === collaborationConversationOwner ? t("iaRoomsTab") : user?.name || peer,
          avatar: user?.avatar,
          count: Number(matchesDirectory),
        });
      }
    }
    if (path) directories.set(path, (directories.get(path) ?? 0) + Number(matchesOwner));
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
