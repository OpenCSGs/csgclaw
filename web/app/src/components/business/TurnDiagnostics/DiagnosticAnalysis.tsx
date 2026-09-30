import { useState } from "react";
import { Button } from "@/components/ui";
import type { DiagnosticSpan, TurnDiagnostic } from "@/api/diagnostics";
import {
  diagnosticBreakdown,
  diagnosticTimelineRows,
  diagnosticDuration,
  diagnosticEventGroups,
  diagnosticEventLabel,
} from "@/models/diagnostics";
import type { TranslateFn } from "@/models/conversations";
import styles from "./DiagnosticAnalysis.module.css";

export function DiagnosticBreakdown({ record, t }: { record: TurnDiagnostic; t: TranslateFn }) {
  const buckets = diagnosticBreakdown(record);
  const largest = buckets[0]?.duration || 1;
  return (
    <section className={styles.analysis} aria-label={t("diagRanking")}>
      <h4>{t("diagRanking")}</h4>
      <p>{t("diagRankingHint")}</p>
      <ol className={styles.ranking}>
        {buckets.map((bucket) => (
          <li key={bucket.key} data-bucket={bucket.key}>
            <span className={styles.bucketLabel}>{t(`diagBucket_${bucket.key}`)}</span>
            <span className={styles.rankTrack} aria-hidden="true">
              <span style={{ width: `${(bucket.duration / largest) * 100}%` }} />
            </span>
            <strong>{diagnosticDuration(bucket.duration)}</strong>
            <small>{bucket.percent.toFixed(1)}%</small>
          </li>
        ))}
      </ol>
      <div className={styles.total}>
        {t("diagAccountedTotal")}{" "}
        <strong>{diagnosticDuration(buckets.reduce((sum, item) => sum + item.duration, 0))}</strong>
      </div>
    </section>
  );
}

export function DiagnosticSpanFields({ span, t }: { span: DiagnosticSpan; t: TranslateFn }) {
  const details = span.details;
  return (
    <div className={styles.fields}>
      {details?.source === "codex_otel" ? (
        <dl>
          <dt>{t("diagMeasurementSource")}</dt>
          <dd>Codex OpenTelemetry</dd>
          <dt>{t("diagNativePhase")}</dt>
          <dd>{details.label}</dd>
        </dl>
      ) : null}
      {details?.reported_duration_ms !== undefined || details?.exit_code !== undefined ? (
        <dl>
          {details?.reported_duration_ms !== undefined ? (
            <>
              <dt>{t("diagReportedDuration")}</dt>
              <dd>{diagnosticDuration(details.reported_duration_ms)}</dd>
            </>
          ) : null}
          {details?.exit_code !== undefined ? (
            <>
              <dt>{t("diagExitCode")}</dt>
              <dd>{details.exit_code}</dd>
            </>
          ) : null}
        </dl>
      ) : null}
      {details?.command ? (
        <div>
          <strong>{t("diagCommand")}</strong>
          <pre>{details.command}</pre>
        </div>
      ) : null}
      {details?.directory ? (
        <div>
          <strong>{t("diagDirectory")}</strong>
          <pre>{details.directory}</pre>
        </div>
      ) : null}
      {details?.arguments && details.arguments !== details.command ? (
        <div>
          <strong>{t("diagArguments")}</strong>
          <pre>{details.arguments}</pre>
        </div>
      ) : null}
      {span.owner === "tool" && !details?.command && !details?.arguments ? <p>{t("diagArgumentsMissing")}</p> : null}
      {span.owner === "llm" && span.name !== "llm.video" ? (
        <dl>
          <dt>{t("diagOperation")}</dt>
          <dd>{details?.label || "LLM"}</dd>
          <dt>HTTP</dt>
          <dd>{details?.http_status || t("diagNotMeasured")}</dd>
          <dt>{t("diagFirstResponse")}</dt>
          <dd>{diagnosticDuration(details?.first_response_ms)}</dd>
        </dl>
      ) : null}
      {span.owner === "llm" ? (
        <p>{t(span.name === "llm.video" ? "diagVideoModelHint" : "diagFirstResponseHint")}</p>
      ) : null}
    </div>
  );
}

