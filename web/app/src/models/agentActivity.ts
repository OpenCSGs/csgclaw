import { AgentActivityKinds, AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import type { IMMessage } from "@/models/conversations";

type UnknownRecord = Record<string, unknown>;

export type AgentActivityKind = (typeof AgentActivityKinds)[keyof typeof AgentActivityKinds];

export type AgentActivityCommand = {
  command: string;
  kind: typeof AgentActivityKinds.execCommand | typeof AgentActivityKinds.other;
  output?: string;
  signature: string;
  title: string;
};

export type AgentActivityTool = {
  command?: string;
  cwd?: string;
  duration_ms?: number;
  exit_code?: number | null;
  id: string;
  input?: unknown;
  input_summary?: string;
  item_id?: string;
  kind?: string;
  output?: unknown;
  output_summary?: string;
  phase?: string;
  status: string;
  title: string;
  tool_call_id?: string;
};

export type AgentActivityActionOption = {
  id: string;
  kind: string;
  label: string;
  scope?: string;
};

export type AgentActivityActionDecision = {
  decided_at?: string;
  kind?: string;
  option_id?: string;
};

export type AgentActivityAction = {
  decision?: AgentActivityActionDecision | null;
  expires_at?: string;
  id: string;
  kind?: string;
  options?: AgentActivityActionOption[];
  requested_at?: string;
  status: string;
  title: string;
};

export type AgentActivityContent = {
  action?: AgentActivityAction;
  body: string;
  msgtype: string;
  tool?: AgentActivityTool;
};

export type AgentActivityPayload = {
  channel: string;
  content: AgentActivityContent;
  event_id: string;
  origin_server_ts: number;
  room_id: string;
  sender: string;
  type: typeof CSGCLAW_AGENT_ACTIVITY_TYPE;
  version: number;
};

export function parseAgentActivity(content: unknown): AgentActivityPayload | null {
  const parsed = typeof content === "string" ? parseJSON(content.trim()) : content;
  if (!isRecord(parsed) || parsed.type !== CSGCLAW_AGENT_ACTIVITY_TYPE || !isRecord(parsed.content)) {
    return null;
  }

  const activityContent = parsed.content;
  const msgtype = stringValue(activityContent.msgtype);
  if (
    !Object.values(AgentActivityMsgTypes).includes(
      msgtype as (typeof AgentActivityMsgTypes)[keyof typeof AgentActivityMsgTypes],
    )
  ) {
    return null;
  }

  return {
    content: {
      action: parseAction(activityContent.action),
      body: stringValue(activityContent.body, "Agent activity"),
      msgtype,
      tool: parseTool(activityContent.tool),
    },
    channel: stringValue(parsed.channel),
    event_id: stringValue(parsed.event_id),
    origin_server_ts: numberValue(parsed.origin_server_ts),
    room_id: stringValue(parsed.room_id),
    sender: stringValue(parsed.sender),
    type: CSGCLAW_AGENT_ACTIVITY_TYPE,
    version: numberValue(parsed.version, 1),
  };
}

export function isToolActivityMessage(message: IMMessage | null | undefined): boolean {
  const activity = parseAgentActivity(message?.content);
  return activity?.content.msgtype === AgentActivityMsgTypes.tool;
}

export function openClawDeliveryKind(message: IMMessage | null | undefined): string {
  const openclaw = openClawMetadata(message);
  const deliveryInfo = openClawDeliveryInfo(openclaw);
  return stringValue(openclaw?.delivery_kind, openclaw?.deliveryKind, openclaw?.kind, deliveryInfo?.kind).toLowerCase();
}

export function isOpenClawToolDeliveryMessage(message: IMMessage | null | undefined): boolean {
  return openClawDeliveryKind(message) === "tool";
}

export function openClawToolCallID(message: IMMessage | null | undefined): string {
  const openclaw = openClawMetadata(message);
  const deliveryInfo = openClawDeliveryInfo(openclaw);
  return stringValue(
    openclaw?.tool_call_id,
    openclaw?.toolCallId,
    deliveryInfo?.toolCallId,
    deliveryInfo?.tool_call_id,
  );
}

export function parseOpenClawDeliveryCommand(message: IMMessage | null | undefined): AgentActivityCommand | null {
  if (!isOpenClawToolDeliveryMessage(message)) {
    return null;
  }

  const parsed = parsePlainAgentCommand(message?.content);
  if (parsed) {
    return {
      ...parsed,
      signature: openClawCommandSignature(message, parsed.command),
    };
  }

  const text = String(message?.content ?? "")
    .replace(/\u200b/g, "")
    .trim();
  const split = splitExplicitToolCommandOutput(text);
  const command = split.command.trim();
  if (!command) {
    return null;
  }
  const firstLine = command.split(/\r?\n/, 1)[0]?.trim() || command;
  return {
    command,
    kind: AgentActivityKinds.execCommand,
    output: split.output,
    signature: openClawCommandSignature(message, command),
    title: plainCommandTitle(firstLine),
  };
}

export function parseMessageActivityCommand(message: IMMessage | null | undefined): AgentActivityCommand | null {
  const openClawCommand = parseOpenClawDeliveryCommand(message);
  if (openClawCommand) {
    return openClawCommand;
  }
  if (openClawDeliveryKind(message)) {
    return null;
  }
  return parseLegacyToolActivityCommand(message?.content);
}

export function parsePlainAgentCommand(content: unknown): AgentActivityCommand | null {
  if (typeof content !== "string") {
    return null;
  }

  const text = content.replace(/\u200b/g, "").trim();
  if (!text) {
    return null;
  }

  const legacyTool = parseLegacyFencedCommand(text);
  if (legacyTool) {
    return legacyTool;
  }

  const marked = stripToolMarker(text);
  if (!marked.command) {
    return null;
  }

  const split = splitPlainCommandOutput(marked.command);
  const command = split.command.trim();
  if (!command) {
    return null;
  }

  const title = plainCommandTitle(command);
  const kind = plainCommandKind(command, marked.hadMarker);
  if (!kind) {
    return null;
  }

  return {
    command,
    kind,
    output: split.output,
    signature: commandSignature(command),
    title,
  };
}

export function parseLegacyToolActivityCommand(content: unknown): AgentActivityCommand | null {
  if (typeof content !== "string") {
    return null;
  }

  const text = content.replace(/\u200b/g, "").trim();
  if (!text) {
    return null;
  }

  return parseLegacyFencedCommand(text);
}

export function actionOptionLabel(option: AgentActivityActionOption): string {
  const label = stringValue(option.label, option.kind, option.id);
  if (optionScope(option) === "agent" && !/\bagent\b/i.test(label)) {
    return `${label} (this agent)`;
  }
  return label;
}

export function statusLabel(status: string): string {
  switch (status) {
    case "allowed":
      return "Allowed";
    case "rejected":
      return "Rejected";
    case "expired":
      return "Expired";
    case "canceled":
      return "Canceled";
    case "completed":
      return "Completed";
    case "failed":
      return "Failed";
    case "running":
      return "Running";
    case "pending":
      return "Pending";
    default:
      return stringValue(status, "Status");
  }
}

export function agentActivityToolMergeKey(tool: AgentActivityTool | null | undefined): string {
  return firstNonEmpty(tool?.id, tool?.item_id, tool?.tool_call_id);
}

export function isTerminalAgentActivityTool(tool: AgentActivityTool | null | undefined): boolean {
  const phase = normalizeActivityStatus(tool?.phase);
  const status = normalizeActivityStatus(tool?.status);
  return (
    phase === "end" ||
    status === "completed" ||
    status === "done" ||
    status === "failed" ||
    status === "blocked" ||
    status === "canceled" ||
    status === "cancelled" ||
    status === "success" ||
    status === "succeeded"
  );
}

function parseTool(value: unknown): AgentActivityTool | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    command: stringValue(value.command, value.cmd),
    cwd: stringValue(value.cwd),
    duration_ms: numberOrUndefined(value.duration_ms, value.durationMs),
    exit_code: numberOrNull(value.exit_code, value.exitCode),
    id: stringValue(value.id),
    input: firstDefined(value.input, value.arguments, value.args),
    input_summary: stringValue(value.input_summary),
    item_id: stringValue(value.item_id, value.itemId),
    kind: stringValue(value.kind),
    output: firstDefined(value.output, value.result),
    output_summary: stringValue(value.output_summary),
    phase: stringValue(value.phase),
    status: stringValue(value.status, "running"),
    title: stringValue(value.title, "Run tool"),
    tool_call_id: stringValue(value.tool_call_id, value.toolCallId),
  };
}

