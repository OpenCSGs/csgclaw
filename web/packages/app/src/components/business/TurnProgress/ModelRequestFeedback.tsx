import { useEffect, useState, type ReactNode } from "react";
import { Button } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { ModelRequestStatus } from "@/models/turnProgress";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import styles from "./TurnProgress.module.css";

const causes: Record<string, string> = {
  connection: "modelRequestConnection",
  rate_limit: "modelRequestRateLimit",
  timeout: "modelRequestTimeout",
  rejected: "modelRequestRejected",
  stream_interrupted: "modelRequestInterrupted",
  server: "modelRequestServerError",
};
const sources: Record<string, string> = {
  client: "modelRequestSourceClient",
  gateway: "modelRequestSourceGateway",
  upstream: "modelRequestSourceUpstream",
  model: "modelRequestSourceModel",
};

export function ModelRequestFeedback({
  status,
  active,
  controls,
  t,
}: {
  status: ModelRequestStatus;
  active: boolean;
  controls?: ReactNode;
  t: TranslateFn;
}) {
  const retrying = active && status.state === "retrying";
  const retryAt = Date.parse(status.retry_at ?? "");
  const [now, setNow] = useState(Date.now);
  const [copied, setCopied] = useState("");
  useEffect(() => {
    if (!retrying || !Number.isFinite(retryAt)) return;
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [retrying, retryAt]);
  const seconds = Number.isFinite(retryAt) ? Math.max(0, Math.ceil((retryAt - Math.max(now, Date.now())) / 1000)) : 0;
  const requestID = status.gateway_request_id || status.request_id;
  const source = t(sources[status.source] ?? "modelRequestSourceGateway");
  const cause = t(causes[status.category] ?? "modelRequestServerError");
  const guidanceKey = status.code === "request_too_large" ? "modelRequestTooLargeNext" : `errors.${status.code ?? ""}`;
  const guidance = t(guidanceKey);
  const rejectedGuidance = guidance === guidanceKey ? "modelRequestRejectedNext" : guidanceKey;
  const outcome =
    status.state === "stopped"
      ? t("modelRequestStopped")
      : status.state === "recovered"
        ? t("modelRequestRecovered")
        : t("modelRequestFailed");
  return (
    <div className={styles.modelRequest}>
      <div className={styles.modelRequestSummary}>
        <div role="status">
          <p className={!active && status.state === "failed" ? styles.modelRequestFailed : undefined}>
            {retrying ? "" : `${outcome} · `}
            {source} · {cause}
            {status.http_status ? ` (${status.http_status})` : ""}
          </p>
          {retrying ? (
            <p className={styles.modelRequestRetry}>
              {t(seconds ? "modelRequestRetryWaiting" : "modelRequestRetrying", {
                attempt: status.attempt ?? "?",
                total: status.max_attempts ?? "?",
                seconds,
              })}
            </p>
          ) : null}
        </div>
        {active ? controls : null}
      </div>
      <p className={styles.modelRequestID}>
        {t("modelRequestID")} <code>{requestID}</code>{" "}
        <Button
          size="sm"
          variant="linkGray"
          onClick={async () => {
            if (await writeTextToClipboard(requestID)) setCopied(requestID);
          }}
        >
          {t(copied === requestID ? "modelRequestCopied" : "modelRequestCopy")}
        </Button>
      </p>
      {status.details ? (
        <details>
          <summary>{t("modelRequestDetails")}</summary>
          {status.gateway_request_id && status.gateway_request_id !== status.request_id ? (
            <p>
              {t("modelRequestClientID")} <code>{status.request_id}</code>
            </p>
          ) : null}
          <pre>{status.details}</pre>
        </details>
      ) : null}
      {!active && status.state !== "recovered" ? (
        <p>
          {t(
            status.state === "stopped"
              ? "modelRequestContinue"
              : status.category === "rejected"
                ? rejectedGuidance
                : "modelRequestFailedNext",
          )}
        </p>
      ) : null}
    </div>
  );
}
