import {
  agentActivityMessageToolMergeKey,
  agentActivityToolMergeKeys,
  isTerminalAgentActivityTool,
  parseAgentActivity,
  parseMessageActivityCommand,
  type AgentActivityCommand,
  type AgentActivityPayload,
  type AgentActivityTool,
} from "@/models/agentActivity";
import { resolveAgentChannelUserID, type AgentLike } from "@/models/agents";
import {
  localIdentitiesMatch,
  resolveUserByLocalIdentity,
  type IMConversation,
  type IMMessage,
  type UsersById,
} from "@/models/conversations";
import { AgentActivityKinds, AgentActivityMsgTypes } from "@/shared/constants/messages";

export type ConversationActivityAgent = {
  id: string;
  identities: string[];
  name: string;
};

export type ConversationActivityTone = "error" | "message" | "other" | "tool";

export type ConversationActivitySource = "agent" | "user";

export type ConversationActivityEntry = {
  activity: AgentActivityPayload | null;
  agentID: string;
  agentName: string;
  command: AgentActivityCommand | null;
  createdAt: string;
  eventType: string;
  id: string;
  index: number;
  message: IMMessage;
  source: ConversationActivitySource;
  tone: ConversationActivityTone;
  updatedAt: string;
};

export type ConversationActivityDetail = {
  kind: "command" | "content" | "input" | "result";
  value: string;
};

export type ConversationActivityDensitySegment = {
  durationPercent: number;
  entry: ConversationActivityEntry;
  startPercent: number;
};

type MutableConversationActivityEntry = Omit<ConversationActivityEntry, "eventType" | "index" | "tone">;

const MAX_POINT_EVENT_WEIGHT_MS = 1_000;

export function conversationActivityAgents(
  conversation: IMConversation,
  agents: readonly AgentLike[],
): ConversationActivityAgent[] {
  const matched = new Map<string, ConversationActivityAgent>();
  for (const agent of agents) {
    const identities = agentIdentityValues(agent);
    if (!identities.length || !conversation.members.some((memberID) => identityMatches(memberID, identities))) {
      continue;
    }
    const id = firstNonEmpty(agent.id, agent.user_id, resolveAgentChannelUserID(agent), identities[0]);
    if (!id || matched.has(id)) {
      continue;
    }
    matched.set(id, {
      id,
      identities,
      name: firstNonEmpty(agent.name, agent.user_name, id),
    });
  }
  return Array.from(matched.values());
}

export function conversationActivityEntries(
  messages: readonly IMMessage[],
  agents: readonly ConversationActivityAgent[],
  conversationMemberIDs: readonly string[] = [],
  usersById?: UsersById,
): ConversationActivityEntry[] {
  const entries: MutableConversationActivityEntry[] = [];
  const mergedEntries = new Map<string, MutableConversationActivityEntry>();

  sortMessages(messages).forEach((message) => {
    const activity = parseAgentActivity(message.content);
    const agent = resolveMessageAgent(message, activity, agents);
    const userPrompt = isConversationUserPrompt(message, activity, agent, conversationMemberIDs, usersById);
    if (!agent && !userPrompt) {
      return;
    }
    const command = agent && !activity ? parseMessageActivityCommand(message) : null;
    const body = cleanMessageBody(message.content);
    if (!activity && !command && !body) {
      return;
    }

    const agentID = agent?.id || "";
    const mergeKeys = agent ? activityMergeKeys(agentID, message, activity, command) : [];
    const existing = mergeKeys.map((key) => mergedEntries.get(key)).find(Boolean);
    if (existing) {
      mergeActivityEntry(existing, message, activity, command);
      mergeKeys.forEach((key) => mergedEntries.set(key, existing));
      return;
    }

    const entry: MutableConversationActivityEntry = {
      activity,
      agentID,
      agentName: agent?.name || "",
      command,
      createdAt: message.created_at || "",
      id: activityEntryID(agentID || "user", message, entries.length),
      message,
      source: userPrompt ? "user" : "agent",
      updatedAt: message.created_at || "",
    };
    entries.push(entry);
    mergeKeys.forEach((key) => mergedEntries.set(key, entry));
  });

  return entries
    .sort((left, right) => activityTime(left.createdAt) - activityTime(right.createdAt))
    .map((entry, index) => {
      const eventType = activityEventType(entry);
      return {
        ...entry,
        eventType,
        index: index + 1,
        tone: activityTone(entry, eventType),
      };
    });
}