function parseAction(value: unknown): AgentActivityAction | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    decision: parseDecision(value.decision),
    expires_at: stringValue(value.expires_at),
    id: stringValue(value.id),
    kind: stringValue(value.kind, "permission"),
    options: Array.isArray(value.options) ? value.options.map(parseOption).filter(isActionOption) : [],
    requested_at: stringValue(value.requested_at),
    status: stringValue(value.status, "pending"),
    title: stringValue(value.title, "Run tool"),
  };
}

function parseOption(value: unknown): AgentActivityActionOption | null {
  if (!isRecord(value)) {
    return null;
  }
  const id = stringValue(value.id);
  if (!id) {
    return null;
  }
  return {
    id,
    kind: stringValue(value.kind),
    label: stringValue(value.label, value.kind, id),
    scope: stringValue(value.scope, defaultOptionScope(stringValue(value.kind))) || undefined,
  };
}

function isActionOption(value: AgentActivityActionOption | null): value is AgentActivityActionOption {
  return value !== null;
}

function parseDecision(value: unknown): AgentActivityActionDecision | null {
  if (!isRecord(value)) {
    return null;
  }
  return {
    decided_at: stringValue(value.decided_at),
    kind: stringValue(value.kind),
    option_id: stringValue(value.option_id),
  };
}

