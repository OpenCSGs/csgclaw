export const DEFAULT_CONTEXT_TOKENS = 200_000;
export type ModelMetadata = {
  context_window: number;
  context_source: string;
};
export type ModelOverride = { context_window?: number };
export type ContextUsage = {
  session_id: string;
  model_id: string;
  used_tokens: number | null;
  context_window: number;
  context_source: string;
  auto_compact: boolean;
  compact_threshold: number;
  compacting: boolean;
  estimated: boolean;
  updated_at: string;
};
const record = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
const positive = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value > 0;
export function normalizeModelMetadataMap(value: unknown): Record<string, ModelMetadata> {
  return Object.fromEntries(
    Object.entries(record(value)).map(([id, raw]) => {
      const m = record(raw);
      return [
        id,
        {
          context_window: positive(m.context_window) ? m.context_window : DEFAULT_CONTEXT_TOKENS,
          context_source: String(m.context_source || "default"),
        },
      ];
    }),
  );
}
export function normalizeModelOverrides(value: unknown): Record<string, ModelOverride> {
  return Object.fromEntries(
    Object.entries(record(value)).map(([id, raw]) => {
      const m = record(raw);
      return [
        id,
        {
          ...(positive(m.context_window) ? { context_window: m.context_window } : {}),
        },
      ];
    }),
  );
}
export function normalizeContextUsage(value: unknown): ContextUsage | undefined {
  const u = record(value);
  if (!positive(u.context_window) || typeof u.session_id !== "string" || typeof u.model_id !== "string")
    return undefined;
  const used =
    typeof u.used_tokens === "number" && Number.isSafeInteger(u.used_tokens) && u.used_tokens >= 0
      ? u.used_tokens
      : null;
  return {
    session_id: u.session_id,
    model_id: u.model_id,
    used_tokens: used,
    context_window: u.context_window,
    context_source: String(u.context_source || "default"),
    auto_compact: u.auto_compact === true,
    compact_threshold: positive(u.compact_threshold) ? u.compact_threshold : Math.floor(u.context_window * 0.75),
    compacting: u.compacting === true,
    estimated: u.estimated === true,
    updated_at: typeof u.updated_at === "string" ? u.updated_at : "",
  };
}
export function contextUsageRatio(usage?: ContextUsage): number | null {
  return usage && usage.used_tokens !== null && usage.context_window > 0
    ? usage.used_tokens / usage.context_window
    : null;
}

export type ContextUnit = "K" | "M";
export function contextUnit(tokens: number): ContextUnit {
  return tokens >= 1_000_000 ? "M" : "K";
}
export function contextInputValue(tokens: number, unit: ContextUnit = contextUnit(tokens)): string {
  return String(tokens / (unit === "M" ? 1_000_000 : 1_000));
}
export function parseContextSize(text: string, unit: ContextUnit): number | undefined {
  const clean = text.replaceAll(",", "").trim();
  if (!/^\d+(?:\.\d+)?$/.test(clean)) return undefined;
  const exact = Number(clean) * (unit === "M" ? 1_000_000 : 1_000);
  const tokens = Math.round(exact);
  return Number.isSafeInteger(tokens) && tokens > 0 && tokens <= 1_000_000_000 && Math.abs(exact - tokens) < 0.000001
    ? tokens
    : undefined;
}
export function formatContextSize(tokens: number): string {
  const unit = contextUnit(tokens);
  return `${(tokens / (unit === "M" ? 1_000_000 : 1_000)).toLocaleString(undefined, { maximumFractionDigits: 2 })} ${unit} tokens`;
}
export function modelOverridesEqual(
  left: Record<string, ModelOverride>,
  right: Record<string, ModelOverride>,
): boolean {
  const normalize = (value: Record<string, ModelOverride>) =>
    Object.entries(value)
      .filter(([, v]) => v.context_window)
      .map(([key, v]) => [key, v.context_window])
      .sort(([a], [b]) => String(a).localeCompare(String(b)));
  return JSON.stringify(normalize(left)) === JSON.stringify(normalize(right));
}