export function mergeConversationActivityMessages(
  ...collections: ReadonlyArray<readonly IMMessage[] | null | undefined>
): IMMessage[] {
  const messages = new Map<string, IMMessage>();
  collections.forEach((collection) => {
    (collection || []).forEach((message) => messages.set(messageMergeKey(message), message));
  });
  return sortMessages(Array.from(messages.values()));
}

export function conversationActivityDensitySegments(
  entries: readonly ConversationActivityEntry[],
): ConversationActivityDensitySegment[] {
  const timedEntries = entries.map((entry) => {
    const start = activityTime(entry.createdAt);
    return {
      end: Math.max(start, activityTime(entry.updatedAt)),
      entry,
      start,
    };
  });
  const durations = timedEntries.map(({ end, start }) => (start > 0 ? Math.max(0, end - start) : 0));
  const positiveDurations = durations.filter((duration) => duration > 0);
  const pointEventWeight = positiveDurations.length
    ? Math.min(MAX_POINT_EVENT_WEIGHT_MS, Math.min(...positiveDurations))
    : 1;
  const weights = durations.map((duration) => duration || pointEventWeight);
  const totalWeight = weights.reduce((total, weight) => total + weight, 0);
  let elapsedWeight = 0;

  return timedEntries.map(({ entry }, index) => {
    const startPercent = totalWeight > 0 ? (elapsedWeight / totalWeight) * 100 : 0;
    elapsedWeight += weights[index] || 0;
    return {
      durationPercent:
        index === timedEntries.length - 1
          ? Math.max(0, 100 - startPercent)
          : totalWeight > 0
            ? ((weights[index] || 0) / totalWeight) * 100
            : 0,
      entry,
      startPercent,
    };
  });
}

export function conversationActivityEntrySummary(entry: ConversationActivityEntry): string {
  if (entry.command) {
    return firstNonEmpty(entry.command.command, entry.command.title, entry.command.output);
  }
  const activity = entry.activity;
  if (activity?.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    const tool = activity.content.tool;
    return firstNonEmpty(
      toolCommandText(tool),
      toolInputText(tool),
      toolOutputText(tool),
      tool.title,
      activity.content.body,
    );
  }
  if (activity?.content.msgtype === AgentActivityMsgTypes.action) {
    const action = activity.content.action;
    return action ? `${action.title} · ${action.status}` : activity.content.body;
  }
  if (activity) {
    return activity.content.body;
  }
  return truncateText(plainText(entry.message.content), 220);
}

export function conversationActivityEntryDetails(entry: ConversationActivityEntry): ConversationActivityDetail[] {
  if (entry.command) {
    return compactDetails([
      { kind: "command", value: entry.command.command },
      { kind: "result", value: entry.command.output || "" },
    ]);
  }
  const activity = entry.activity;
  if (activity?.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    const tool = activity.content.tool;
    const command = toolCommandText(tool);
    return compactDetails([
      { kind: command ? "command" : "input", value: command || toolInputText(tool) },
      { kind: "result", value: toolOutputText(tool) },
    ]);
  }
  if (activity) {
    return compactDetails([{ kind: "content", value: activity.content.body }]);
  }
  return compactDetails([{ kind: "content", value: cleanMessageBody(entry.message.content) }]);
}

