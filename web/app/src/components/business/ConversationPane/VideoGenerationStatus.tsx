import type { IMMessage, TranslateFn } from "@/models/conversations";

export function VideoGenerationStatus({ message, t }: { message: IMMessage; t: TranslateFn }) {
  const raw = message.metadata?.video_generation;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  const task = raw as Record<string, unknown>;
  const state = String(task.state ?? "");
  const upstreamStatus = String(task.upstream_status ?? "");
  const labels: Record<string, string> = {
    generating: t("videoGenerating"),
    delivering: t("videoDelivering"),
    completed: t("videoCompleted"),
    failed: t("videoGenerationFailed"),
    delivery_failed: t("videoDeliveryFailed"),
  };
  const progressLabels: Record<string, string> = {
    queued: t("videoQueued"),
    in_progress: t("videoGenerating"),
    downloading: t("videoDownloading"),
  };
  const errorDetails = task.error_details && typeof task.error_details === "object" && !Array.isArray(task.error_details)
    ? task.error_details as Record<string, unknown>
    : null;
  const errorMessage = String(errorDetails?.message ?? "").trim();
  return (
    <div className="image-generation-status" role="status" aria-live="polite">
      <span>{state === "generating" && progressLabels[upstreamStatus] ? progressLabels[upstreamStatus] : labels[state] || t("videoGenerating")}</span>
      {state === "failed" && errorMessage ? <p>{errorMessage}</p> : null}
      <details className="image-generation-prompt">
        <summary>{t("videoViewPrompt")}</summary>
        <p>{String(task.prompt ?? message.content ?? "")}</p>
      </details>
    </div>
  );
}
