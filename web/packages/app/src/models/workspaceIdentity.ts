import { agentMatchesUser, isDirectConversation, resolveConversationUser } from "./conversations";
import { resolveAgentChannelUserID } from "./agents";
import { MANAGER_AGENT_ID, MANAGER_PARTICIPANT_ID } from "@/shared/constants/agents";
import type { AgentLike } from "./agents";
import type { IMConversation, IMData, IMUser } from "./conversations";

function isBootstrapAdminUser(user: IMUser | null | undefined) {
  return user?.id === "u-admin" || String(user?.name ?? "").toLowerCase() === "admin";
}

export function initialsForIdentity(name: string) {
  const trimmed = name.trim();
  if (!trimmed) {
    return "ME";
  }
  const parts = trimmed.split(/\s+/).filter(Boolean);
  if (parts.length > 1) {
    return `${parts[0]?.[0] ?? ""}${parts[1]?.[0] ?? ""}`.toUpperCase();
  }
  return trimmed.slice(0, 2).toUpperCase();
}

export function resolveManagerDirectConversation(
  rooms: readonly IMConversation[],
  currentUserID: string,
  managerAgent: AgentLike | null | undefined,
): IMConversation | null {
  if (!currentUserID) {
    return null;
  }
  const managerUserIDs = new Set(
    [
      resolveAgentChannelUserID(managerAgent),
      managerAgent?.id,
      managerAgent?.user_id,
      MANAGER_PARTICIPANT_ID,
      MANAGER_AGENT_ID,
    ]
      .map((value) => String(value ?? "").trim())
      .filter(Boolean),
  );
  return (
    rooms.find(
      (room) =>
        isDirectConversation(room) &&
        room.members.includes(currentUserID) &&
        room.members.some((memberID) => memberID !== currentUserID && managerUserIDs.has(memberID)),
    ) ?? null
  );
}

export function resolveDirectConversationAgent(
  conversation: IMConversation | null | undefined,
  currentUserID: string,
  usersById: Map<string, IMUser>,
  agents: readonly AgentLike[],
): AgentLike | null {
  if (!conversation || !currentUserID || !isDirectConversation(conversation)) {
    return null;
  }
  const otherMemberID = conversation.members.find((id) => id && id !== currentUserID) ?? "";
  const directUser = resolveConversationUser(conversation, currentUserID, usersById);
  return (
    agents.find((item) => {
      const channelUserID = resolveAgentChannelUserID(item);
      return (
        Boolean(otherMemberID && (item.id === otherMemberID || item.user_id === otherMemberID)) ||
        Boolean(channelUserID && channelUserID === otherMemberID) ||
        agentMatchesUser(item, directUser)
      );
    }) ?? null
  );
}

export function withLocalIdentity(data: IMData | null, fallbackName: string): IMData | null {
  if (!data?.current_user_id) {
    return data;
  }

  const currentUser = data.users.find((user) => user.id === data.current_user_id);
  if (!currentUser) {
    return data;
  }

  const displayName = isBootstrapAdminUser(currentUser) ? fallbackName : currentUser.name;
  if (!displayName || displayName === currentUser.name) {
    return data;
  }

  return {
    ...data,
    users: data.users.map((user) =>
      user.id === data.current_user_id
        ? {
            ...user,
            avatar: user.avatar || initialsForIdentity(displayName),
            name: displayName,
          }
        : user,
    ),
  };
}
