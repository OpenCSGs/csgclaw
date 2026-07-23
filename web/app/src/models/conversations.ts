import { flattenMentionText } from "@/components/business/MessageContent/mentions";
import {
  isOpenClawToolDeliveryMessage,
  openClawDeliveryKind,
  parseAgentActivity,
  parseLegacyToolActivityCommand,
  parseMessageActivityCommand,
  parsePlainAgentCommand,
} from "@/models/agentActivity";
import { renderSlashCommandPreviewText } from "@/models/slashCommands";
import type { WorkspaceTeam } from "@/models/tasks";
import type { MessageAttachment } from "@/models/attachments";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";

export type LocaleCode = "zh" | "en" | string;
export type TranslateFn = (key: string, params?: Record<string, string | number>) => string;

export type IMParticipantLike = {
  agent_id?: string | null;
  channel?: string | null;
  channel_app_ref?: string | null;
  channel_user_kind?: string | null;
  channel_user_ref?: string | null;
  id?: string | null;
  lifecycle_status?: string | null;
  mentionable?: boolean | null;
  metadata?: Record<string, unknown> | null;
  name?: string | null;
  type?: string | null;
  user_id?: string | null;
  user_name?: string | null;
};

export type IMUser = {
  accent_hex?: string | null;
  avatar?: string | null;
  description?: string | null;
  id: string;
  is_online?: boolean | null;
  name?: string | null;
  participants?: IMParticipantLike[] | null;
  role?: string | null;
  user_id?: string | null;
};

export type HumanChannelID = "feishu";

export type HumanConnectedChannel = {
  channelUserName?: string;
  channelUserRef?: string;
  id: HumanChannelID;
  name: string;
  participantID: string;
};

export const HUMAN_CHANNELS: Record<HumanChannelID, { id: HumanChannelID; name: string }> = {
  feishu: {
    id: "feishu",
    name: "Feishu",
  },
};

export type MessageMention = string | { id?: string | null };
export const THREAD_RELATION_TYPE = "m.thread";

export type IMMessageEvent = {
  actor_id?: string | null;
  key?: string | null;
  target_ids?: string[] | null;
  title?: string | null;
};

export type MessageRelation = {
  event_id?: string | null;
  rel_type?: string | null;
};

export type ThreadContextSummary = {
  after_count?: number;
  before_count?: number;
  message_count?: number;
  root_excerpt?: string;
};

export type ThreadSummary = {
  context_summary?: ThreadContextSummary;
  current_user_participated?: boolean;
  latest_reply?: IMMessage | null;
  participants?: { id?: string | null; name?: string | null }[] | null;
  reply_count?: number;
  root_id?: string;
};

export type ThreadState = {
  context?: IMMessage[] | null;
  created_at?: string;
  root_message_id?: string;
  summary?: ThreadContextSummary;
};

export type IMMessage = {
  attachments?: MessageAttachment[] | null;
  id?: string;
  content: string;
  created_at?: string;
  event?: IMMessageEvent | null;
  kind?: string | null;
  metadata?: Record<string, unknown> | null;
  mentions?: MessageMention[] | null;
  relates_to?: MessageRelation | null;
  sender_id?: string | null;
  thread?: ThreadSummary | null;
};

export type IMConversation = {
  description?: string | null;
  id: string;
  is_direct?: boolean | null;
  members: string[];
  messages: IMMessage[];
  threads?: ThreadState[] | null;
  title?: string | null;
};

export type IMData = {
  current_user_id?: string;
  rooms: IMConversation[];
  users: IMUser[];
};

export type ParticipantWorkUpdate = {
  expires_at: string;
  kind: "agent_turn";
  lease_id: string;
  participant_id: string;
  reason: "started" | "renewed" | "released" | "expired";
  registry_epoch: string;
  request_id: string;
  revision: number;
  room_id: string;
  state: "working" | "idle";
  thread_root_id?: string | null;
  user_id: string;
};

export type IMServerEvent = {
  message?: IMMessage | null;
  participant?: IMParticipantLike | null;
  room?: Partial<IMConversation> | null;
  room_id?: string | null;
  team?: WorkspaceTeam | null;
  team_id?: string | null;
  thread?: ThreadView | null;
  type?: string | null;
  upgrade?: unknown;
  user?: IMUser | null;
  work?: ParticipantWorkUpdate | null;
};