function agentIdentityValues(agent: AgentLike): string[] {
  const identities: string[] = [];
  const add = (value: unknown) => {
    const id = String(value ?? "").trim();
    if (id && !identities.includes(id)) {
      identities.push(id);
    }
  };
  add(agent.id);
  add(agent.user_id);
  add(resolveAgentChannelUserID(agent));
  agent.participants?.forEach((participant) => {
    add(participant.id);
    add(participant.user_id);
    add(participant.agent_id);
    add(participant.channel_user_ref);
  });
  return identities;
}

function resolveMessageAgent(
  message: IMMessage,
  activity: AgentActivityPayload | null,
  agents: readonly ConversationActivityAgent[],
): ConversationActivityAgent | null {
  const senderIDs = [message.sender_id, activity?.sender];
  return agents.find((agent) => senderIDs.some((senderID) => identityMatches(senderID, agent.identities))) ?? null;
}

function isConversationUserPrompt(
  message: IMMessage,
  activity: AgentActivityPayload | null,
  agent: ConversationActivityAgent | null,
  conversationMemberIDs: readonly string[],
  usersById?: UsersById,
): boolean {
  if (agent || activity || message.kind === "event") {
    return false;
  }
  if (conversationMemberIDs.some((memberID) => localIdentitiesMatch(memberID, message.sender_id))) {
    return true;
  }
  const sender = usersById ? resolveUserByLocalIdentity(message.sender_id, usersById) : undefined;
  return Boolean(
    sender &&
    String(sender.role || "")
      .trim()
      .toLowerCase() !== "worker",
  );
}

function activityMergeKeys(
  agentID: string,
  message: IMMessage,
  activity: AgentActivityPayload | null,
  command: AgentActivityCommand | null,
): string[] {
  const requestScope = activityRequestScope(message);
  const scoped = (values: readonly string[]) =>
    Array.from(new Set(values.filter(Boolean).map((value) => `${agentID}:${requestScope}:${value}`)));
  if (activity?.content.msgtype === AgentActivityMsgTypes.tool) {
    return scoped(agentActivityToolMergeKeys(activity.content.tool));
  }
  if (activity?.content.msgtype === AgentActivityMsgTypes.action) {
    return scoped(activity.content.action?.id ? [`action:${activity.content.action.id}`] : []);
  }
  if (command) {
    return scoped([agentActivityMessageToolMergeKey(message)]);
  }
  return [];
}

function activityRequestScope(message: IMMessage): string {
  const metadata = recordValue(message.metadata);
  const openclaw = recordValue(metadata?.openclaw);
  const codex = recordValue(metadata?.codex);
  return firstNonEmpty(
    openclaw?.request_id,
    openclaw?.requestId,
    openclaw?.source_message_id,
    codex?.request_id,
    codex?.requestId,
    "global",
  );
}

function mergeActivityEntry(
  entry: MutableConversationActivityEntry,
  message: IMMessage,
  activity: AgentActivityPayload | null,
  command: AgentActivityCommand | null,
): void {
  const previousCommand = entry.command;
  if (activity) {
    entry.activity = entry.activity ? mergeActivityPayload(entry.activity, activity) : activity;
    entry.command = null;
    if (previousCommand) {
      mergeCommandIntoActivity(entry, previousCommand);
    }
  }
  if (command) {
    if (entry.activity) {
      mergeCommandIntoActivity(entry, command);
    } else {
      entry.command = mergeActivityCommand(entry.command, command);
    }
  }
  if (activityTime(message.created_at || "") >= activityTime(entry.updatedAt)) {
    entry.message = message;
    entry.updatedAt = message.created_at || entry.updatedAt;
  }
}

