import {
  ACTION_REBUILD_MANAGER,
  CSGCLAW_ACTION_CARD_TYPE,
  CSGCLAW_NOTIFY_CARD_TYPE,
} from "@/shared/constants/messages";
import type { ActionCardPayload, MessageAction, ParsedStructuredMessage, StructuredMessagePayload } from "./types";

type UnknownRecord = Record<string, unknown>;

export function parseStructuredMessage(content: unknown): ParsedStructuredMessage | null {
  const cleaned = String(content ?? "").trim();
  if (!cleaned) {
    return null;
  }
  if (isLegacyToolFeedback(cleaned)) {
    return null;
  }

  const fencedJSON = cleaned.match(/^```(?:json|javascript|js)?\s*([\s\S]+?)\s*```$/i);
  const rawJSON = fencedJSON ? fencedJSON[1].trim() : cleaned;
  const fromPrimary = structuredPayloadFromParsed(tryParseJSON(rawJSON));
  if (fromPrimary) {
    return fromPrimary;
  }

  const extracted = extractTopLevelJSONObject(cleaned);
  const fromExtracted = structuredEnvelopePayloadFromParsed(tryParseJSON(extracted));
  if (fromExtracted) {
    return fromExtracted;
  }

  const codeBlock = extractSingleLargeCodeBlock(cleaned);
  if (codeBlock) {
    return buildCodeBlockPayload(codeBlock);
  }

  return null;
}

export function structuredPayloadFromParsed(parsed: unknown): ParsedStructuredMessage | null {
  if (!parsed) {
    return null;
  }
  const envelope = structuredEnvelopePayloadFromParsed(parsed);
  if (envelope) {
    return envelope;
  }
  if (isStructuredPayload(parsed)) {
    return buildStructuredPayload(parsed);
  }
  return null;
}

function structuredEnvelopePayloadFromParsed(parsed: unknown): ParsedStructuredMessage | null {
  if (!parsed) {
    return null;
  }
  if (isNotifyCardPayload(parsed)) {
    return buildNotifyCardPayload(parsed);
  }
  if (isActionCardPayload(parsed)) {
    return buildActionCardPayload(parsed);
  }
  return null;
}

function isLegacyToolFeedback(content: string): boolean {
  return content.trimStart().startsWith("🔧 ");
}

export function tryParseJSON(input: string | null): unknown | null {
  if (!input || (!input.startsWith("{") && !input.startsWith("["))) {
    return null;
  }
  try {
    return JSON.parse(input);
  } catch {
    return null;
  }
}

export function extractTopLevelJSONObject(input: string): string | null {
  if (!input) {
    return null;
  }
  const firstBrace = input.indexOf("{");
  if (firstBrace < 0) {
    return null;
  }

  let depth = 0;
  let inString = false;
  let escaped = false;
  for (let index = firstBrace; index < input.length; index += 1) {
    const char = input[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (inString) {
      if (char === "\\") {
        escaped = true;
        continue;
      }
      if (char === '"') {
        inString = false;
      }
      continue;
    }
    if (char === '"') {
      inString = true;
      continue;
    }
    if (char === "{") {
      depth += 1;
      continue;
    }
    if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        return input.slice(firstBrace, index + 1);
      }
    }
  }

  return null;
}

export function isActionCardPayload(value: unknown): value is UnknownRecord & { actions: unknown[] } {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const payload = value as UnknownRecord;
  return payload.type === CSGCLAW_ACTION_CARD_TYPE && Array.isArray(payload.actions);
}

export function isNotifyCardPayload(value: unknown): value is UnknownRecord {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  return (value as UnknownRecord).type === CSGCLAW_NOTIFY_CARD_TYPE;
}