function parseJSON(input: string): unknown {
  if (!input.startsWith("{")) {
    return null;
  }
  try {
    return JSON.parse(input);
  } catch {
    return null;
  }
}

function optionScope(option: AgentActivityActionOption): string {
  return stringValue(option.scope, defaultOptionScope(option.kind));
}

function defaultOptionScope(kind: string): string {
  return kind === "allow_always" ? "agent" : "";
}

function isRecord(value: unknown): value is UnknownRecord {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function recordValue(value: unknown): UnknownRecord | null {
  return isRecord(value) ? value : null;
}

function openClawMetadata(message: IMMessage | null | undefined): UnknownRecord | null {
  const metadata = recordValue(message?.metadata);
  return recordValue(metadata?.openclaw) ?? metadata;
}

function openClawDeliveryInfo(openclaw: UnknownRecord | null): UnknownRecord | null {
  return recordValue(openclaw?.delivery_info) ?? recordValue(openclaw?.deliveryInfo);
}

function stringValue(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return "";
}

function firstDefined(...values: unknown[]): unknown {
  for (const value of values) {
    if (value !== undefined && value !== null) {
      return value;
    }
  }
  return undefined;
}

function numberValue(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function numberOrUndefined(...values: unknown[]): number | undefined {
  for (const value of values) {
    if (typeof value === "number" && Number.isFinite(value)) {
      return value;
    }
  }
  return undefined;
}

function numberOrNull(...values: unknown[]): number | null | undefined {
  for (const value of values) {
    if (value === null) {
      return null;
    }
    if (typeof value === "number" && Number.isFinite(value)) {
      return value;
    }
  }
  return undefined;
}

function normalizeActivityStatus(value: unknown): string {
  return typeof value === "string" ? value.replace(/\s+/g, " ").trim().toLowerCase() : "";
}

function parseLegacyFencedCommand(text: string): AgentActivityCommand | null {
  if (!text.startsWith("🔧 ")) {
    return null;
  }

  const firstLineBreak = text.indexOf("\n");
  const firstLine = (firstLineBreak >= 0 ? text.slice(0, firstLineBreak) : text).replace(/^🔧\s*/, "").trim();
  const body = legacyFencedPayload(text.slice(firstLineBreak >= 0 ? firstLineBreak + 1 : text.length));
  const decoded = decodeSummary(body);
  const command = firstNonEmpty(
    summaryValue(decoded, "command", "cmd"),
    summaryValue(decoded, "input", "query", "path", "file", "filename", "pattern", "description", "prompt"),
    summaryText(decoded),
  );
  if (!command) {
    return null;
  }

  const output = firstNonEmpty(
    summaryValue(decoded, "output", "result", "stdout", "stderr", "error"),
    typeof decoded === "string" && decoded !== command ? decoded : "",
  );

  return {
    command,
    kind: AgentActivityKinds.execCommand,
    output,
    signature: commandSignature(command || firstLine),
    title: firstLine.replace(/`/g, "").trim() || "exec_command",
  };
}

function stripToolMarker(text: string): { command: string; hadMarker: boolean } {
  const command = text.replace(/^(?:🔧|🛠️?|🔎|📄|📖|🧠|✍️?|🩹|📩)\s*/u, "").trim();
  return { command, hadMarker: command !== text };
}

function splitTrailingStructuredOutput(text: string): { command: string; output?: string } {
  for (let index = 1; index < text.length; index += 1) {
    const char = text[index];
    if ((char !== "{" && char !== "[") || !/\s/.test(text[index - 1] || "")) {
      continue;
    }
    const before = text.slice(0, index).trim();
    const after = text.slice(index).trim();
    if (!before || !after) {
      continue;
    }
    try {
      const parsed = JSON.parse(after);
      return {
        command: before,
        output: JSON.stringify(parsed, null, 2),
      };
    } catch {
      // Keep scanning; a command argument can contain a non-result brace.
    }
  }
  return { command: text.trim() };
}

function splitPlainCommandOutput(text: string): { command: string; output?: string } {
  const structured = splitTrailingStructuredOutput(text);
  if (structured.output) {
    return structured;
  }
  return splitKnownPlainOutput(structured.command);
}

function splitExplicitToolCommandOutput(text: string): { command: string; output?: string } {
  const split = splitPlainCommandOutput(text);
  if (split.output || !text.includes("\n")) {
    return split;
  }
  const newline = text.search(/\r?\n/);
  if (newline <= 0) {
    return split;
  }
  const command = text.slice(0, newline).trim();
  const output = text.slice(newline).trim();
  return command && output ? { command, output } : split;
}

function splitKnownPlainOutput(text: string): { command: string; output?: string } {
  const command = text.trim();
  const firstLine = splitFirstLineCommandOutput(command);
  if (firstLine) {
    return firstLine;
  }

  const inlineScript = splitInlineScriptCommandPrefix(command);
  if (inlineScript) {
    return inlineScript;
  }

  const markers = [" Traceback (most recent call last):", " Error:", " Exception:"];
  for (const marker of markers) {
    const idx = command.indexOf(marker);
    if (idx <= 0) {
      continue;
    }
    const before = command.slice(0, idx).trim();
    const output = command.slice(idx).trim();
    if (before && output && isKnownPlainCommand(before)) {
      return { command: before, output };
    }
  }

  return { command };
}

function splitFirstLineCommandOutput(text: string): { command: string; output?: string } | null {
  const newline = text.search(/\r?\n/);
  if (newline <= 0) {
    return null;
  }
  const command = text.slice(0, newline).trim();
  const output = text.slice(newline).trim();
  if (!command || !output || !isKnownPlainCommand(command)) {
    return null;
  }
  return { command, output };
}

function splitInlineScriptCommandPrefix(text: string): { command: string; output?: string } | null {
  const match = text.match(/^run\s+(node|python|python3|ruby|php)\s+inline script(?: \(heredoc\))?/i);
  if (!match?.[0]) {
    return null;
  }
  return splitFixedCommandPrefix(text, match[0]);
}

function splitFixedCommandPrefix(text: string, prefix: string): { command: string; output?: string } | null {
  if (commandSignature(text) === prefix) {
    return { command: prefix };
  }
  const lower = text.toLowerCase();
  if (!lower.startsWith(prefix) || !/\s/.test(text[prefix.length] || "")) {
    return null;
  }
  const output = text.slice(prefix.length).trim();
  return output ? { command: prefix, output } : { command: prefix };
}

function plainCommandKind(command: string, hadMarker: boolean): AgentActivityCommand["kind"] | null {
  if (isPlainMessageReceipt(command)) {
    return AgentActivityKinds.other;
  }
  if (hadMarker || isKnownPlainCommand(command)) {
    return AgentActivityKinds.execCommand;
  }
  return null;
}

function isPlainMessageReceipt(command: string): boolean {
  return commandSignature(command) === "message";
}

function openClawCommandSignature(message: IMMessage | null | undefined, command: string): string {
  const toolCallID = openClawToolCallID(message);
  return toolCallID ? `openclaw-tool:${toolCallID}` : commandSignature(command);
}

function isKnownPlainCommand(command: string): boolean {
  const normalized = command.toLowerCase();
  return (
    normalized.startsWith("web search:") ||
    normalized.startsWith("web fetch:") ||
    normalized.startsWith("read:") ||
    normalized.startsWith("memory search:") ||
    normalized.startsWith("write:") ||
    normalized.startsWith("apply patch:") ||
    normalized.startsWith("edit:") ||
    normalized.startsWith("run cd ") ||
    /^run\s+(node|python|python3|ruby|php)\s+\S+/.test(normalized) ||
    /^run\s+(node|python|python3|ruby|php)\s+inline script(?: \(heredoc\))?$/.test(normalized)
  );
}

function plainCommandTitle(command: string): string {
  const colon = command.indexOf(":");
  if (colon > 0 && colon <= 32) {
    return command.slice(0, colon).trim();
  }
  const words = command.split(/\s+/).slice(0, 2).join(" ").trim();
  return words || "exec_command";
}

function commandSignature(command: string): string {
  return command.replace(/\s+/g, " ").trim().toLowerCase();
}

function legacyFencedPayload(body: string): string {
  const sections: string[] = [];
  const fencePattern = /```[a-zA-Z0-9_-]*\s*\n?([\s\S]*?)```/g;
  for (const match of body.matchAll(fencePattern)) {
    const value = String(match[1] || "").trim();
    if (isJSONLikeText(value)) {
      sections.push(value);
    }
  }
  if (sections.length) {
    return sections.join("\n\n");
  }
  const trimmed = body.trim();
  return isJSONLikeText(trimmed) ? trimmed : "";
}

function isJSONLikeText(value: string): boolean {
  const text = value.trim();
  return text.startsWith("{") || text.startsWith("[");
}

function summaryValue(value: unknown, ...keys: string[]): string {
  const decoded = decodeSummary(value);
  if (!isRecord(decoded)) {
    return "";
  }
  for (const key of keys) {
    const text = summaryText(decoded[key]);
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
  if (!text) {
    return "";
  }
  if (!text.startsWith("{") && !text.startsWith("[")) {
    return text;
  }
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function firstNonEmpty(...values: unknown[]): string {
  for (const value of values) {
    const text = typeof value === "string" ? value.trim() : summaryText(value);
    if (text) {
      return text;
    }
  }
  return "";
}
