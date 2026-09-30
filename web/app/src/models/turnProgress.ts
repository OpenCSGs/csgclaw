import type { IMMessage, TranslateFn } from "@/models/conversations";

export type ProgressTool = {
  name: string;
  actions: string[];
  status: string;
  input?: string;
  output?: string;
  command?: string;
  cwd?: string;
  exit_code?: number;
  duration_ms?: number;
  truncated?: boolean;
};
export type ProgressItem = {
  id: string;
  kind: "commentary" | "reasoning" | "tool";
  text?: string;
  tool?: ProgressTool;
};
export type TurnProgress = {
  id: string;
  revision: number;
  status: string;
  started_at: string;
  updated_at: string;
  ended_at?: string;
  items: ProgressItem[];
  error?: string;
};
function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
export function parseTurnProgress(message: unknown): TurnProgress | null {
  const metadata = record(record(message)?.metadata);
  const progress = record(record(metadata?.csgclaw)?.turn_progress);
  if (
    !progress ||
    typeof progress.id !== "string" ||
    typeof progress.revision !== "number" ||
    typeof progress.status !== "string" ||
    typeof progress.started_at !== "string" ||
    !Array.isArray(progress.items) ||
    !progress.items.every((item) => {
      const value = record(item);
      if (!value || typeof value.id !== "string" || !["commentary", "reasoning", "tool"].includes(String(value.kind)))
        return false;
      if (value.kind !== "tool") return value.text === undefined || typeof value.text === "string";
      const tool = record(value.tool);
      return (
        tool &&
        typeof tool.name === "string" &&
        typeof tool.status === "string" &&
        Array.isArray(tool.actions) &&
        tool.actions.every((action) => typeof action === "string")
      );
    })
  )
    return null;
  return progress as TurnProgress;
}
export function newerProgressMessage<T>(previous: T, incoming: T): T {
  const old = parseTurnProgress(previous);
  const next = parseTurnProgress(incoming);
  return old && next && old.id === next.id && old.revision >= next.revision ? previous : incoming;
}
export function progressActive(progress: TurnProgress): boolean {
  return progress.status === "running" || progress.status === "waiting";
}
export function progressDuration(progress: TurnProgress, now: number): string {
  const start = Date.parse(progress.started_at);
  const end = Date.parse(progress.ended_at || "");
  const seconds = Math.max(0, Math.floor(((Number.isFinite(end) ? end : now) - start) / 1000)) || 0;
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
export type ProgressGroup = { id: string; items: ProgressItem[]; kind: ProgressItem["kind"]; firstToolNumber: number };
export function progressGroups(items: ProgressItem[]): ProgressGroup[] {
  const result: ProgressGroup[] = [];
  let toolCount = 0;
  for (const item of items) {
    const last = result[result.length - 1];
    if (item.kind === "tool" && last?.kind === "tool") last.items.push(item);
    else result.push({ id: `${item.kind}:${item.id}`, kind: item.kind, items: [item], firstToolNumber: toolCount + 1 });
    if (item.kind === "tool") toolCount += 1;
  }
  return result;
}
export function toolGroupLabel(items: ProgressItem[], t: TranslateFn): string {
  const actions = [...new Set(items.flatMap((item) => item.tool?.actions || ["tool"]))];
  const active = items.some((item) => !terminalTool(item.tool?.status));
  const failed = items.some((item) => ["failed", "error", "declined"].includes(item.tool?.status || ""));
  const state = active
    ? "running"
    : failed
      ? "failed"
      : items.some((item) => item.tool?.status === "interrupted")
        ? "interrupted"
        : "completed";
  const known = new Set(["read", "search", "list", "edit", "execute", "web", "load", "tool"]);
  const label = actions
    .map((action) => t(`progressAction_${known.has(action) ? action : "tool"}`))
    .join(t("progressSeparator"));
  return t(`progressTools_${state}`, { actions: label, count: items.length });
}
export function terminalTool(status?: string): boolean {
  return [
    "completed",
    "complete",
    "success",
    "succeeded",
    "failed",
    "error",
    "canceled",
    "cancelled",
    "declined",
    "interrupted",
  ].includes(status || "");
}

// The activity inspector reads the same turn projection as the transcript.
export function expandProgressMessage(message: IMMessage): IMMessage[] {
  const progress = parseTurnProgress(message);
  if (!progress) return [message];
  const entries: IMMessage[] = progress.items.flatMap((item, index) => {
    if (item.kind === "reasoning") return [];
    const id = `${message.id}:${index}:${item.id}`;
    const tool = item.tool;
    const activity = tool
      ? {
          type: "com.opencsg.csgclaw.agent.activity",
          version: 1,
          channel: "csgclaw",
          room_id: "",
          sender: message.sender_id || "",
          event_id: id,
          origin_server_ts: Date.parse(progress.started_at),
          content: {
            msgtype: "com.opencsg.csgclaw.agent.tool",
            body: tool.name,
            tool: {
              id,
              kind: tool.actions[0] || "tool",
              title: tool.name,
              status: tool.status,
              command: tool.command,
              cwd: tool.cwd,
              input_summary: tool.input,
              output_summary: tool.output,
              duration_ms: tool.duration_ms,
              exit_code: tool.exit_code,
            },
          },
        }
      : null;
    return {
      ...message,
      id,
      content: item.text || tool?.name || "",
      attachments: [],
      metadata: {
        csgclaw: activity ? { agent_activity: activity, delivery_kind: "tool" } : { delivery_kind: "activity" },
      },
    };
  });
  if (message.content?.replace(/\u200b/g, "").trim() || progress.error)
    entries.push({
      ...message,
      content: message.content?.replace(/\u200b/g, "").trim() ? message.content : progress.error || "",
    });
  return entries;
}

export function emptyCompletedProgress(message: IMMessage | null | undefined): boolean {
  if (!message) return false;
  const progress = parseTurnProgress(message);
  return (
    progress?.status === "succeeded" &&
    progress.items.length === 0 &&
    !message.content?.replace(/\u200b/g, "").trim() &&
    !message.attachments?.length
  );
}
