import { flattenMentionText } from "@/components/business/MessageContent/mentions";

export type LocaleCode = "zh" | "en" | string;
export type TranslateFn = (key: string, params?: Record<string, string | number>) => string;

export type IMUser = {
  accent_hex?: string | null;
  avatar?: string | null;
  handle?: string | null;
  id: string;
  name?: string | null;
  role?: string | null;
  user_id?: string | null;
};

export type MessageMention = string | { id?: string | null };

export type IMMessageEvent = {
  actor_id?: string | null;
  key?: string | null;
  target_ids?: string[] | null;
  title?: string | null;
};

export type IMMessage = {
  id?: string;
  content: string;
  created_at?: string;
  event?: IMMessageEvent | null;
  kind?: string | null;
  mentions?: MessageMention[] | null;
  sender_id?: string | null;
};

export type IMConversation = {
  description?: string | null;
  id: string;
  is_direct?: boolean | null;
  members: string[];
  messages: IMMessage[];
  title?: string | null;
};

export type IMData = {
  current_user_id?: string;
  rooms: IMConversation[];
  users: IMUser[];
};

export type IMServerEvent = {
  message?: IMMessage | null;
  room?: Partial<IMConversation> | null;
  room_id?: string | null;
  type?: string | null;
  upgrade?: unknown;
  user?: IMUser | null;
};

export type UsersById = Map<string, IMUser>;

export function isToolCallMessage(content: unknown): boolean {
  return String(content ?? "")
    .trimStart()
    .startsWith("🔧 ");
}

export function isEventMessage(message: IMMessage | null | undefined): boolean {
  if (message?.kind === "event") {
    return true;
  }
  return isLegacySystemEventContent(message?.content);
}

export function formatConversationPreview(
  message: IMMessage | null | undefined,
  conversation: IMConversation,
  currentUserID: string,
  usersById: UsersById,
  locale: LocaleCode,
  t: TranslateFn,
): string {
  if (message) {
    if (isEventMessage(message)) {
      return formatEventMessage(message, usersById, locale);
    }
    return flattenMentionText(message.content);
  }
  return getConversationSubtitle(conversation, currentUserID, usersById, locale, t);
}

export function formatEventMessage(
  message: IMMessage | null | undefined,
  usersById: UsersById,
  locale: LocaleCode,
): string {
  if (!message) {
    return "";
  }
  if (message.event?.key === "room_created") {
    const actor = userDisplayName(message.event.actor_id || message.sender_id, usersById);
    const title = message.event.title || message.content || "";
    return locale === "zh" ? `${actor} 创建了房间“${title}”` : `${actor} created the room "${title}"`;
  }
  if (message.event?.key === "room_members_added") {
    const actor = userDisplayName(message.event.actor_id || message.sender_id, usersById);
    const targets = (message.event.target_ids || mentionIDs(message.mentions) || [])
      .map((id) => userDisplayName(id, usersById))
      .filter(Boolean);
    if (targets.length > 0) {
      return locale === "zh"
        ? `${actor} 邀请 ${targets.join("、")} 加入了房间`
        : `${actor} invited ${targets.join(", ")} to join the room`;
    }
  }
  return message.content || "";
}

export function mentionIDs(mentions: readonly MessageMention[] | null | undefined): string[] {
  return (mentions || [])
    .map((mention) => {
      if (typeof mention === "string") {
        return mention;
      }
      return mention?.id || "";
    })
    .filter(Boolean);
}

export function isLegacySystemEventContent(content: unknown): boolean {
  const text = String(content ?? "").trim();
  if (!text) {
    return false;
  }
  return [
    /^.+ invited .+ to join the room\.?$/,
    /^.+ invited .+ to join the channel\.?$/,
    /^.+ created the room ".+"\.?$/,
    /^.+ created the channel ".+"\.?$/,
    /^.+ 邀请 .+ 加入了房间。?$/,
    /^.+ 邀请 .+ 加入了频道。?$/,
    /^.+ 创建了房间“.+”。?$/,
    /^.+ 创建了频道“.+”。?$/,
  ].some((pattern) => pattern.test(text));
}

export function userDisplayName(userID: string | null | undefined, usersById: UsersById): string {
  if (!userID) {
    return "";
  }
  const user = usersById.get(userID);
  if (!user) {
    return userID;
  }
  return user.name || (user.handle ? `@${user.handle}` : userID);
}

export function resolveConversationUser(
  conversation: IMConversation,
  currentUserID: string,
  usersById: UsersById,
): IMUser | undefined {
  const otherID = conversation.members.find((id) => id !== currentUserID) ?? currentUserID;
  return usersById.get(otherID);
}

export function agentMatchesUser(
  agent: { handle?: string | null; id?: string | null; name?: string | null; user_id?: string | null } | null,
  user: { handle?: string | null; id?: string | null; name?: string | null } | null | undefined,
): boolean {
  if (!agent || !user) {
    return false;
  }
  const agentHandle = normalizeComparable(agent.handle);
  const userHandle = normalizeComparable(user.handle);
  const agentName = normalizeComparable(agent.name);
  const userName = normalizeComparable(user.name);
  return (
    agent.id === user.id ||
    agent.user_id === user.id ||
    Boolean(agentHandle && userHandle && agentHandle === userHandle) ||
    Boolean(agentName && userName && agentName === userName)
  );
}

