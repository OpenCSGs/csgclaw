import { Info } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button, Tooltip } from "@/components/ui";
import { tokenCountLabel, visibleUsageModel, type TokenUsage } from "@/models/tokenUsage";
import styles from "./MessageTokenUsage.module.css";
import type { TranslateFn } from "@/models/conversations";

type MessageTokenUsageProps = { t: TranslateFn; usage: TokenUsage };

export function MessageTokenUsage({ t, usage }: MessageTokenUsageProps) {
  const usageRef = useRef<HTMLDivElement>(null);
  const measurementRef = useRef<HTMLSpanElement>(null);
  const [compact, setCompact] = useState(false);
  const model = visibleUsageModel(usage.model_id);
  const label = usage.known
    ? t("turnTokenUsage", { count: tokenCountLabel(usage.total_tokens) })
    : t("turnTokenUsageUnavailable");
  const details = `${label}${usage.known && usage.partial ? ` ${t("turnTokenUsagePartial")}` : ""}${model ? ` · ${t("turnTokenUsageModel", { model })}` : ""}`;
  const counters = usage.known
    ? t("turnTokenUsageDetails", {
        input: usage.input_tokens,
        output: usage.output_tokens,
        cached: usage.cached_input_tokens,
        reasoning: usage.reasoning_tokens,
      })
    : "";
  const tooltip = counters ? `${details}\n${counters}` : details;

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
    <div ref={usageRef} className={styles.usage}>
      <Tooltip content={tooltip} contentProps={{ className: styles.tooltip, side: "top", sideOffset: 6 }}>
        <Button variant="linkGray" size="sm" type="button" className={styles.trigger} aria-label={details}>
          {compact ? <Info aria-hidden="true" size={16} /> : <span>{details}</span>}
        </Button>
      </Tooltip>
      <span ref={measurementRef} className={styles.measure} aria-hidden="true">
        {details}
      </span>
    </div>
  );
}
