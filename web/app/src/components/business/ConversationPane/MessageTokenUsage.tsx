import { useEffect, useRef, useState } from "react";
import { Info } from "lucide-react";
import { Tooltip } from "@/components/ui";
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
  const usageRef = useRef<HTMLDivElement>(null);
  const measurementRef = useRef<HTMLSpanElement>(null);
  const [compact, setCompact] = useState(false);
  const output = t("tokenUsageOutput", { count: formatTokenCount(MOCK_USAGE.output) });
  const model = t("tokenUsageModel", { model: MOCK_USAGE.model });
  const details = `${output} · ${model}`;

  useEffect(() => {
    const usage = usageRef.current;
    const toolbar = usage?.parentElement;
    const measurement = measurementRef.current;
    if (!toolbar || !measurement) return;

    const updateCompactMode = () => {
      setCompact(measurement.getBoundingClientRect().width > toolbar.clientWidth + 1);
    };

    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(updateCompactMode) : null;
    observer?.observe(toolbar);
    observer?.observe(measurement);
    window.addEventListener("resize", updateCompactMode);
    updateCompactMode();

    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateCompactMode);
    };
  }, [details]);

  return (
    <div ref={usageRef} className="message-token-usage">
      {compact ? (
        <Tooltip
          content={details}
          contentProps={{ className: "message-token-usage-tooltip", side: "top", sideOffset: 6 }}
        >
          <button type="button" className="message-action-button message-token-usage-info" aria-label={details}>
            <Info aria-hidden="true" size={14} />
          </button>
        </Tooltip>
      ) : (
        <span className="message-token-usage-text">{details}</span>
      )}
      <span ref={measurementRef} className="message-token-usage-measure" aria-hidden="true">
        {details}
      </span>
    </div>
  );
}
