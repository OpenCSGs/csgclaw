export type TokenUsage = {
  known: boolean;
  model_id: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cached_input_tokens: number;
  reasoning_tokens: number;
  partial: boolean;
};

function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function messageTokenUsage(metadata: unknown): TokenUsage | null {
  const progress = record(record(record(metadata)?.csgclaw)?.turn_progress);
  if (!progress || !["succeeded", "failed", "canceled", "interrupted"].includes(String(progress.status))) return null;
  const raw = record(progress.usage);
  if (!raw || typeof raw.model_id !== "string" || typeof raw.known !== "boolean") return null;
  const values = [
    raw.input_tokens,
    raw.output_tokens,
    raw.total_tokens,
    raw.cached_input_tokens ?? 0,
    raw.reasoning_tokens ?? 0,
  ];
  if (!values.every((v) => typeof v === "number" && Number.isSafeInteger(v) && v >= 0 && v <= 1e12)) return null;
  const [input, output, total, cached, reasoning] = values as number[];
  if (input + output !== total || cached > input || reasoning > output) return null;
  return {
    known: raw.known,
    model_id: raw.model_id,
    input_tokens: input,
    output_tokens: output,
    total_tokens: total,
    cached_input_tokens: cached,
    reasoning_tokens: reasoning,
    partial: raw.partial === true,
  };
}

export function tokenCountLabel(value: number): string {
  if (value < 1000) return String(value);
  return `${Number((value / (value >= 1_000_000 ? 1_000_000 : 1000)).toFixed(2))} ${value >= 1_000_000 ? "M" : "K"}`;
}

export function visibleUsageModel(model: string): string {
  const value = model.trim();
  return value.toLowerCase() === "auto" ? "" : value;
}
