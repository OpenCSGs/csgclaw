import type { TranslateFn } from "@/models/conversations";

type MessageTokenUsageProps = {
  t: TranslateFn;
};

// Temporary UI fixture. Replace with normalized usage from message metadata
// when the backend begins persisting per-turn model usage.
const MOCK_USAGE = { output: 1220, model: "gpt-5.6-sol" };

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) {
    return `${trimDecimal(value / 1_000_000)} M`;
  }
  if (value >= 1_000) {
    return `${trimDecimal(value / 1_000)} K`;
  }
  return value.toLocaleString();
}

function trimDecimal(value: number): string {
  return value.toFixed(2).replace(/\.0+$|(?<=\.[0-9])0$/, "");
}

export function MessageTokenUsage({ t }: MessageTokenUsageProps) {
  return (
    <div className="message-token-usage">
      <span>{t("tokenUsageOutput", { count: formatTokenCount(MOCK_USAGE.output) })}</span>
      <span aria-hidden="true">·</span>
      <span>{t("tokenUsageModel", { model: MOCK_USAGE.model })}</span>
    </div>
  );
}
