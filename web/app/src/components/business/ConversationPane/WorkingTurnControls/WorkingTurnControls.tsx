import { Tooltip } from "@/components/ui";
import { ContextUsageRing } from "../ContextUsageRing";
import type { TranslateFn } from "@/models/conversations";
import type { ConversationWorkingParticipant, VoidOrPromise } from "../types";
import styles from "./WorkingTurnControls.module.css";

export function WorkingTurnControls({
  participant,
  t,
  onStop,
  placement = "inline",
}: {
  participant: ConversationWorkingParticipant;
  t: TranslateFn;
  onStop?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
  placement?: "inline" | "message";
}) {
  const busy = participant.stopping || participant.stopSending;
  const stopLabel = participant.stopping
    ? t("conversationWorkingStopping")
    : participant.stopSending
      ? t("conversationWorkingStopSending")
      : t("conversationWorkingStop");
  const message = placement === "message";
  if (
    message &&
    !busy &&
    !participant.contextUsage?.compacting &&
    !(participant.canStop && onStop) &&
    !participant.stopError
  )
    return null;
  const content = (
    <>
      {!message && (participant.showContextUsage || participant.contextUsage) ? (
        <ContextUsageRing usage={participant.contextUsage} t={t} />
      ) : null}
      {participant.canStop && onStop ? (
        <Tooltip content={stopLabel} contentProps={{ side: "top", sideOffset: 6 }}>
          <button
            type="button"
            className="composer-working-stop"
            aria-label={t("conversationWorkingStopAria", { name: participant.name })}
            disabled={busy}
            onClick={() => void onStop(participant)}
          >
            <span className="composer-working-stop-icon" aria-hidden="true" />
          </button>
        </Tooltip>
      ) : null}
      {message && (busy || participant.contextUsage?.compacting) ? (
        <span className={styles.status} role="status">
          {busy ? stopLabel : t("contextCompacting")}
        </span>
      ) : null}
      {message && participant.stopError ? (
        <span className={styles.error} role="alert">
          {participant.stopError}
        </span>
      ) : null}
    </>
  );
  return message ? (
    <div className={styles.message} role="group" aria-label={t("progressControls", { name: participant.name })}>
      {content}
    </div>
  ) : (
    content
  );
}
