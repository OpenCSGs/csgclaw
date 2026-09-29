import { useState } from "react";
import type { TurnDiagnostic } from "@/api/diagnostics";
import { Button, Tooltip, TooltipProvider } from "@/components/ui";
import {
  diagnosticDuration,
  diagnosticSpanLabel,
  diagnosticTimestamp,
  diagnosticIsHidden,
  diagnosticDefaultHiddenCategories,
  diagnosticTimelineRows,
  type DiagnosticHiddenCategory,
} from "@/models/diagnostics";
import type { TranslateFn } from "@/models/conversations";
import { DiagnosticSpanFields } from "./DiagnosticAnalysis";
import styles from "./TurnDiagnostics.module.css";

export function DiagnosticTimeline({
  record,
  owner,
  hiddenCategories = diagnosticDefaultHiddenCategories,
  selected,
  onSelect,
  t,
}: {
  record: TurnDiagnostic;
  owner: string;
  hiddenCategories?: readonly DiagnosticHiddenCategory[];
  selected: string;
  onSelect: (id: string) => void;
  t: TranslateFn;
}) {
  const [limit, setLimit] = useState(100);
  const total = Math.max(record.total_ms, 1);
  const rows = diagnosticTimelineRows(record).filter(
    ({ span }) => !diagnosticIsHidden(span, hiddenCategories) && (!owner || span.owner === owner),
  );
  return (
    <TooltipProvider>
      <p className={styles.hint}>{t("diagTimelineHint")}</p>
      <div className={styles.timeline}>
        {rows.slice(0, limit).map(({ span, ordinal, callOrdinal }) => (
          <Tooltip
            key={span.id}
            contentProps={{ side: "top", align: "start", className: styles.spanTooltip }}
            content={
              <>
                <strong>
                  #{ordinal} {diagnosticSpanLabel(span, t, callOrdinal)}
                </strong>
                <dl>
                  <dt>{t("diagStartOffset")}</dt>
                  <dd>+{diagnosticDuration(span.start_ms)}</dd>
                  <dt>{t("diagStartTime")}</dt>
                  <dd>{diagnosticTimestamp(record, span.start_ms)}</dd>
                  <dt>{t("diagEndTime")}</dt>
                  <dd>{diagnosticTimestamp(record, span.end_ms)}</dd>
                  <dt>{t("diagOwner")}</dt>
                  <dd>{t(`diagOwner_${span.owner}`)}</dd>
                  <dt>{t("diagStatus")}</dt>
                  <dd>{t(`diagStatus_${span.status}`)}</dd>
                </dl>
                <DiagnosticSpanFields span={span} t={t} />
              </>
            }
          >
            <button
              type="button"
              className={styles.spanRow}
              data-event-number={ordinal}
              data-call-id={callOrdinal !== undefined ? span.id : undefined}
              data-call-number={callOrdinal}
              data-call-owner={callOrdinal !== undefined ? span.owner : undefined}
              aria-pressed={selected === span.id}
              onClick={() => onSelect(span.id)}
            >
              <span className={styles.spanName}>
                <span className={styles.ordinal}>#{ordinal}</span>
                <span className={styles.dot} data-owner={span.owner} />
                <span>
                  {diagnosticSpanLabel(span, t, callOrdinal)}{" "}
                  <span
                    className={styles.eventOffset}
                    aria-label={`${t("diagStartOffset")}: ${diagnosticDuration(span.start_ms)}`}
                  >
                    +{diagnosticDuration(span.start_ms)}
                  </span>
                </span>
              </span>
              <span className={styles.track} aria-hidden="true">
                {[record.runtime_start_ms, record.runtime_end_ms].map((boundary, index) =>
                  boundary !== undefined ? (
                    <i
                      key={index}
                      className={styles.boundary}
                      style={{ left: `${Math.min(100, (boundary / total) * 100)}%` }}
                    />
                  ) : null,
                )}
                <span
                  className={styles.bar}
                  data-owner={span.owner}
                  style={{
                    left: `${Math.min(99.7, (span.start_ms / total) * 100)}%`,
                    width: `${Math.max(0.3, (Math.min(total - span.start_ms, (span.end_ms ?? total) - span.start_ms) / total) * 100)}%`,
                  }}
                />
              </span>
              <span className={styles.time}>{diagnosticDuration((span.end_ms ?? total) - span.start_ms)}</span>
            </button>
          </Tooltip>
        ))}
        {rows.length > limit ? (
          <Button size="sm" variant="secondaryGray" onClick={() => setLimit((value) => value + 100)}>
            {t("diagMoreEvents")} ({rows.length - limit})
          </Button>
        ) : null}
      </div>
    </TooltipProvider>
  );
}