export function normalizeComparable(value: unknown): string {
  return String(value || "")
    .trim()
    .toLowerCase();
}

export function isDirectConversation(conversation: { is_direct?: boolean | null } | null | undefined): boolean {
  return Boolean(conversation?.is_direct);
}

export function getConversationSubtitle(
  conversation: IMConversation,
  currentUserID: string,
  usersById: UsersById,
  locale: LocaleCode,
  t: TranslateFn,
): string {
  void conversation;
  void currentUserID;
  void usersById;
  void locale;
  void t;
  return "";
}

export function getConversationDescription(
  conversation: IMConversation,
  currentUserID: string,
  usersById: UsersById,
  locale: LocaleCode,
  t: TranslateFn,
): string {
  void currentUserID;
  void usersById;
  void locale;
  void t;
  if (isDirectConversation(conversation)) {
    return "";
  }
  return conversation.description || "";
}

export function formatTime(value: string | number | Date | null | undefined, locale: LocaleCode): string {
  if (!value) return "";
  return new Date(value).toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: locale === "zh" ? "Asia/Shanghai" : "UTC",
  });
}

export function latestAt(conversation: { messages?: readonly IMMessage[] | null }): number {
  const messages = conversation.messages ?? [];
  if (!messages.length) return 0;
  return new Date(messages[messages.length - 1].created_at || "").getTime();
}

export function applyIMEvent<T extends IMData | null | undefined>(
  current: T,
  event: IMServerEvent | null | undefined,
): T | IMData {
  if (!current || !event?.type) {
    return current;
  }

  if (event.type === "user.created" && event.user) {
    return upsertUserInData(current, event.user);
  }
  if (event.type === "user.deleted" && event.user) {
    return removeUserFromData(current, event.user.id);
  }
  if (event.type === "message.created" && event.message) {
    return appendMessageToData(current, event.room_id, event.message);
  }
  if (
    (event.type === "conversation.created" ||
      event.type === "conversation.members_added" ||
      event.type === "room.created" ||
      event.type === "room.members_added") &&
    event.room?.id
  ) {
    return upsertConversationInData(current, event.room as IMConversation);
  }
  return current;
}

export function isAgentRosterEvent(event: IMServerEvent | null | undefined): boolean {
  if (!event?.type) {
    return false;
  }
  if (event.type === "user.created" || event.type === "user.deleted") {
    return true;
  }
  if (event.type === "conversation.created" || event.type === "room.created") {
    return Boolean(event.room?.is_direct);
  }
  return false;
}

export function appendMessageToData<T extends IMData | null | undefined>(
  current: T,
  conversationID: string | null | undefined,
  message: IMMessage | null | undefined,
): T | IMData {
  if (!current || !conversationID || !message) {
    return current;
  }

  const rooms = current.rooms.map((room) => {
    if (room.id !== conversationID) {
      return room;
    }
    if (room.messages.some((item) => item.id === message.id)) {
      return room;
    }
    return { ...room, messages: [...room.messages, message] };
  });
  return { ...current, rooms: sortConversations(rooms) };
}

export function upsertConversationInData<T extends IMData | null | undefined>(
  current: T,
  conversation: IMConversation | null | undefined,
): T | IMData {
  if (!current || !conversation) {
    return current;
  }

  const existing = current.rooms.some((item) => item.id === conversation.id);
  const rooms = existing
    ? current.rooms.map((item) => (item.id === conversation.id ? conversation : item))
    : [conversation, ...current.rooms];
  return { ...current, rooms: sortConversations(rooms) };
}

export function upsertUserInData<T extends IMData | null | undefined>(
  current: T,
  user: IMUser | null | undefined,
): T | IMData {
  if (!current || !user) {
    return current;
  }

  const existing = current.users.some((item) => item.id === user.id);
  const users = existing ? current.users.map((item) => (item.id === user.id ? user : item)) : [...current.users, user];
  users.sort((a, b) => a.name.localeCompare(b.name));
  return { ...current, users };
}

export function removeUserFromData<T extends IMData | null | undefined>(
  current: T,
  userID: string | null | undefined,
): T | IMData {
  if (!current || !userID) {
    return current;
  }

  const users = current.users.filter((item) => item.id !== userID);
  const rooms = current.rooms
    .map((room) => {
      const members = room.members.filter((id) => id !== userID);
      const messages = room.messages.filter((message) => message.sender_id !== userID);
      if (members.length < 2) {
        return null;
      }
      return {
        ...room,
        members,
        messages,
      };
    })
    .filter(Boolean);

  return { ...current, users, rooms: sortConversations(rooms) };
}

export function removeConversationFromData<T extends IMData | null | undefined>(
  current: T,
  conversationID: string | null | undefined,
): T | IMData {
  if (!current || !conversationID) {
    return current;
  }

  const rooms = current.rooms.filter((item) => item.id !== conversationID);
  return { ...current, rooms };
}

export function sortConversations(conversations: readonly IMConversation[]): IMConversation[] {
  return [...conversations].sort((a, b) => latestAt(b) - latestAt(a));
}

export function normalizeIMData<T extends Partial<IMData> | null | undefined>(payload: T): T | IMData {
  if (!payload) {
    return payload;
  }
  return { ...payload, rooms: payload.rooms ?? [], users: payload.users ?? [] } as T | IMData;
}
