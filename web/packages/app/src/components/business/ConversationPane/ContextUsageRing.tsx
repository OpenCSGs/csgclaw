import contextUsageRingStyles from "./ContextUsageRing.module.css";
import { useState, type ReactNode, type KeyboardEventHandler } from "react";
import { Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { contextUsageRatio, formatContextSize, type ContextUsage } from "@/models/modelMetadata";

export function ContextUsageRing({
  usage,
  t,
  content,
  label: customLabel,
  onKeyDown,
}: {
  usage?: ContextUsage;
  t: TranslateFn;
  content?: ReactNode;
  label?: string;
  onKeyDown?: KeyboardEventHandler<HTMLButtonElement>;
}) {
  const [open, setOpen] = useState(false);
  const [hovered, setHovered] = useState(false);
  const ratio = contextUsageRatio(usage);
  const percent = ratio === null ? null : Math.round(ratio * 100);
  const label = customLabel ?? (percent === null ? t("contextUsageUnknown") : t("contextUsagePercent", { percent }));
  const used = usage?.used_tokens;
  const tone =
    ratio !== null && ratio >= 1
      ? "danger"
      : usage && used !== null && used !== undefined && used >= usage.compact_threshold
        ? "warning"
        : "normal";
  const circumference = 2 * Math.PI * 7;

  return (
    <Tooltip
      open={open}
      onOpenChange={setOpen}
      delayDuration={0}
      content={content ?? <ContextUsageDetails usage={usage} t={t} />}
      contentProps={{ side: "top", sideOffset: 6, "aria-label": content ? label : undefined }}
    >
      <button
        type="button"
        className={`${contextUsageRingStyles["context-usage-ring"]} is-${tone}`}
        aria-label={label}
        onKeyDown={onKeyDown}
        data-preserve-scroll={hovered || open ? "true" : undefined}
        onPointerEnter={() => setHovered(true)}
        onPointerLeave={() => setHovered(false)}
      >
        <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
          <circle
            className={contextUsageRingStyles["context-usage-track"]}
            cx="8"
            cy="8"
            r="7"
            fill="none"
            strokeWidth="2"
          />
          {ratio !== null ? (
            <circle
              className="context-usage-value"
              cx="8"
              cy="8"
              r="7"
              fill="none"
              strokeWidth="2"
              strokeDasharray={`${Math.max(0, Math.min(1, ratio)) * circumference} ${circumference}`}
              strokeLinecap={ratio > 0 ? "round" : "butt"}
              transform="rotate(-90 8 8)"
            />
          ) : null}
        </svg>
      </button>
    </Tooltip>
  );
}

export function ContextUsageDetails({
  usage,
  t,
  compact = false,
}: {
  usage?: ContextUsage;
  t: TranslateFn;
  compact?: boolean;
}) {
  const ratio = contextUsageRatio(usage);
  const percent = ratio === null ? null : Math.round(ratio * 100);
  const label = percent === null ? t("contextUsageUnknown") : t("contextUsagePercent", { percent });
  const used = usage?.used_tokens;
  const format = formatContextSize;
  return (
    <div
      className={`${contextUsageRingStyles["context-usage-details"]}${compact ? ` ${contextUsageRingStyles.compact}` : ""}`}
    >
      <strong>{label}</strong>
      {usage ? (
        <>
          <span>{t("contextUsageModel", { model: usage.model_id })}</span>
          {used !== null && used !== undefined ? (
            <>
              <span>{t("contextUsageUsed", { count: format(used) })}</span>
              <span>{t("contextUsageRemaining", { count: format(Math.max(0, usage.context_window - used)) })}</span>
            </>
          ) : null}
          <span>{t("contextUsageCapacity", { count: format(usage.context_window) })}</span>
          <span>{t("contextUsageSource", { source: t(`modelMetadataSource_${usage.context_source}`) })}</span>
          <span>{t(usage.auto_compact ? "contextAutoCompactEnabled" : "contextAutoCompactDisabled")}</span>
          {usage.compacting ? <span role="status">{t("contextCompacting")}</span> : null}
          {usage.updated_at ? (
            <span>
              {t("contextUsageReported")} · {new Date(usage.updated_at).toLocaleTimeString()}
            </span>
          ) : null}
          {usage.estimated && used !== null ? <span>{t("contextUsageEstimated")}</span> : null}
        </>
      ) : null}
    </div>
  );
}