export type ThreadView = {
  context?: IMMessage[] | null;
  replies?: IMMessage[] | null;
  room_id?: string;
  root?: IMMessage | null;
  summary?: ThreadSummary | null;
};

export type UsersById = Map<string, IMUser>;

export function buildUsersById(users: readonly IMUser[] | null | undefined): UsersById {
  const result: UsersById = new Map();
  (users || []).forEach((user) => {
    addUserIdentityAliases(result, user);
  });
  return result;
}

export function resolveUserByLocalIdentity(id: string | null | undefined, usersById: UsersById): IMUser | undefined {
  const aliases = localIdentityAliases(id);
  for (const alias of aliases) {
    const user = usersById.get(alias);
    if (user) {
      return user;
    }
  }
  return undefined;
}

export function localIdentitiesMatch(a: string | null | undefined, b: string | null | undefined): boolean {
  const aUserID = userIDForLocalIdentity(a);
  const bUserID = userIDForLocalIdentity(b);
  return Boolean(aUserID && bUserID && aUserID === bUserID);
}

export function userIDForLocalIdentity(id: string | null | undefined): string {
  const raw = String(id || "").trim();
  if (!raw) {
    return "";
  }
  const base = localIdentityBase(raw);
  if (!base) {
    return "";
  }
  return `user-${base}`;
}

export function participantIDForLocalIdentity(id: string | null | undefined): string {
  const raw = String(id || "").trim();
  if (!raw) {
    return "";
  }
  const base = localIdentityBase(raw);
  if (!base) {
    return "";
  }
  return `pt-${base}`;
}

function addUserIdentityAliases(usersById: UsersById, user: IMUser): void {
  for (const alias of localIdentityAliases(user.id)) {
    addUserAlias(usersById, alias, user);
  }
  (user.participants || []).forEach((participant) => {
    for (const alias of localIdentityAliases(participant.id)) {
      addUserAlias(usersById, alias, user);
    }
    for (const alias of localIdentityAliases(participant.channel_user_ref)) {
      addUserAlias(usersById, alias, user);
    }
  });
}

function addUserAlias(usersById: UsersById, alias: string, user: IMUser): void {
  if (!alias || usersById.has(alias)) {
    return;
  }
  usersById.set(alias, user);
}

function localIdentityAliases(id: string | null | undefined): string[] {
  const raw = String(id || "").trim();
  if (!raw) {
    return [];
  }
  const userID = userIDForLocalIdentity(raw);
  const participantID = participantIDForLocalIdentity(raw);
  const base = localIdentityBase(raw);
  return uniqueStrings([
    raw,
    userID,
    participantID,
    base,
    base ? `agent-${base}` : "",
    base ? `u-${base}` : "",
    base ? `user-agent-${base}` : "",
    base ? `pt-agent-${base}` : "",
  ]);
}

function localIdentityBase(id: string): string {
  let value = String(id || "").trim();
  if (!value) {
    return "";
  }
  for (;;) {
    const stripped = stripLocalIdentityPrefixes(value);
    const hashTrimmed = trimStableHashSuffix(stripped);
    const next = stripLocalIdentityPrefixes(hashTrimmed);
    if (next === value) {
      return next;
    }
    value = next;
  }
}

function stripLocalIdentityPrefixes(id: string): string {
  let value = String(id || "").trim();
  for (;;) {
    const next = ["user-", "agent-", "pt-", "u-"].find((prefix) => value.startsWith(prefix));
    if (!next) {
      return value;
    }
    value = value.slice(next.length);
  }
}

function trimStableHashSuffix(id: string): string {
  const idx = id.lastIndexOf("-");
  if (idx <= 0 || id.length - idx - 1 !== 8) {
    return id;
  }
  return /^[0-9a-f]{8}$/.test(id.slice(idx + 1)) ? id.slice(0, idx) : id;
}

function uniqueStrings(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  values.forEach((value) => {
    const trimmed = String(value || "").trim();
    if (!trimmed || seen.has(trimmed)) {
      return;
    }
    seen.add(trimmed);
    out.push(trimmed);
  });
  return out;
}