function mergeActivityPayload(base: AgentActivityPayload, next: AgentActivityPayload): AgentActivityPayload {
  const baseTool = base.content.tool;
  const nextTool = next.content.tool;
  if (!baseTool || !nextTool) {
    return next;
  }
  const keepTerminalState = isTerminalAgentActivityTool(baseTool) && !isTerminalAgentActivityTool(nextTool);
  return {
    ...base,
    ...next,
    content: {
      ...base.content,
      ...next.content,
      body: firstNonEmpty(next.content.body, base.content.body),
      tool: {
        ...baseTool,
        ...nextTool,
        command: firstNonEmpty(nextTool.command, baseTool.command),
        cwd: firstNonEmpty(nextTool.cwd, baseTool.cwd),
        duration_ms: nextTool.duration_ms ?? baseTool.duration_ms,
        exit_code: nextTool.exit_code ?? baseTool.exit_code,
        input: nextTool.input ?? baseTool.input,
        input_summary: firstNonEmpty(nextTool.input_summary, baseTool.input_summary),
        item_id: firstNonEmpty(nextTool.item_id, baseTool.item_id),
        output: nextTool.output ?? baseTool.output,
        output_summary: firstNonEmpty(nextTool.output_summary, baseTool.output_summary),
        phase: keepTerminalState ? baseTool.phase : firstNonEmpty(nextTool.phase, baseTool.phase),
        status: keepTerminalState ? baseTool.status : firstNonEmpty(nextTool.status, baseTool.status),
        tool_call_id: firstNonEmpty(nextTool.tool_call_id, baseTool.tool_call_id),
      },
    },
  };
}

function mergeCommandIntoActivity(entry: MutableConversationActivityEntry, command: AgentActivityCommand): void {
  const tool = entry.activity?.content.tool;
  if (!entry.activity || !tool) {
    return;
  }
  entry.activity = {
    ...entry.activity,
    content: {
      ...entry.activity.content,
      tool: {
        ...tool,
        command: firstNonEmpty(tool.command, command.command),
        output: command.output || tool.output,
        output_summary: firstNonEmpty(command.output, tool.output_summary),
        phase: command.output ? "end" : tool.phase,
        status: command.output ? "completed" : tool.status,
      },
    },
  };
}

function mergeActivityCommand(base: AgentActivityCommand | null, next: AgentActivityCommand): AgentActivityCommand {
  if (!base) {
    return next;
  }
  return {
    ...base,
    ...next,
    command: firstNonEmpty(base.command, next.command),
    output: firstNonEmpty(next.output, base.output) || undefined,
  };
}

function activityEventType(entry: MutableConversationActivityEntry): string {
  if (entry.source === "user") {
    return AgentActivityKinds.message;
  }
  const activity = entry.activity;
  if (activity?.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    const tool = activity.content.tool;
    if (isFailedTool(tool)) {
      return "error";
    }
    const kind = normalizeEventType(tool.kind || "");
    return ["bash", "command", "exec"].includes(kind) ? AgentActivityKinds.execCommand : kind || "tool_call";
  }
  if (entry.command) {
    return entry.command.kind;
  }
  if (activity?.content.msgtype === AgentActivityMsgTypes.action) {
    return normalizeEventType(activity.content.action?.kind || "") || "action";
  }
  return activity ? "activity" : AgentActivityKinds.message;
}

function activityTone(entry: MutableConversationActivityEntry, eventType: string): ConversationActivityTone {
  if (eventType === "error") {
    return "error";
  }
  if (eventType === AgentActivityKinds.message) {
    return "message";
  }
  if (entry.command || entry.activity?.content.msgtype === AgentActivityMsgTypes.tool) {
    return "tool";
  }
  return "other";
}

function isFailedTool(tool: AgentActivityTool): boolean {
  const status = String(tool.status || "")
    .trim()
    .toLowerCase();
  return (
    ["blocked", "canceled", "cancelled", "error", "failed", "timeout"].includes(status) ||
    (typeof tool.exit_code === "number" && tool.exit_code !== 0)
  );
}

function toolCommandText(tool: AgentActivityTool): string {
  return firstNonEmpty(
    summaryValue(tool.input_summary, "command", "cmd"),
    summaryValue(tool.input, "command", "cmd"),
    tool.command,
  );
}

