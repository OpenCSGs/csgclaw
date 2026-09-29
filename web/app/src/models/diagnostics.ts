import type { TranslateFn } from "./conversations";
import { parseTurnProgress, progressActive } from "./turnProgress";
import type { DiagnosticSpan, TurnDiagnostic } from "@/api/diagnostics";

export function diagnosticDuration(ms: number | undefined): string {
  if (ms === undefined || !Number.isFinite(ms)) return "N/A";
  if (ms === 0) return "0 ms";
  if (ms < 1) return "< 1 ms";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(2)} s`;
  return `${Math.floor(ms / 60000)} min ${((ms % 60000) / 1000).toFixed(1)} s`;
}
export function diagnosticPhases(record: TurnDiagnostic) {
  const total = Math.max(0, record.total_ms);
  const start = record.runtime_start_ms;
  const end = record.runtime_end_ms;
  return [
    { id: "before", name: "diagBefore", start: 0, end: start ?? total },
    { id: "runtime", name: "diagRuntime", start, end: start === undefined ? undefined : (end ?? total) },
    { id: "after", name: "diagAfter", start: end, end: end === undefined ? undefined : total },
  ];
}
// Nested and concurrent spans are wall-clock intervals, never additive costs.
export function diagnosticOccupied(spans: DiagnosticSpan[], owner: string, total: number): number {
  return diagnosticIntervalUnion(
    spans.filter((span) => span.owner === owner),
    total,
  );
}
function diagnosticIntervalUnion(spans: DiagnosticSpan[], total: number): number {
  const intervals = spans
    .map((span) => [Math.max(0, span.start_ms), Math.min(total, span.end_ms ?? total)])
    .sort((a, b) => a[0] - b[0]);
  let result = 0;
  let end = 0;
  for (const [start, next] of intervals) {
    if (next > end) result += Math.max(0, next - Math.max(start, end));
    end = Math.max(end, next);
  }
  return result;
}
export function diagnosticReference(metadata: Record<string, unknown> | null | undefined) {
  const value = metadata?.diagnostics;
  if (!value || typeof value !== "object") return null;
  const ref = value as Record<string, unknown>;
  if (typeof ref.source_id !== "string" || typeof ref.room_id !== "string") return null;
  return { source: ref.source_id, room: ref.room_id, turn: typeof ref.turn_id === "string" ? ref.turn_id : undefined };
}

// Streamed progress messages retain delivery_kind=activity on failure or stop.
// Use their authoritative lifecycle rather than treating only text replies as final.
export function diagnosticFinal(metadata: Record<string, unknown> | null | undefined): boolean {
  const progress = parseTurnProgress({ metadata });
  if (progress) return !progressActive(progress);
  const runtime = (metadata?.codex ?? metadata?.openclaw) as Record<string, unknown> | undefined;
  return runtime?.delivery_kind === "final";
}

export type DiagnosticBucket = { key: string; duration: number; percent: number };

// Partition the wall-clock axis. Parallel owners receive their own bucket;
// neither nested spans nor overlapped model/tool work are counted twice.
export function diagnosticBreakdown(record: TurnDiagnostic): DiagnosticBucket[] {
  const total = Math.max(0, record.total_ms);
  const start = Math.max(0, Math.min(total, record.runtime_start_ms ?? total));
  const end = Math.max(start, Math.min(total, record.runtime_end_ms ?? total));
  const sums: Record<string, number> = { before: start, after: total - end };
  const edges: { time: number; owner: string; delta: number }[] = [];
  for (const span of record.spans ?? []) {
    const native = span.details?.source === "codex_otel" && span.details.category !== "turn";
    if (!native && !["llm", "tool", "csgclaw", "user"].includes(span.owner)) continue;
    const owner = native ? `native_${span.details?.category}` : span.owner;
    const left = Math.max(start, span.start_ms);
    const right = Math.min(end, span.end_ms ?? total);
    if (right <= left) continue;
    edges.push({ time: left, owner, delta: 1 }, { time: right, owner, delta: -1 });
  }
  edges.push({ time: end, owner: "", delta: 0 });
  edges.sort((a, b) => a.time - b.time);
  const active = new Map<string, number>();
  let position = start;
  for (const edge of edges) {
    const owners = [...active].filter(([, count]) => count > 0).map(([owner]) => owner);
    const measured = owners.filter((owner) => !owner.startsWith("native_"));
    // Native observations explain only otherwise unmeasured gaps. They must
    // never double-count a bridge request or CSGClaw tool/event measurement.
    const native = [
      "native_tools",
      "native_request",
      "native_stream",
      "native_persist",
      "native_prepare",
      "native_dispatch",
    ].find((owner) => owners.includes(owner));
    const key = measured.length > 1 ? "overlap" : measured[0] || native || "unattributed";
    sums[key] = (sums[key] || 0) + Math.max(0, edge.time - position);
    if (edge.owner) active.set(edge.owner, (active.get(edge.owner) || 0) + edge.delta);
    position = edge.time;
  }
  return Object.entries(sums)
    .filter(([, value]) => value > 0)
    .map(([key, duration]) => ({ key, duration, percent: total ? (duration / total) * 100 : 0 }))
    .sort((a, b) => b.duration - a.duration);
}

export function diagnosticEventGroups(spans: DiagnosticSpan[]) {
  const groups = new Map<string, { type: string; count: number; duration: number; peak: number }>();
  for (const span of spans) {
    const type = span.details?.event_type || "unknown";
    const item = groups.get(type) ?? { type, count: 0, duration: 0, peak: 0 };
    const duration = Math.max(0, (span.end_ms ?? span.start_ms) - span.start_ms);
    item.count++;
    item.duration += duration;
    item.peak = Math.max(item.peak, duration);
    groups.set(type, item);
  }
  return [...groups.values()].sort((a, b) => b.duration - a.duration);
}

export function diagnosticTimelineRows(record: TurnDiagnostic) {
  let llm = 0,
    tool = 0;
  return [...(record.spans || [])]
    .sort((a, b) => a.start_ms - b.start_ms)
    .map((span, index) => ({
      span,
      ordinal: index + 1,
      callOrdinal: span.owner === "llm" ? ++llm : span.owner === "tool" ? ++tool : undefined,
      duration: Math.max(0, (span.end_ms ?? record.total_ms) - span.start_ms),
    }));
}
export function diagnosticSpanLabel(span: DiagnosticSpan, t: TranslateFn, callOrdinal?: number) {
  if (callOrdinal !== undefined && span.owner === "llm")
    return `${t("diagLLMRequest")} #${callOrdinal} · ${span.details?.label || "LLM"}`;
  if (callOrdinal !== undefined && span.owner === "tool")
    return `${t("diagTool")} #${callOrdinal} · ${span.details?.label || span.name.slice(5)}`;
  if (span.details?.source === "codex_otel")
    return t(`diagNative_${span.details.label?.replaceAll(".", "_").replaceAll("/", "_")}`);
  if (span.name === "event.deliver")
    return `${t("diagStage_event_deliver")} · ${diagnosticEventLabel(span.details?.event_type || "unknown", t)}`;
  if (span.name === "llm.request") return `${t("diagLLMRequest")} · ${span.details?.label || ""}`;
  return (
    span.details?.label ||
    (span.name.startsWith("tool.")
      ? `${t("diagTool")} · ${span.name.slice(5)}`
      : t(`diagStage_${span.name.replaceAll(".", "_")}`))
  );
}
export function diagnosticTimestamp(record: TurnDiagnostic, offset?: number) {
  if (offset === undefined) return "N/A";
  return diagnosticLocalTime(new Date(record.started_at).getTime() + offset);
}