export function isToolCallMessage(messageOrContent: IMMessage | unknown): boolean {
  if (isMessageLike(messageOrContent)) {
    const activity = parseAgentActivity(messageOrContent);
    if (activity) {
      return activity.content.msgtype !== AgentActivityMsgTypes.question;
    }
    if (isOpenClawToolDeliveryMessage(messageOrContent)) {
      return true;
    }
    if (openClawDeliveryKind(messageOrContent)) {
      return false;
    }
    return Boolean(parseMessageActivityCommand(messageOrContent));
  }
  return isNonMessageActivityContent(messageOrContent);
}

function isNonMessageActivityContent(content: unknown): boolean {
  const activity = parseAgentActivity(content);
  return Boolean(
    (activity && activity.content.msgtype !== AgentActivityMsgTypes.question) || isLegacyToolCallContent(content),
  );
}

function isLegacyToolCallContent(content: unknown): boolean {
  return Boolean(parseLegacyToolActivityCommand(content) || parsePlainAgentCommand(content));
}

function isMessageLike(value: unknown): value is IMMessage {
  return Boolean(value && typeof value === "object" && "content" in value);
}

export function isThreadReply(message: IMMessage | null | undefined): boolean {
  return Boolean(threadRootID(message));
}

export function threadRootID(message: IMMessage | null | undefined): string {
  if (message?.relates_to?.rel_type !== THREAD_RELATION_TYPE) {
    return "";
  }
  return String(message.relates_to.event_id || "").trim();
}

export function isEventMessage(message: IMMessage | null | undefined): boolean {
  if (message?.kind === "event") {
    return true;
  }
  return isLegacySystemEventContent(message?.content);
}

export function formatConversationPreview(
  message: IMMessage | null | undefined,
  conversation: IMConversation | null | undefined,
  currentUserID: string,
  usersById: UsersById,
  locale: LocaleCode,
  t: TranslateFn,
): string {
  if (message) {
    if (isEventMessage(message)) {
      return formatEventMessage(message, usersById, locale);
    }
    return formatMessagePreviewText(message.content);
  }
  return conversation ? getConversationSubtitle(conversation, currentUserID, usersById, locale, t) : "";
}

export function formatMessagePreviewText(content: unknown): string {
  return collapsePreviewWhitespace(stripPreviewCodeFence(flattenMentionText(renderSlashCommandPreviewText(content))));
}

export type MessagePreviewTextToken = {
  text: string;
  type: "text" | "mention" | "slash";
};

const previewTokenPattern = /(^|[\s])\/[A-Za-z0-9._-]+(?=$|[\s.,!?;:)\]])|@[\w.-]+/g;

export function splitMessagePreviewText(content: unknown): MessagePreviewTextToken[] {
  const text = formatMessagePreviewText(content);
  if (!text) {
    return [];
  }
  const tokens: MessagePreviewTextToken[] = [];
  let last = 0;
  for (const match of text.matchAll(previewTokenPattern)) {
    const fullMatch = match[0] || "";
    if (!fullMatch) {
      continue;
    }
    const matchText = fullMatch.trimStart();
    const start = (match.index || 0) + (fullMatch.length - matchText.length);
    if (start > last) {
      tokens.push({ text: text.slice(last, start), type: "text" });
    }
    const type: MessagePreviewTextToken["type"] = matchText.startsWith("/") ? "slash" : "mention";
    tokens.push({ text: matchText, type });
    last = Math.max(last, start + matchText.length);
  }
  if (last < text.length) {
    tokens.push({ text: text.slice(last), type: "text" });
  }
  return tokens.length > 0 ? tokens : [{ text, type: "text" }];
}

