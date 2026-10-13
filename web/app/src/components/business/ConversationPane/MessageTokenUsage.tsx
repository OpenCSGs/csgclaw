import { useState } from "react";
import { ChevronDown, ChevronRight, Coins } from "lucide-react";
import type { TranslateFn } from "@/models/conversations";

type MessageTokenUsageProps = {
  t: TranslateFn;
};

// Temporary UI fixture. Replace with normalized usage from message metadata
// when the backend begins persisting per-turn model usage.
const MOCK_USAGE = { input: 1284, output: 326, total: 1610, model: "GPT-4.1" };

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) {
    return `${trimDecimal(value / 1_000_000)}M`;
  }
  if (value >= 1_000) {
    return `${trimDecimal(value / 1_000)}K`;
  }
  return value.toLocaleString();
}

function trimDecimal(value: number): string {
  return value.toFixed(1).replace(/\.0$/, "");
}

export function MessageTokenUsage({ t }: MessageTokenUsageProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={`message-token-usage ${expanded ? "is-expanded" : ""}`.trim()}>
      <button
        type="button"
        className="message-token-usage-trigger"
        aria-expanded={expanded}
        onClick={() => setExpanded((value) => !value)}
      >
        <Coins aria-hidden="true" size={13} />
        <span>{t("tokenUsageTotal", { count: formatTokenCount(MOCK_USAGE.total) })}</span>
        {expanded ? <ChevronRight aria-hidden="true" size={13} /> : <ChevronDown aria-hidden="true" size={13} />}
      </button>
      {expanded ? (
        <div className="message-token-usage-details">
          <span>{t("tokenUsageInput", { count: MOCK_USAGE.input.toLocaleString() })}</span>
          <span>{t("tokenUsageOutput", { count: MOCK_USAGE.output.toLocaleString() })}</span>
          <span>{t("tokenUsageModel", { model: MOCK_USAGE.model })}</span>
          <span className="message-token-usage-sample">{t("tokenUsageEstimate")}</span>
        </div>
      ) : null}
    </div>
  );
}