export function diagnosticEventLabel(type: string, t: TranslateFn) {
  if (type === "unknown") return t("diagEventUnknown");
  const [kind, ...extra] = type.split("/");
  const label = [
    "text_delta",
    "thought_delta",
    "tool_call_start",
    "tool_call_update",
    "activity_update",
    "interaction_request",
    "output_item",
  ].includes(kind)
    ? t(`diagEvent_${kind}`)
    : kind;
  return extra.length ? `${label} · ${extra.join("/")}` : label;
}

export function diagnosticLocalTime(value: number | string, timeZone?: string) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "N/A";
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
    hourCycle: "h23",
    timeZoneName: "shortOffset",
    timeZone,
  }).format(date);
}
export const diagnosticHiddenCategories = ["progress", "persistence", "scope", "hooks", "http"] as const;
export type DiagnosticHiddenCategory = (typeof diagnosticHiddenCategories)[number];
export const diagnosticDefaultHiddenCategories: readonly DiagnosticHiddenCategory[] = [
  "progress",
  "persistence",
  "scope",
  "hooks",
];
export function diagnosticHiddenCategory(span: DiagnosticSpan): DiagnosticHiddenCategory | undefined {
  if (span.name === "event.deliver") return "progress";
  if (span.details?.source === "codex_otel") {
    if (span.details.category === "persist") return "persistence";
    if (span.details.label === "session_task.turn") return "scope";
    if (["run_hooks_and_record_inputs", "run_turn_stop_hooks"].includes(span.details.label || "")) return "hooks";
    if (span.details.label === "responses.stream_request") return "http";
  }
}
export function diagnosticIsHidden(span: DiagnosticSpan, hidden: readonly DiagnosticHiddenCategory[]) {
  const category = diagnosticHiddenCategory(span);
  return category !== undefined && hidden.includes(category);
}
export function diagnosticHiddenStats(record: TurnDiagnostic, hidden: readonly DiagnosticHiddenCategory[], owner = "") {
  const spans = (record.spans || []).filter((span) => !owner || span.owner === owner);
  const cost = (items: DiagnosticSpan[]) => ({
    count: items.length,
    duration: diagnosticIntervalUnion(items, record.total_ms),
  });
  const hiddenSpans = spans.filter((span) => diagnosticIsHidden(span, hidden));
  const failed = (span: DiagnosticSpan) => ["failed", "canceled", "interrupted"].includes(span.status);
  // Aggregate turn scopes are inclusive, not standalone slow work.
  const slow = (span: DiagnosticSpan) =>
    diagnosticHiddenCategory(span) !== "scope" && (span.end_ms ?? record.total_ms) - span.start_ms >= 100;
  const attention = hiddenSpans.filter((span) => failed(span) || slow(span));
  return {
    count: hiddenSpans.length,
    duration: diagnosticIntervalUnion(
      hiddenSpans.filter((span) => diagnosticHiddenCategory(span) !== "scope"),
      record.total_ms,
    ),
    attention: {
      count: attention.length,
      failed: attention.filter(failed).length,
      slow: attention.filter(slow).length,
      categories: diagnosticHiddenCategories.filter((key) =>
        attention.some((span) => diagnosticHiddenCategory(span) === key),
      ),
    },
    categories: diagnosticHiddenCategories.map((key) => ({
      key,
      ...cost(spans.filter((span) => diagnosticHiddenCategory(span) === key)),
    })),
  };
}