export function DiagnosticCalls({ record, t }: { record: TurnDiagnostic; t: TranslateFn }) {
  const [limit, setLimit] = useState(20);
  const calls = diagnosticTimelineRows(record)
    .filter((row) => row.callOrdinal !== undefined)
    .map(({ span, callOrdinal, duration }) => ({ span, ordinal: callOrdinal, duration }))
    .sort((a, b) => b.duration - a.duration);
  if (!calls.length) return null;
  return (
    <section className={styles.analysis} aria-label={t("diagCalls")}>
      <h4>{t("diagCalls")}</h4>
      <p>{t("diagCallsHint")}</p>
      {calls.slice(0, limit).map(({ span, ordinal, duration }) => (
        <details
          className={styles.call}
          key={span.id}
          data-call-id={span.id}
          data-call-number={ordinal}
          data-call-owner={span.owner}
        >
          <summary>
            <span className={styles.badge} data-owner={span.owner}>
              {span.owner === "llm" ? "LLM" : t("diagTool")} #{ordinal}
            </span>
            <span className={styles.callName}>
              {span.owner === "llm"
                ? span.name === "llm.video"
                  ? span.details?.label || t("diagVideoModelRequest")
                  : record.model || span.details?.label || "LLM"
                : span.details?.label || span.name.slice(5)}
              {span.details?.command ? <code>{span.details.command.split("\n")[0].slice(0, 160)}</code> : null}
            </span>
            <span className={styles.callTime}>
              {diagnosticDuration(duration)}
              <small>{t(`diagStatus_${span.status}`)}</small>
            </span>
          </summary>
          <div className={styles.offset}>
            {t("diagOffset")}: {diagnosticDuration(span.start_ms)} → {diagnosticDuration(span.end_ms)}
          </div>
          <DiagnosticSpanFields span={span} t={t} />
        </details>
      ))}
      {calls.length > limit ? (
        <Button size="sm" variant="tertiaryGray" onClick={() => setLimit((value) => value + 20)}>
          {t("diagMoreCalls")}
        </Button>
      ) : null}
    </section>
  );
}

export function DiagnosticEvents({ spans, t }: { spans: DiagnosticSpan[]; t: TranslateFn }) {
  const [limit, setLimit] = useState(100);
  if (!spans.length) return null;
  const groups = diagnosticEventGroups(spans);
  return (
    <details className={styles.events}>
      <summary>
        {t("diagEventSummary", { count: String(spans.length) })}
        <strong>{diagnosticDuration(groups.reduce((sum, group) => sum + group.duration, 0))}</strong>
      </summary>
      <p>{t("diagEventHint")}</p>
      <table>
        <thead>
          <tr>
            <th>{t("diagEventType")}</th>
            <th>{t("diagCount")}</th>
            <th>{t("diagEventTotal")}</th>
            <th>{t("diagEventPeak")}</th>
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => (
            <tr key={group.type}>
              <td>{diagnosticEventLabel(group.type, t)}</td>
              <td>{group.count}</td>
              <td>{diagnosticDuration(group.duration)}</td>
              <td>{diagnosticDuration(group.peak)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <details className={styles.eventList}>
        <summary>{t("diagEventList")}</summary>
        <table>
          <thead>
            <tr>
              <th>{t("diagEventType")}</th>
              <th>{t("diagOffset")}</th>
              <th>{t("diagEventTotal")}</th>
            </tr>
          </thead>
          <tbody>
            {spans.slice(0, limit).map((span) => (
              <tr key={span.id}>
                <td>{diagnosticEventLabel(span.details?.event_type || "unknown", t)}</td>
                <td>{diagnosticDuration(span.start_ms)}</td>
                <td>{diagnosticDuration((span.end_ms ?? span.start_ms) - span.start_ms)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {spans.length > limit ? (
          <Button size="sm" variant="tertiaryGray" onClick={() => setLimit((value) => value + 100)}>
            {t("diagMoreCalls")}
          </Button>
        ) : null}
      </details>
    </details>
  );
}
