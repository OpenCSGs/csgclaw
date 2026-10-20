import { ComposerAddMenu } from "./ComposerAddMenu";
import type { ChatFileScope } from "@/components/business/ChatSidePanel/types";
import { useFileMentions } from "./FileMentionPicker";
import type { SkillContinuation } from "@/models/slashCommands";
import { WorkingTurnControls } from "./WorkingTurnControls";
import { memo, useId, useMemo, useRef } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject, ReactNode } from "react";
import { ArrowUp, ChevronRight, RotateCcw, Square, Undo2 } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { Button, Tooltip } from "@/components/ui";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
import type { AgentProfileLike } from "@/models/agents";
import type { AttachmentDraft } from "@/models/attachments";
import { providerNeedsAuth } from "@/models/agents";
import {
  insertComposerSegmentsAtSelection,
  insertPlainTextAtSelection,
  normalizeTextMentions,
  type ComposerMentionUser,
  type ComposerSegment,
} from "@/models/composer";
import type { TranslateFn } from "@/models/conversations";
import { composerActionSuggestions, type SlashPickerCandidate } from "@/models/slashCommands";
import { MentionPicker } from "./MentionPicker";
import { SlashPicker } from "./SlashPicker";
import { AttachmentDraftStrip } from "./ConversationAttachments";
import { dataTransferHasFiles, filesFromDataTransfer } from "./attachmentFiles";
import {
  ConversationWorkingActions,
  type ComposerSendStatus,
  type ConversationWorkingAction,
  type ConversationWorkingParticipant,
  type MentionPickerUser,
  type VoidOrPromise,
} from "./types";

export type ConversationComposerProps = {
  fileScope?: ChatFileScope;
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  composerDisabled: boolean;
  composerDisabledReason?: string;
  composerError: string;
  draftSegments: ComposerSegment[];
  draftText: string;
  attachmentDrafts?: AttachmentDraft[];
  removedAttachmentCount?: number;
  removedAttachmentName?: string;
  sendError?: string;
  sendProgress?: number;
  sendStatus?: ComposerSendStatus;
  editorRef: RefObject<HTMLDivElement | null>;
  managerProfile?: AgentProfileLike | null;
  managerProvider: string;
  mentionCandidates: MentionPickerUser[];
  mentionIndex: number;
  mentionableUsersByName: Map<string, ComposerMentionUser>;
  onApplyMention: (user: MentionPickerUser) => void;
  onApplySlashCandidate: (name: string) => void;
  onAddAttachments?: (files: File[]) => void;
  onComposerCompositionEnd: () => void;
  onComposerCompositionStart: () => void;
  onComposerKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  onRetrySend?: () => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onStopSend?: () => void;
  onUndoRemoveAttachment?: () => void;
  onStopWorkingTurn?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
  onRemoveAttachment?: (id: string) => void;
  onSyncComposer: () => void;
  onWorkingAction?: (participant?: ConversationWorkingParticipant) => void;
  slashCandidates: SlashPickerCandidate[];
  slashIndex: number;
  slashContinuation?: SkillContinuation;
  slashPickerLoading: boolean;
  slashPickerOpen: boolean;
  t: TranslateFn;
  workingParticipants?: ConversationWorkingParticipant[];
  toolbar?: ReactNode;
  hideWorkingContextUsage?: boolean;
};

