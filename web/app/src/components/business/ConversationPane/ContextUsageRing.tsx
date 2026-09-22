import { Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { contextUsageRatio, formatContextSize, type ContextUsage } from "@/models/modelMetadata";

export function ContextUsageRing({ usage, t }: { usage?: ContextUsage; t: TranslateFn }) {
  const ratio = contextUsageRatio(usage);
  const percent = ratio === null ? null : Math.round(ratio * 100);
  const label = percent === null ? t("contextUsageUnknown") : t("contextUsagePercent", { percent });
  const used = usage?.used_tokens;
  const tone =
    ratio !== null && ratio >= 1
      ? "danger"
      : usage && used !== null && used !== undefined && used >= usage.compact_threshold
        ? "warning"
        : "normal";
  const circumference = 2 * Math.PI * 7;
  const format = formatContextSize;
  const detail = (
    <div className="context-usage-details">
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
  return (
    <Tooltip content={detail} contentProps={{ side: "top", sideOffset: 6 }}>
      <button type="button" className={`context-usage-ring is-${tone}`} aria-label={label}>
        <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
          <circle
            className="context-usage-track"
            cx="8"
            cy="8"
            r="7"
            fill="none"
            strokeWidth="2"
            strokeDasharray={ratio === null ? "2 2" : undefined}
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
              transform="rotate(-90 8 8)"
            />
          ) : null}
        </svg>
      </button>
    </Tooltip>
  );
}