function stripPreviewCodeFence(content: string): string {
  let text = content.trim();
  if (!text.startsWith("```")) {
    return text;
  }

  text = text.slice(3).trimStart();
  const lineBreakIndex = text.search(/\r?\n/);
  if (lineBreakIndex >= 0) {
    text = text.slice(lineBreakIndex).trimStart();
  } else {
    text = text.replace(
      /^(?:text|txt|plain|json|jsonc|yaml|yml|toml|go|ts|tsx|js|jsx|bash|sh|zsh|fish|python|py|html|css|sql|md|markdown|diff|xml|dockerfile|makefile|ini|env|csv)\b\s+/i,
      "",
    );
  }
  return text.replace(/\s*```\s*$/, "").trim();
}

function collapsePreviewWhitespace(content: string): string {
  return content.replace(/\s+/g, " ").trim();
}

export function threadKey(roomID: string | null | undefined, rootID: string | null | undefined): string {
  return `${String(roomID || "").trim()}:${String(rootID || "").trim()}`;
}

export function threadMessageKey(roomID: string | null | undefined, message: IMMessage | null | undefined): string {
  return threadKey(roomID, threadRootID(message));
}

export function threadViewKey(thread: ThreadView | null | undefined): string {
  return threadKey(thread?.room_id, thread?.summary?.root_id || thread?.root?.id);
}

export function formatThreadReplyCount(count: number | null | undefined, t: TranslateFn): string {
  return t("threadReplies", { count: Number(count) || 0 });
}

export function threadHasReplies(summary: ThreadSummary | null | undefined): boolean {
  return Number(summary?.reply_count) > 0;
}

export function conversationThreadViews(conversation: IMConversation | null | undefined): ThreadView[] {
  if (!conversation?.messages?.length) {
    return [];
  }
  return conversation.messages
    .filter((message) => threadHasReplies(message.thread))
    .map((message) => ({
      room_id: conversation.id,
      root: message,
      summary: message.thread,
    }))
    .sort((a, b) => latestThreadTimestamp(b) - latestThreadTimestamp(a));
}

export function latestThreadTimestamp(thread: ThreadView | null | undefined): number {
  const value = thread?.summary?.latest_reply?.created_at || thread?.root?.created_at;
  const ms = value ? new Date(value).getTime() : 0;
  return Number.isFinite(ms) ? ms : 0;
}

export function formatEventMessage(
  message: IMMessage | null | undefined,
  usersById: UsersById,
  locale: LocaleCode,
): string {
  if (!message) {
    return "";
  }
  if (message.event?.key === "task_assigned") {
    return formatTaskAssignedEventMessage(message.event, usersById, locale);
  }
  return message.content || "";
}

function formatTaskAssignedEventMessage(event: IMMessageEvent, usersById: UsersById, locale: LocaleCode): string {
  const taskLabel = String(event.title || "").trim() || "task";
  const targets = (event.target_ids || [])
    .map((id) => userDisplayName(id, usersById))
    .filter(Boolean)
    .join(isChineseLocale(locale) ? "、" : ", ");
  if (!targets) {
    return isChineseLocale(locale) ? `${taskLabel} 已指派` : `${taskLabel} assigned`;
  }
  return isChineseLocale(locale) ? `${taskLabel} 指派给 ${targets}` : `${taskLabel} assigned to ${targets}`;
}

function isChineseLocale(locale: LocaleCode): boolean {
  return String(locale || "")
    .toLowerCase()
    .startsWith("zh");
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
    /^.+ created the room(?: ".+")?\.?$/,
    /^.+ created the channel(?: ".+")?\.?$/,
    /^.+ 邀请 .+ 加入了房间。?$/,
    /^.+ 邀请 .+ 加入了频道。?$/,
    /^.+ 创建了房间(?:“.+”)?。?$/,
    /^.+ 创建了频道(?:“.+”)?。?$/,
  ].some((pattern) => pattern.test(text));
}

export function userDisplayName(userID: string | null | undefined, usersById: UsersById): string {
  if (!userID) {
    return "";
  }
  const user = resolveUserByLocalIdentity(userID, usersById);
  if (!user) {
    return userID;
  }
  return user.name || userID;
}

export function resolveConversationUser(
  conversation: IMConversation,
  currentUserID: string,
  usersById: UsersById,
): IMUser | undefined {
  const otherID = conversation.members.find((id) => !localIdentitiesMatch(id, currentUserID)) ?? currentUserID;
  return resolveUserByLocalIdentity(otherID, usersById);
}

export function agentMatchesUser(
  agent: {
    id?: string | null;
    name?: string | null;
    participants?: IMParticipantLike[] | null;
    user_id?: string | null;
  } | null,
  user:
    | { id?: string | null; name?: string | null; participants?: IMParticipantLike[] | null; user_id?: string | null }
    | null
    | undefined,
): boolean {
  if (!agent || !user) {
    return false;
  }
  if (agentIdentityMatchesUser(agent, user)) {
    return true;
  }
  const agentName = normalizeComparable(agent.name);
  const userName = normalizeComparable(user.name);
  if (agentName && userName && agentName === userName) {
    return true;
  }
  return false;
}

export function agentIdentityMatchesUser(
  agent: {
    id?: string | null;
    participants?: IMParticipantLike[] | null;
    user_id?: string | null;
  } | null,
  user: { id?: string | null; participants?: IMParticipantLike[] | null; user_id?: string | null } | null | undefined,
): boolean {
  if (!agent || !user) {
    return false;
  }
  const agentAliases = localEntityAliasSet([
    agent.id,
    agent.user_id,
    ...(agent.participants || []).flatMap(participantAliases),
  ]);
  const userAliases = localEntityAliasSet([
    user.id,
    user.user_id,
    ...(user.participants || []).flatMap(participantAliases),
  ]);
  return [...agentAliases].some((alias) => userAliases.has(alias));
}

export function resolveAgentForUser<
  T extends {
    id?: string | null;
    name?: string | null;
    participants?: IMParticipantLike[] | null;
    user_id?: string | null;
  },
>(
  agents: readonly T[],
  user:
    | { id?: string | null; name?: string | null; participants?: IMParticipantLike[] | null; user_id?: string | null }
    | null
    | undefined,
  alternateUsers: readonly {
    id?: string | null;
    name?: string | null;
    participants?: IMParticipantLike[] | null;
    user_id?: string | null;
  }[] = [],
): T | null {
  const users = [user, ...alternateUsers].filter((item): item is NonNullable<typeof user> => item != null);
  return (
    agents.find((agent) => users.some((candidate) => agentIdentityMatchesUser(agent, candidate))) ??
    agents.find((agent) => users.some((candidate) => agentMatchesUser(agent, candidate))) ??
    null
  );
}

function localEntityAliasSet(values: Array<string | null | undefined>): Set<string> {
  const aliases = new Set<string>();
  values.forEach((value) => {
    localIdentityAliases(value).forEach((alias) => aliases.add(alias));
  });
  return aliases;
}

function participantAliases(participant: IMParticipantLike | null | undefined): Array<string | null | undefined> {
  return [participant?.id, participant?.user_id, participant?.agent_id, participant?.channel_user_ref];
}

export function feishuHumanParticipant(user: IMUser | null | undefined): IMParticipantLike | null {
  const participant = user?.participants?.find((candidate) => {
    if (String(candidate?.channel || "").trim() !== "feishu") {
      return false;
    }
    if (String(candidate?.type || "").trim() !== "human") {
      return false;
    }
    if (String(candidate?.channel_user_kind || "").trim() !== "open_id") {
      return false;
    }
    return Boolean(String(candidate?.id || "").trim() && String(candidate?.channel_user_ref || "").trim());
  });
  return participant ?? null;
}

export function humanParticipantDisplayName(participant: IMParticipantLike | null | undefined): string {
  const name = String(participant?.name || "").trim();
  if (name) {
    return name;
  }
  return String(participant?.channel_user_ref || "").trim();
}

export function humanConnectedChannels(user: IMUser | null | undefined): HumanConnectedChannel[] {
  const feishuParticipant = feishuHumanParticipant(user);
  if (!feishuParticipant) {
    return [];
  }
  return [
    {
      id: "feishu",
      name: HUMAN_CHANNELS.feishu.name,
      participantID: String(feishuParticipant.id || ""),
      channelUserName: humanParticipantDisplayName(feishuParticipant),
      channelUserRef: String(feishuParticipant.channel_user_ref || ""),
    },
  ];
}

export function hasConnectedHumanChannel(user: IMUser | null | undefined, channelID: HumanChannelID): boolean {
  return humanConnectedChannels(user).some((channel) => channel.id === channelID);
}

export function normalizeComparable(value: unknown): string {
  return String(value || "")
    .trim()
    .toLowerCase();
}

export function isDirectConversation(conversation: { is_direct?: boolean | null } | null | undefined): boolean {
  return Boolean(conversation?.is_direct);
}

export function resolveRoomInviterID(
  room: Pick<IMConversation, "members"> | null | undefined,
  options: { preferredInviterIDs?: Array<string | null | undefined> } = {},
): string {
  const members = new Set((room?.members ?? []).map((id) => String(id).trim()).filter(Boolean));
  if (!members.size) {
    return "";
  }
  for (const candidate of options.preferredInviterIDs ?? []) {
    const id = String(candidate ?? "").trim();
    if (id && members.has(id)) {
      return id;
    }
  }
  return [...members][0] ?? "";
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
  });
}

/** Format a timestamp for sidebar list items:
 *  today → time only, yesterday → "昨天"/"Yesterday",
 *  2–7 days ago → weekday, same year → month-day, older → year-month-day. */
export function formatSidebarTime(
  value: string | number | Date | null | undefined,
  locale: LocaleCode,
  t: TranslateFn,
  now: Date = new Date(),
): string {
  const parts = formatMessageTimestampParts(value, locale, t, now);
  if (!parts.label) return "";
  const dayDiff = differenceInCalendarDays(now, value ? new Date(value) : new Date(NaN));
  return dayDiff === 0 ? parts.label : parts.dividerLabel;
}

export type MessageTimestampParts = {
  dateTime: string;
  dividerLabel: string;
  label: string;
  shortLabel: string;
  tooltip: string;
};

export function formatMessageTimestampParts(
  value: string | number | Date | null | undefined,
  locale: LocaleCode,
  t: TranslateFn,
  now: Date = new Date(),
): MessageTimestampParts {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) {
    return { dateTime: "", dividerLabel: "", label: "", shortLabel: "", tooltip: "" };
  }

  const dayDiff = differenceInCalendarDays(now, date);
  const time = formatClockTime(date);
  const dateTime = date.toISOString();
  const tooltip = formatTimestampTooltip(date);

  if (dayDiff === 0) {
    return { dateTime, dividerLabel: t("timestampToday"), label: time, shortLabel: time, tooltip };
  }
  if (dayDiff === 1) {
    return {
      dateTime,
      dividerLabel: t("timestampYesterday"),
      label: `${t("timestampYesterday")} ${time}`,
      shortLabel: time,
      tooltip,
    };
  }
  if (dayDiff > 1 && dayDiff <= 7) {
    const weekday = formatWeekday(date, locale);
    return { dateTime, dividerLabel: weekday, label: `${weekday} ${time}`, shortLabel: time, tooltip };
  }
  if (date.getFullYear() === now.getFullYear()) {
    const monthDay = formatTimestampMonthDay(date, locale);
    return { dateTime, dividerLabel: monthDay, label: `${monthDay} ${time}`, shortLabel: time, tooltip };
  }
  const yearMonthDay = formatTimestampYearMonthDay(date, locale);
  return { dateTime, dividerLabel: yearMonthDay, label: `${yearMonthDay} ${time}`, shortLabel: time, tooltip };
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

  if ((event.type === "user.created" || event.type === "user.updated") && event.user) {
    return upsertUserInData(current, event.user);
  }
  if (event.type === "user.deleted" && event.user) {
    return removeUserFromData(current, event.user.id);
  }
  if (event.type === "message.created" && event.message) {
    return appendMessageToData(current, event.room_id, event.message);
  }
  if ((event.type === "thread.created" || event.type === "thread.updated") && event.thread) {
    return applyThreadToData(current, event.room_id || event.thread.room_id, event.thread);
  }
  if (
    (event.type === "conversation.created" ||
      event.type === "conversation.members_added" ||
      event.type === "room.created" ||
      event.type === "room.members_added" ||
      event.type === "room.members_removed" ||
      event.type === "room.messages_cleared") &&
    event.room?.id
  ) {
    return upsertConversationInData(current, event.room as IMConversation);
  }
  if (event.type === "room.deleted") {
    return removeConversationFromData(current, event.room_id || event.room?.id);
  }
  return current;
}

function formatClockTime(date: Date): string {
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

function formatTimestampTooltip(date: Date): string {
  return `${formatTimestampDate(date)} ${formatTimestampSeconds(date)}`;
}

function formatTimestampDate(date: Date): string {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
}

function formatTimestampMonthDay(date: Date, locale: LocaleCode): string {
  if (locale === "zh") {
    return `${date.getMonth() + 1}月${date.getDate()}日`;
  }
  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
}

function formatTimestampYearMonthDay(date: Date, locale: LocaleCode): string {
  if (locale === "zh") {
    return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日`;
  }
  return formatTimestampDate(date);
}

