import { useEffect, useState } from "react";
import { LoaderCircle, Minimize2 } from "lucide-react";
import { compactionTargetNotice, type CompactionOperation, type CompactionTarget } from "@/models/contextCompaction";
import { formatContextSize } from "@/models/modelMetadata";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ContextCompactionCard.module.css";

export function CompactionTargetResult({ target, t }: { target: CompactionTarget; t: TranslateFn }) {
  const before = target.before?.used_tokens;
  const after = target.after?.used_tokens;
  const completed = target.status === "completed";
  return (
    <div className={styles.result}>
      {completed ? (
        <>
          <span>{t("contextCompactHistorySummarized")}</span>
          {before != null && after != null ? (
            <>
              <span className={styles.change}>
                {t("contextCompactBeforeAfter", { before: formatContextSize(before), after: formatContextSize(after) })}
              </span>
              <span>
                {t(after < before ? "contextCompactSaved" : "contextCompactNoReduction", {
                  count: formatContextSize(Math.max(0, before - after)),
                })}
                {target.after?.estimated ? ` · ${t("contextCompactEstimated")}` : ""}
              </span>
            </>
          ) : (
            <span>
              {before != null
                ? t("contextCompactBeforeOnly", { before: formatContextSize(before) })
                : t("contextCompactUsageUnavailable")}
            </span>
          )}
        </>
      ) : (
        <span>{compactionTargetNotice(target, t)}</span>
      )}
    </div>
  );
}

export function ContextCompactionCard({ operation, t }: { operation: CompactionOperation; t: TranslateFn }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!operation.running) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [operation.running]);
  const end = operation.finished_at ? new Date(operation.finished_at).getTime() : now;
  const seconds = Math.max(0, Math.floor((end - new Date(operation.started_at).getTime()) / 1000));
  const single = !operation.batch && operation.targets.length === 1 ? operation.targets[0] : undefined;
  const completed = operation.targets.filter((target) => target.status === "completed").length;
  const skipped = operation.targets.filter((target) =>
    ["unnecessary", "no_history", "unknown", "busy", "unavailable", "unsupported"].includes(target.status),
  ).length;
  const unfinished = operation.targets.length - completed - skipped;
  const summary = [
    completed ? t("contextCompactCountCompleted", { count: completed }) : "",
    skipped ? t("contextCompactCountSkipped", { count: skipped }) : "",
    unfinished ? t("contextCompactCountUnfinished", { count: unfinished }) : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const title = operation.running
    ? t("contextCompactStatus_running")
    : single
      ? t(single.status === "completed" ? "contextCompactSingleDone" : "contextCompactSingleSkipped", {
          name: single.name,
        })
      : t("contextCompactBatchDone");
  return (
    <section className={styles.card} aria-label={t("contextCompactTitle")} aria-busy={operation.running}>
      <div className={styles.heading} role="status">
        {operation.running ? <LoaderCircle size={15} className={styles.spinner} /> : <Minimize2 size={15} />}
        <strong>{title}</strong>
        <span>{t("contextCompactElapsed", { seconds })}</span>
      </div>
      {!single ? (
        <p className={styles.summary}>
          {operation.running
            ? t("contextCompactBatchProgress", { count: completed + skipped, total: operation.targets.length })
            : summary}
        </p>
      ) : null}
      {operation.targets.map((target) => (
        <div className={styles.target} key={target.agent_id}>
          {!single ? <strong>{target.name}</strong> : null}
          <CompactionTargetResult target={target} t={t} />
        </div>
      ))}
    </section>
  );
}