export function isSafeHttpURL(url: unknown): boolean {
  try {
    const parsed = new URL(String(url ?? ""));
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function buildNotifyCardPayload(value: UnknownRecord): StructuredMessagePayload {
  const meta = Array.isArray(value.meta)
    ? value.meta
        .filter((row): row is UnknownRecord => Boolean(row && typeof row === "object" && !Array.isArray(row)))
        .map((row) => ({
          label: String(row.label ?? "").trim(),
          value: String(row.value ?? "").trim(),
        }))
        .filter((row) => row.label || row.value)
    : [];
  const raw = typeof value.raw === "string" ? value.raw.trim() : "";
  return {
    badge: firstNonEmptyString(value.badge),
    code: "",
    codeSummary: "",
    link: firstNonEmptyString(value.link),
    meta,
    payload: raw,
    payloadSummary: raw ? "查看原始 JSON" : "",
    subtitle: firstNonEmptyString(value.subtitle),
    summary: firstNonEmptyString(value.summary),
    title: firstNonEmptyString(value.title, "Notification"),
  };
}

export function buildActionCardPayload(value: UnknownRecord & { actions: unknown[] }): ActionCardPayload {
  return {
    actions: normalizeActionCardActions(value.actions),
    badge: firstNonEmptyString(value.badge, value.status),
    fallback: firstNonEmptyString(value.fallback),
    kind: "action_card",
    subtitle: firstNonEmptyString(value.subtitle, value.bot_id),
    summary: firstNonEmptyString(value.summary, value.message, value.description),
    title: firstNonEmptyString(value.title, value.name, "Action required"),
  };
}

export function normalizeActionCardActions(actions: unknown[]): MessageAction[] {
  return (actions ?? [])
    .filter((action): action is UnknownRecord =>
      Boolean(action && typeof action === "object" && !Array.isArray(action)),
    )
    .filter((action) => action.id === ACTION_REBUILD_MANAGER)
    .slice(0, 1)
    .map((action) => ({
      confirm: firstNonEmptyString(action.confirm),
      id: ACTION_REBUILD_MANAGER,
      label: firstNonEmptyString(action.label, "重建 Manager"),
      style: action.style === "danger" ? "danger" : "default",
    }));
}

export function isStructuredPayload(value: unknown): value is UnknownRecord {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const payload = value as UnknownRecord;
  if (payload.type === CSGCLAW_NOTIFY_CARD_TYPE || payload.type === CSGCLAW_ACTION_CARD_TYPE) {
    return false;
  }
  const keys = Object.keys(value);
  return keys.some((key) =>
    ["tool", "name", "arguments", "input", "file", "path", "code", "content", "status", "action"].includes(key),
  );
}

export function buildStructuredPayload(value: UnknownRecord): StructuredMessagePayload {
  const title = String(value.tool || value.name || value.action || "Structured output");
  const target = firstNonEmptyString(value.file, value.path, value.file_path, value.filename);
  const code = findLargeCodeString(value);

  return {
    badge: inferPayloadBadge(value),
    code,
    codeSummary: code ? summarizeCode(code) : "",
    payload: JSON.stringify(value, null, 2),
    payloadSummary: `查看原始 JSON · ${Object.keys(value).length} 个字段`,
    subtitle: target && title !== target ? target : "",
    summary: summarizeStructuredValue(value, code),
    title,
  };
}

export function buildCodeBlockPayload(codeBlock: { code: string; language?: string }): StructuredMessagePayload {
  const lineCount = codeBlock.code.split("\n").length;
  return {
    badge: lineCount > 80 ? "Long output" : "Code",
    code: codeBlock.code,
    codeSummary: `展开代码 · ${lineCount} 行`,
    payload: "",
    payloadSummary: "",
    subtitle: codeBlock.language ? codeBlock.language.toUpperCase() : "Plain text",
    summary: `检测到 ${lineCount} 行代码，默认折叠以避免聊天流被长内容撑开。`,
    title: "Long code block",
  };
}

export function extractSingleLargeCodeBlock(content: string): { code: string; language?: string } | null {
  const match = content.match(/^```([\w-]+)?\n([\s\S]+?)\n```$/);
  if (!match) {
    return null;
  }
  const code = match[2];
  if (code.length < 600 && code.split("\n").length < 18) {
    return null;
  }
  return {
    code,
    language: match[1] || "",
  };
}

export function findLargeCodeString(value: unknown, seen = new Set<unknown>()): string {
  if (!value || typeof value !== "object" || seen.has(value)) {
    return "";
  }
  seen.add(value);
  const record = value as UnknownRecord;

  for (const key of ["code", "content", "text", "body", "source"]) {
    if (typeof record[key] === "string" && looksLikeCode(record[key])) {
      return record[key];
    }
  }

  for (const item of Object.values(record)) {
    if (typeof item === "string" && looksLikeCode(item)) {
      return item;
    }
    if (item && typeof item === "object") {
      const nested = findLargeCodeString(item, seen);
      if (nested) {
        return nested;
      }
    }
  }

  return "";
}

export function looksLikeCode(text: unknown): text is string {
  if (typeof text !== "string") {
    return false;
  }
  const trimmed = text.trim();
  if (trimmed.length < 180) {
    return false;
  }
  return /[{};<>]/.test(trimmed) || trimmed.includes("\n");
}

export function summarizeStructuredValue(value: UnknownRecord, code: string): string {
  const parts: string[] = [];
  const args = value.arguments || value.input || value.params;
  if (args && typeof args === "object" && !Array.isArray(args)) {
    const interestingKeys = Object.keys(args).slice(0, 3);
    if (interestingKeys.length > 0) {
      parts.push(`参数: ${interestingKeys.join(", ")}`);
    }
  }
  if (code) {
    parts.push(`代码: ${summarizeCode(code)}`);
  }
  return parts.join(" · ") || "已识别为结构化工具输出，默认折叠原始内容。";
}

export function summarizeCode(code: string): string {
  const lines = code.split("\n").length;
  const chars = code.length;
  return `${lines} 行 / ${chars} 字符`;
}

export function inferPayloadBadge(value: UnknownRecord): string {
  if (typeof value.status === "string" && value.status.trim()) {
    return value.status.trim();
  }
  if (typeof value.tool === "string" && value.tool.trim()) {
    return "Tool";
  }
  return "JSON";
}

export function firstNonEmptyString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return "";
}