export const ConversationComposer = memo(function ConversationComposer({
  fileScope,
  authBusyProvider,
  authStatuses,
  composerDisabled,
  composerDisabledReason = "",
  composerError,
  draftSegments,
  draftText,
  attachmentDrafts = [],
  removedAttachmentCount = 0,
  removedAttachmentName = "",
  sendError = "",
  sendProgress = 0,
  sendStatus = "idle",
  editorRef,
  managerProfile,
  managerProvider,
  mentionCandidates,
  mentionIndex,
  mentionableUsersByName,
  slashCandidates,
  slashIndex,
  slashContinuation,
  slashPickerLoading,
  slashPickerOpen,
  t,
  workingParticipants = [],
  toolbar,
  hideWorkingContextUsage = false,
  onApplyMention,
  onApplySlashCandidate,
  onAddAttachments = () => {},
  onComposerCompositionEnd,
  onComposerCompositionStart,
  onComposerKeyDown,
  onProviderLogin,
  onPreviewAttachment,
  onRetrySend,
  onRemoveAttachment = () => {},
  onSendMessage,
  onStopSend,
  onStopWorkingTurn,
  onSyncComposer,
  onUndoRemoveAttachment,
  onWorkingAction,
}: ConversationComposerProps) {
  const fileMentions = useFileMentions({
    scope: fileScope,
    editorRef,
    onSync: onSyncComposer,
    people: mentionCandidates,
    personIndex: mentionIndex,
    onPerson: onApplyMention,
    t,
  });
  const sync = () => {
    onSyncComposer();
    fileMentions.sync();
  };
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const composerHelpId = useId();
  const isSending = sendStatus === "sending";
  const interactionDisabled = composerDisabled || isSending;
  const sendDisabled = interactionDisabled || (!draftText.trim() && attachmentDrafts.length === 0);
  const actionSuggestions = useMemo(() => composerActionSuggestions(draftText), [draftText]);

  function handleFiles(files: File[]) {
    if (interactionDisabled || files.length === 0) {
      return;
    }
    onAddAttachments(files);
  }

  return (
    <footer className={`composer${workingParticipants.length > 0 ? " has-working-status" : ""}`}>
      {slashPickerOpen ? (
        <SlashPicker
          candidates={slashCandidates}
          activeIndex={slashIndex}
          continuation={slashContinuation}
          loading={slashPickerLoading}
          t={t}
          onSelect={(name) => onApplySlashCandidate(name)}
        />
      ) : null}
      {fileScope ? (
        fileMentions.picker
      ) : mentionCandidates.length > 0 ? (
        <MentionPicker users={mentionCandidates} activeIndex={mentionIndex} t={t} onSelect={onApplyMention} />
      ) : null}
      {managerProfile &&
      providerNeedsAuth(managerProfile.provider) &&
      authStatuses[managerProvider]?.authenticated === false ? (
        <CLIProxyAuthControl
          provider={managerProfile.provider}
          t={t}
          status={authStatuses[managerProvider]}
          busy={authBusyProvider === managerProvider}
          onLogin={onProviderLogin}
        />
      ) : null}
      {workingParticipants.length > 0 ? (
        <ComposerWorkingIndicator
          participants={workingParticipants}
          hideContextUsage={hideWorkingContextUsage}
          t={t}
          onAction={onWorkingAction}
          onStop={onStopWorkingTurn}
        />
      ) : null}
      <div
        className="composer-box"
        onDragOver={(event) => {
          if (interactionDisabled || !dataTransferHasFiles(event.dataTransfer)) {
            return;
          }
          event.preventDefault();
          event.dataTransfer.dropEffect = "copy";
        }}
        onDrop={(event) => {
          const files = filesFromDataTransfer(event.dataTransfer);
          if (files.length === 0) {
            return;
          }
          event.preventDefault();
          handleFiles(files);
        }}
      >
        <AttachmentDraftStrip
          drafts={attachmentDrafts}
          progress={sendProgress}
          status={sendStatus === "sending" ? "uploading" : sendStatus === "failed" ? "failed" : "idle"}
          t={t}
          onPreviewAttachment={onPreviewAttachment}
          onRemove={onRemoveAttachment}
        />
        <div className="composer-editor-wrap">
          {draftSegments.length === 0 ? (
            <div className="composer-placeholder" aria-hidden="true">
              {composerDisabled ? composerDisabledReason || t("profileIncomplete") : t("inputPlaceholder")}
            </div>
          ) : null}
          <div
            ref={editorRef}
            className={`composer-editor ${interactionDisabled ? "disabled" : ""}`}
            contentEditable={interactionDisabled ? "false" : "true"}
            tabIndex={interactionDisabled ? -1 : 0}
            suppressContentEditableWarning={true}
            role="textbox"
            aria-multiline="true"
            aria-label={t("inputPlaceholder")}
            aria-describedby={composerHelpId}
            aria-disabled={interactionDisabled}
            onInput={sync}
            onClick={sync}
            onKeyDown={(event) => {
              if (!fileMentions.handleKeyDown(event)) onComposerKeyDown(event);
            }}
            onCompositionStart={onComposerCompositionStart}
            onCompositionEnd={onComposerCompositionEnd}
            onKeyUp={sync}
            onPaste={(event) => {
              const files = filesFromDataTransfer(event.clipboardData);
              const pasted = event.clipboardData?.getData("text/plain") ?? "";
              if (files.length > 0) {
                event.preventDefault();
                handleFiles(files);
                if (!pasted) {
                  return;
                }
              } else {
                event.preventDefault();
              }
              const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByName);
              if (segments.some((segment) => segment.type === "mention")) {
                insertComposerSegmentsAtSelection(segments);
              } else {
                insertPlainTextAtSelection(pasted);
              }
              sync();
            }}
          />
        </div>
        {actionSuggestions.length > 0 ? (
          <div className="composer-action-suggestions" aria-label={t("suggestedActions")}>
            <span>{t("suggestedActions")}</span>
            {actionSuggestions.map((suggestion) => (
              <button
                key={suggestion.name}
                type="button"
                className="composer-action-suggestion"
                title={suggestion.description}
                onClick={() => onApplySlashCandidate(suggestion.name)}
              >
                /{suggestion.name}
              </button>
            ))}
            <small>{t("suggestedActionsOnly")}</small>
          </div>
        ) : null}
        <div className="composer-toolbar">
          <ComposerAddMenu disabled={interactionDisabled} t={t} onAddFiles={() => fileInputRef.current?.click()} />
          <input
            ref={fileInputRef}
            className="sr-only"
            type="file"
            multiple
            aria-label={t("addAttachment")}
            onChange={(event) => {
              handleFiles(Array.from(event.currentTarget.files || []));
              event.currentTarget.value = "";
            }}
          />
          {toolbar}
          <span id={composerHelpId} className="sr-only">
            {t("composerTip")}
          </span>
          <div className="composer-toolbar-actions">
            {isSending ? (
              <span className="composer-send-state" role="status" aria-live="polite">
                {attachmentDrafts.length > 0
                  ? t("sendingWithProgress", { progress: Math.round(sendProgress) })
                  : t("sending")}
              </span>
            ) : null}
            {sendStatus === "failed" && onRetrySend ? (
              <Tooltip content={t("retrySend")}>
                <Button
                  aria-label={t("retrySend")}
                  className="composer-retry-button"
                  iconOnly
                  size="sm"
                  variant="tertiaryGray"
                  onClick={onRetrySend}
                >
                  <RotateCcw aria-hidden="true" size={16} />
                </Button>
              </Tooltip>
            ) : null}
            <Tooltip content={isSending ? t("stopSending") : t("send")} contentProps={{ side: "top" }}>
              <span>
                <Button
                  variant="primary"
                  className={`composer-send-button${isSending ? " is-stopping" : ""}`}
                  aria-label={isSending ? t("stopSending") : t("send")}
                  disabled={isSending ? !onStopSend : sendDisabled}
                  iconOnly
                  size="lg"
                  onClick={isSending ? onStopSend : onSendMessage}
                >
                  {isSending ? (
                    <Square aria-hidden="true" size={16} fill="currentColor" />
                  ) : (
                    <ArrowUp aria-hidden="true" size={22} strokeWidth={2.25} />
                  )}
                </Button>
              </span>
            </Tooltip>
          </div>
        </div>
      </div>
      {removedAttachmentCount > 0 && onUndoRemoveAttachment ? (
        <div className="composer-feedback-row" role="status">
          <span>
            {removedAttachmentCount === 1
              ? t("attachmentRemoved", { name: removedAttachmentName })
              : t("attachmentsRemoved", { count: removedAttachmentCount })}
          </span>
          <Button size="sm" variant="secondaryGray" onClick={onUndoRemoveAttachment}>
            <Undo2 aria-hidden="true" size={14} />
            {t("undo")}
          </Button>
        </div>
      ) : null}
      {composerError || sendError ? (
        <div className="form-error composer-error" role="alert">
          {sendError || composerError}
        </div>
      ) : null}
    </footer>
  );
});

function ComposerWorkingIndicator({
  hideContextUsage,
  participants,
  t,
  onAction,
  onStop,
}: {
  participants: readonly ConversationWorkingParticipant[];
  hideContextUsage?: boolean;
  t: TranslateFn;
  onAction?: (participant?: ConversationWorkingParticipant) => void;
  onStop?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
}) {
  return (
    <div className="composer-working">
      <div className="composer-working-status" role="status" aria-live="polite">
        {participants.map((participant) => (
          <ComposerWorkingTurn
            key={participant.leaseID || participant.id || participant.name}
            participant={participant}
            hideContextUsage={hideContextUsage}
            t={t}
            onAction={onAction}
            onStop={onStop}
          />
        ))}
      </div>
    </div>
  );
}

function ComposerWorkingTurn({
  hideContextUsage,
  participant,
  t,
  onAction,
  onStop,
}: {
  participant: ConversationWorkingParticipant;
  hideContextUsage?: boolean;
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
      <strong className="composer-working-name">{participant.name}</strong>
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
            className="composer-working-item"
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
          <div className="composer-working-item" data-working-action={action}>
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
