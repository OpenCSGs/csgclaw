import generatedMediaStyles from "./GeneratedMedia.module.css";
import { useState } from "react";
import type { ReactNode } from "react";
import { Button, MediaGeneration } from "@/components/ui";
import { retryImageGenerationRequest } from "@/api/im";
import { localizeAPIError } from "@/shared/i18n";
import type { IMMessage, TranslateFn } from "@/models/conversations";

export function ImageGenerationStatus({
  message,
  roomID,
  t,
  result,
}: {
  message: IMMessage;
  roomID?: string;
  t: TranslateFn;
  result?: ReactNode;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const raw = message.metadata?.image_generation;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return result ?? null;
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
      setError(localizeAPIError(err, t, t("imageRetryFailed")));
    } finally {
      setBusy(false);
    }
  }
  return (
    <MediaGeneration
      kind="image"
      busy={busy || state === "generating" || state === "delivering"}
      state={retryable ? "failed" : state === "completed" ? "completed" : "pending"}
      label={label}
      result={result}
      promptDisclosureLabel={t("imageViewPrompt")}
      prompt={String(task.prompt ?? message.content ?? "")}
      failureMessage={detailMessage}
      feedback={error ? <span role="alert">{error}</span> : null}
      details={
        retryable && diagnostics ? (
          <div className={generatedMediaStyles["image-generation-error-details"]}>
            <small>{diagnostics}</small>
          </div>
        ) : null
      }
      actions={
        retryable ? (
          <>
            <Button size="sm" variant="secondaryGray" disabled={busy} loading={busy} onClick={() => void retry()}>
              {t(state === "delivery_failed" ? "imageRetryDelivery" : "imageRegenerate")}
            </Button>
          </>
        ) : null
      }
    />
  );
}
