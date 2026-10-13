import { useState } from "react";
import { ChevronDown, ChevronRight, Coins } from "lucide-react";
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
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={`message-token-usage ${expanded ? "is-expanded" : ""}`.trim()}>
      <button
        type="button"
        className="message-token-usage-trigger"
        aria-label={t("tokenUsageToggle")}
        aria-expanded={expanded}
        onClick={() => setExpanded((value) => !value)}
      >
        <Coins aria-hidden="true" size={13} />
        {expanded ? <ChevronRight aria-hidden="true" size={13} /> : <ChevronDown aria-hidden="true" size={13} />}
      </button>
      {expanded ? (
        <div className="message-token-usage-details">
          <span>{t("tokenUsageOutput", { count: formatTokenCount(MOCK_USAGE.output) })}</span>
          <span aria-hidden="true">·</span>
          <span>{t("tokenUsageModel", { model: MOCK_USAGE.model })}</span>
        </div>
      ) : null}
    </div>
  );
}
