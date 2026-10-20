import { ChevronRight } from "lucide-react";
import { WorkingTurnControls } from "../WorkingTurnControls";
import type { TranslateFn } from "@/models/conversations";
import {
  ConversationWorkingActions,
  type ConversationWorkingAction,
  type ConversationWorkingParticipant,
  type VoidOrPromise,
} from "../types";
import styles from "./WorkingTurnStatus.module.css";

export function WorkingTurnStatus({
  hideContextUsage,
  showName = true,
  participant,
  t,
  onAction,
  onStop,
}: {
  participant: ConversationWorkingParticipant;
  hideContextUsage?: boolean;
  showName?: boolean;
  t: TranslateFn;
  onAction?: (participant?: ConversationWorkingParticipant) => void;
  onStop?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
}) {
  const action =
    participant.activity?.action ||
    (participant.thinkingText?.trim()
      ? ConversationWorkingActions.thinking
      : ConversationWorkingActions.preparingReply);
  const toolName =
    participant.stopping || participant.stopSending ? "" : (participant.activity?.toolName?.trim() ?? "");
  const actionLabel = participant.stopping
    ? t("conversationWorkingStopping")
    : participant.stopSending
      ? t("conversationWorkingStopSending")
      : toolName || workingActionLabel(action, t);
  const summary = participant.activity?.summary?.trim() || "";
  const thinkingText = participant.thinkingText;
  const thinkingLatestLine = thinkingText === undefined ? "" : latestThinkingLine(thinkingText);
  const content = (
    <>
      <span className="composer-working-dots" aria-hidden="true">
        <span />
        <span />
        <span />
      </span>
      {showName ? <strong className="composer-working-name">{participant.name}</strong> : null}
      <span className={`composer-working-verb${toolName ? " is-tool" : ""}`}>
        {actionLabel}
        {toolName && summary ? <ChevronRight aria-hidden="true" size={12} strokeWidth={2} /> : null}
      </span>
      {summary ? <span className="composer-working-summary">{summary}</span> : null}
    </>
  );

  return (
    <div className={`composer-working-turn${participant.stopping ? " is-stopping" : ""}`}>
      <div className="composer-working-row">
        {onAction ? (
          <button
            type="button"
            className={`composer-working-item${showName ? "" : ` ${styles.withoutName}`}`}
            data-working-action={action}
            aria-label={t("conversationWorkingOpenActivity", {
              detail: summary || actionLabel,
              name: participant.name,
            })}
            title={summary || actionLabel}
            onClick={() => onAction(participant)}
          >
            {content}
          </button>
        ) : (
          <div
            className={`composer-working-item${showName ? "" : ` ${styles.withoutName}`}`}
            data-working-action={action}
          >
            {content}
          </div>
        )}
        <WorkingTurnControls participant={participant} t={t} onStop={onStop} hideContextUsage={hideContextUsage} />
        {participant.contextUsage?.compacting || thinkingLatestLine ? (
          <span className="composer-thinking-latest">
            {participant.contextUsage?.compacting ? t("contextCompacting") : thinkingLatestLine}
          </span>
        ) : null}
      </div>
      {participant.stopError ? <div className="composer-working-error">{participant.stopError}</div> : null}
    </div>
  );
}

function latestThinkingLine(text: string): string {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index].trim();
    if (line) {
      return line;
    }
  }
  return "";
}

function workingActionLabel(action: ConversationWorkingAction, t: TranslateFn): string {
  switch (action) {
    case ConversationWorkingActions.editing:
      return t("conversationWorkingEditing");
    case ConversationWorkingActions.generatingReply:
      return t("conversationWorkingGeneratingReply");
    case ConversationWorkingActions.preparingReply:
      return t("conversationWorkingPreparingReply");
    case ConversationWorkingActions.reading:
      return t("conversationWorkingReading");
    case ConversationWorkingActions.replying:
      return t("conversationWorkingReplying");
    case ConversationWorkingActions.running:
      return t("conversationWorkingRunning");
    case ConversationWorkingActions.searching:
      return t("conversationWorkingSearching");
    case ConversationWorkingActions.usingTool:
      return t("conversationWorkingUsingTool");
    case ConversationWorkingActions.waiting:
      return t("conversationWorkingWaiting");
    default:
      return t("conversationWorkingThinking");
  }
}
