import { useState } from "react";
import type { ReactNode } from "react";
import { Button, MediaGeneration } from "@/components/ui";
import { apiErrorCode } from "@/api/client";
import { retryImageGenerationRequest } from "@/api/im";
import type { IMMessage, TranslateFn } from "@/models/conversations";

const imageErrors = new Set([
  "image_model_not_configured",
  "image_model_unavailable",
  "image_generation_failed",
  "image_generation_blocked",
  "image_generation_output_blocked",
  "image_generation_timeout",
  "image_generation_canceled",
  "image_delivery_failed",
]);

function imageRetryError(error: { code?: string; message?: string } | undefined, t: TranslateFn): string {
  for (const code of [error?.code, error?.message]) {
    if (code && imageErrors.has(code)) return t(code);
  }
  if (error?.code === "image_reference_unavailable") return t("errors.image_reference_unavailable");
  return t("imageRetryFailed");
}

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
  const failed = state === "failed" || state === "delivery_failed";
  const errorKey = String(task.error ?? "image_generation_failed");
  if (state === "failed" && errorKey === "image_model_not_configured") {
    return result ?? <p>{t(errorKey)}</p>;
  }
  const label = failed
    ? t(imageErrors.has(errorKey) ? errorKey : "image_generation_failed")
    : t(state === "completed" ? "imageCompleted" : state === "delivering" ? "imageDelivering" : "imageGenerating");
  async function retry() {
    setBusy(true);
    setError("");
    try {
      const route = message.metadata?.image_generation_context as Record<string, unknown> | undefined;
      const result = await retryImageGenerationRequest(String(message.id ?? ""), roomID || String(route?.RoomID ?? ""));
      if (result.status !== "succeeded") setError(imageRetryError(result.error, t));
    } catch (err) {
      setError(imageRetryError({ code: apiErrorCode(err) }, t));
    } finally {
      setBusy(false);
    }
  }
  return (
    <MediaGeneration
      kind="image"
      busy={busy || state === "generating" || state === "delivering"}
      state={failed ? "failed" : state === "completed" ? "completed" : "pending"}
      label={label}
      result={result}
      promptDisclosureLabel={t("imageViewPrompt")}
      prompt={String(task.prompt ?? message.content ?? "")}
      feedback={error ? <span role="alert">{error}</span> : null}
      actions={
        failed ? (
          <Button size="sm" variant="secondaryGray" disabled={busy} loading={busy} onClick={() => void retry()}>
            {t(state === "delivery_failed" ? "imageRetryDelivery" : "imageRegenerate")}
          </Button>
        ) : null
      }
    />
  );
}
