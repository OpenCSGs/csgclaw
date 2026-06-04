import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import type { IMMessage } from "@/models/conversations";

type UnknownRecord = Record<string, unknown>;

export type AgentActivityTool = {
  id: string;
  input_summary?: string;
  kind?: string;
  output_summary?: string;
  status: string;
  title: string;
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

function parseTool(value: unknown): AgentActivityTool | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    id: stringValue(value.id),
    input_summary: stringValue(value.input_summary),
    kind: stringValue(value.kind),
    output_summary: stringValue(value.output_summary),
    status: stringValue(value.status, "running"),
    title: stringValue(value.title, "Run tool"),
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

function stringValue(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return "";
}

function numberValue(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}