function formatTimestampSeconds(date: Date): string {
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`;
}

function formatWeekday(date: Date, locale: LocaleCode): string {
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    weekday: "short",
  }).format(date);
}

function differenceInCalendarDays(left: Date, right: Date): number {
  const leftStart = new Date(left.getFullYear(), left.getMonth(), left.getDate());
  const rightStart = new Date(right.getFullYear(), right.getMonth(), right.getDate());
  return Math.floor((leftStart.getTime() - rightStart.getTime()) / 86400000);
}

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

export function isAgentRosterEvent(event: IMServerEvent | null | undefined): boolean {
  if (!event?.type) {
    return false;
  }
  if (event.type === "user.created" || event.type === "user.updated" || event.type === "user.deleted") {
    return true;
  }
  if (
    event.type === "participant.created" ||
    event.type === "participant.updated" ||
    event.type === "participant.deleted"
  ) {
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
  if (isThreadReply(message)) {
    return current;
  }

  const rooms = current.rooms.map((room) => {
    if (room.id !== conversationID) {
      return room;
    }
    const existingIndex = room.messages.findIndex((item) => item.id === message.id);
    if (existingIndex >= 0) {
      return {
        ...room,
        messages: room.messages.map((item, index) => (index === existingIndex ? message : item)),
      };
    }
    return { ...room, messages: [...room.messages, message] };
  });
  return { ...current, rooms: sortConversations(rooms) };
}

export function applyThreadToData<T extends IMData | null | undefined>(
  current: T,
  conversationID: string | null | undefined,
  thread: ThreadView | null | undefined,
): T | IMData {
  const rootID = thread?.summary?.root_id || thread?.root?.id;
  if (!current || !conversationID || !rootID || !thread?.summary) {
    return current;
  }

  const rooms = current.rooms.map((room) => {
    if (room.id !== conversationID) {
      return room;
    }
    const messages = room.messages.map((message) =>
      message.id === rootID ? { ...message, thread: thread.summary } : message,
    );
    const threads = upsertThreadState(room.threads ?? [], thread);
    return { ...room, messages, threads };
  });
  return { ...current, rooms: sortConversations(rooms) };
}

export function upsertThreadState(states: readonly ThreadState[], thread: ThreadView): ThreadState[] {
  const rootID = thread?.summary?.root_id || thread?.root?.id;
  if (!rootID) {
    return [...states];
  }
  const state: ThreadState = {
    root_message_id: rootID,
    created_at: thread?.root?.created_at || new Date().toISOString(),
    context: thread?.context ?? [],
    summary: thread?.summary?.context_summary ?? {},
  };
  const existing = states.some((item) => item.root_message_id === rootID);
  return existing
    ? states.map((item) => (item.root_message_id === rootID ? { ...item, ...state } : item))
    : [...states, state];
}

export function appendReplyToThreadView(
  current: ThreadView | null | undefined,
  message: IMMessage | null | undefined,
): ThreadView | null | undefined {
  if (!current || !message || threadRootID(message) !== (current.summary?.root_id || current.root?.id)) {
    return current;
  }
  if (current.replies?.some((item) => item.id === message.id)) {
    const replies = current.replies.map((item) => (item.id === message.id ? message : item));
    const summary = {
      ...(current.summary ?? {}),
      reply_count: replies.length,
      latest_reply: replies[replies.length - 1],
    };
    return {
      ...current,
      root: current.root ? { ...current.root, thread: summary } : current.root,
      replies,
      summary,
    };
  }
  const replies = [...(current.replies ?? []), message];
  const summary = {
    ...(current.summary ?? {}),
    reply_count: replies.length,
    latest_reply: message,
  };
  return {
    ...current,
    root: current.root ? { ...current.root, thread: summary } : current.root,
    replies,
    summary,
  };
}

export function upsertConversationInData<T extends IMData | null | undefined>(
  current: T,
  conversation: IMConversation | null | undefined,
): T | IMData {
  if (!current || !conversation) {
    return current;
  }

  const existing = current.rooms.some((item) => item.id === conversation.id);
  const normalized = normalizeRoom(conversation);
  const rooms = existing
    ? current.rooms.map((item) => (item.id === conversation.id ? normalized : item))
    : [normalized, ...current.rooms];
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
  users.sort((a, b) => String(a.name ?? "").localeCompare(String(b.name ?? "")));
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
    .filter((room): room is IMConversation => Boolean(room));

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
  return { ...payload, rooms: (payload.rooms ?? []).map(normalizeRoom), users: payload.users ?? [] } as T | IMData;
}

export function normalizeRoom(room: IMConversation): IMConversation {
  return {
    ...room,
    messages: room.messages ?? [],
    threads: room.threads ?? [],
  };
}
