import { useState } from "react";
import { Button } from "@/components/ui";
import { retryImageGenerationRequest } from "@/api/im";
import type { IMMessage, TranslateFn } from "@/models/conversations";

export function ImageGenerationStatus({ message, roomID, t }: { message: IMMessage; roomID?: string; t: TranslateFn }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const raw = message.metadata?.image_generation;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  const task = raw as Record<string, unknown>;
  const state = String(task.state ?? "");
  const details =
    task.error_details && typeof task.error_details === "object" && !Array.isArray(task.error_details)
      ? (task.error_details as Record<string, unknown>)
      : null;
  const detailMessage = typeof details?.message === "string" ? details.message : "";
  const diagnostics = [
    typeof details?.http_status === "number" && details.http_status > 0 ? `HTTP ${details.http_status}` : "",
    typeof details?.code === "string" ? details.code : "",
    details?.stage === "output"
      ? t("imageErrorOutputStage")
      : details?.stage === "input"
        ? t("imageErrorInputStage")
        : "",
    typeof details?.request_id === "string" && details.request_id
      ? `${t("imageErrorRequestID")}: ${details.request_id}`
      : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const retryable = state === "failed" || state === "delivery_failed";
  const pending = state === "generating" || state === "delivering";
  const errorKey = String(task.error ?? "image_generation_failed");
  const knownErrors = [
    "image_model_not_configured",
    "image_model_unavailable",
    "image_generation_failed",
    "image_generation_blocked",
    "image_generation_output_blocked",
    "image_generation_timeout",
    "image_generation_canceled",
    "image_delivery_failed",
  ];
  const label = retryable
    ? t(knownErrors.includes(errorKey) ? errorKey : "image_generation_failed")
    : t(state === "completed" ? "imageCompleted" : state === "delivering" ? "imageDelivering" : "imageGenerating");
  async function retry() {
    setBusy(true);
    setError("");
    try {
      const route = message.metadata?.image_generation_context as Record<string, unknown> | undefined;
      const result = await retryImageGenerationRequest(String(message.id ?? ""), roomID || String(route?.RoomID ?? ""));
      if (result.status !== "succeeded") setError(result.error?.message || t("imageRetryFailed"));
    } catch (err) {
      setError(err instanceof Error ? err.message : t("imageRetryFailed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="image-generation-status" role="status" aria-live="polite" aria-busy={pending || busy}>
      <span>{label}</span>
      {retryable && (detailMessage || diagnostics) ? (
        <div className="image-generation-error-details">
          {detailMessage ? <p>{detailMessage}</p> : null}
          {diagnostics ? <small>{diagnostics}</small> : null}
        </div>
      ) : null}
      {retryable ? (
        <Button size="sm" variant="secondaryGray" disabled={busy} loading={busy} onClick={() => void retry()}>
          {t(state === "delivery_failed" ? "imageRetryDelivery" : "imageRegenerate")}
        </Button>
      ) : null}
      {error && !(retryable && detailMessage) ? <span role="alert">{error}</span> : null}
      <details className="image-generation-prompt">
        <summary>{t("imageViewPrompt")}</summary>
        <p>{String(task.prompt ?? message.content ?? "")}</p>
      </details>
    </div>
  );
}