function toolInputText(tool: AgentActivityTool): string {
  return firstNonEmpty(
    summaryValue(tool.input_summary, "input", "query", "path", "file", "prompt", "arguments", "args"),
    summaryValue(tool.input, "input", "query", "path", "file", "prompt", "arguments", "args"),
    summaryText(tool.input_summary),
    summaryText(tool.input),
  );
}

function toolOutputText(tool: AgentActivityTool): string {
  const output = firstNonEmpty(
    summaryValue(tool.output_summary, "output", "result", "stdout", "stderr", "error"),
    summaryValue(tool.output, "output", "result", "stdout", "stderr", "error"),
    summaryText(tool.output_summary),
    summaryText(tool.output),
  );
  if (output) {
    return output;
  }
  const status = firstNonEmpty(tool.status);
  const exitCode = tool.exit_code === undefined ? "" : `exitCode=${tool.exit_code === null ? "null" : tool.exit_code}`;
  const duration = tool.duration_ms === undefined ? "" : `durationMs=${tool.duration_ms}`;
  return [status && status !== "running" ? `status=${status}` : "", exitCode, duration].filter(Boolean).join("\n");
}

function compactDetails(details: ConversationActivityDetail[]): ConversationActivityDetail[] {
  return details.filter((detail) => detail.value.trim() !== "");
}

function summaryValue(value: unknown, ...keys: string[]): string {
  const decoded = decodeSummary(value);
  const record = recordValue(decoded);
  if (!record) {
    return "";
  }
  for (const key of keys) {
    const text = summaryText(record[key]);
    if (text) {
      return text;
    }
  }
  return "";
}

function summaryText(value: unknown): string {
  const decoded = decodeSummary(value);
  if (typeof decoded === "string") {
    return decoded.trim();
  }
  if (typeof decoded === "number" || typeof decoded === "boolean") {
    return String(decoded);
  }
  if (decoded === null || decoded === undefined) {
    return "";
  }
  try {
    return JSON.stringify(decoded, null, 2);
  } catch {
    return "";
  }
}

function decodeSummary(value: unknown): unknown {
  if (typeof value !== "string") {
    return value;
  }
  const text = value.trim();
  if (!text || (!text.startsWith("{") && !text.startsWith("["))) {
    return text;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

function recordValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function sortMessages(messages: readonly IMMessage[]): IMMessage[] {
  return messages
    .map((message, index) => ({ index, message }))
    .sort((left, right) => {
      const delta = activityTime(left.message.created_at || "") - activityTime(right.message.created_at || "");
      return delta || left.index - right.index;
    })
    .map(({ message }) => message);
}

function messageMergeKey(message: IMMessage): string {
  const id = String(message.id || "").trim();
  return id
    ? `id:${id}`
    : [message.sender_id, message.created_at, message.content, message.relates_to?.event_id].map(String).join(":");
}

function activityEntryID(agentID: string, message: IMMessage, index: number): string {
  return `${agentID}:${message.id || `${message.created_at || "unknown"}:${index}`}`;
}

function cleanMessageBody(content: unknown): string {
  return String(content || "")
    .replace(/\u200b/g, "")
    .trim();
}

function plainText(content: unknown): string {
  return cleanMessageBody(content)
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/!\[[^\]]*]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/[#>*_\-|]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function truncateText(value: string, maxLength: number): string {
  return value.length <= maxLength ? value : `${value.slice(0, maxLength - 1).trimEnd()}…`;
}

function normalizeEventType(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

function identityMatches(value: string | null | undefined, candidates: readonly string[]): boolean {
  const id = String(value || "").trim();
  return Boolean(id && candidates.some((candidate) => candidate === id || localIdentitiesMatch(candidate, id)));
}

function activityTime(value: string): number {
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function firstNonEmpty(...values: unknown[]): string {
  for (const value of values) {
    const text = typeof value === "string" ? value.trim() : value == null ? "" : String(value).trim();
    if (text) {
      return text;
    }
  }
  return "";
}
