import { useEffect, useRef, useState } from "react";
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
  const [compact, setCompact] = useState(true);
  const output = t("tokenUsageOutput", { count: formatTokenCount(MOCK_USAGE.output) });
  const model = t("tokenUsageModel", { model: MOCK_USAGE.model });
  const details = `${output} · ${model}`;

  useEffect(() => {
    const usage = usageRef.current;
    const toolbar = usage?.parentElement;
    const measurement = measurementRef.current;
    if (!toolbar || !measurement) return;

    const updateCompactMode = () => {
      const otherItems = Array.from(toolbar.children).filter((child) => child !== usage);
      const gap = Number.parseFloat(window.getComputedStyle(toolbar).columnGap) || 0;
      const usedWidth = otherItems.reduce((total, child) => total + child.getBoundingClientRect().width, 0);
      const availableWidth = Math.max(0, toolbar.clientWidth - usedWidth - gap * otherItems.length);
      setCompact(measurement.getBoundingClientRect().width > availableWidth + 1);
    };

    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(updateCompactMode) : null;
    observer?.observe(toolbar);
    observer?.observe(measurement);
    Array.from(toolbar.children).forEach((child) => observer?.observe(child));
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
            <svg
              aria-hidden="true"
              width="16"
              height="16"
              viewBox="0 0 16 16"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
            >
              <g clipPath="url(#message-token-usage-info-clip)">
                <path
                  d="M8.00001 10.6663V7.99967M8.00001 5.33301H8.00668M14.6667 7.99967C14.6667 11.6816 11.6819 14.6663 8.00001 14.6663C4.31811 14.6663 1.33334 11.6816 1.33334 7.99967C1.33334 4.31778 4.31811 1.33301 8.00001 1.33301C11.6819 1.33301 14.6667 4.31778 14.6667 7.99967Z"
                  stroke="currentColor"
                  strokeWidth="1"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </g>
              <defs>
                <clipPath id="message-token-usage-info-clip">
                  <rect width="16" height="16" fill="white" />
                </clipPath>
              </defs>
            </svg>
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
