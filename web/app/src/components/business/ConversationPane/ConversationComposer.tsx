import { ContextUsageRing } from "./ContextUsageRing";
import { memo, useEffect, useId, useMemo, useRef, useState } from "react";
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, ReactNode, PointerEvent as ReactPointerEvent, RefObject } from "react";
import { ArrowUp, ChevronDown, ChevronRight, Paperclip, Plus, RotateCcw, Square, Undo2 } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { Button, PopoverClose, PopoverContent, PopoverRoot, PopoverTrigger, Tooltip } from "@/components/ui";
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

const WORKING_STATUS_LINGER_MS = 3500;
const WORKING_PROCESS_DEFAULT_HEIGHT = 104;
const WORKING_PROCESS_MIN_HEIGHT = 64;
const WORKING_PROCESS_MAX_HEIGHT = 280;

export type ConversationComposerProps = {
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
  slashPickerLoading: boolean;
  slashPickerOpen: boolean;
  t: TranslateFn;
  workingParticipants?: ConversationWorkingParticipant[];
};

export const ConversationComposer = memo(function ConversationComposer({
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
  slashPickerLoading,
  slashPickerOpen,
  t,
  workingParticipants = [],
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
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const composerHelpId = useId();
  const isSending = sendStatus === "sending";
  const interactionDisabled = composerDisabled || isSending;
  const sendDisabled = interactionDisabled || (!draftText.trim() && attachmentDrafts.length === 0);
  const actionSuggestions = useMemo(() => composerActionSuggestions(draftText), [draftText]);
  const lingeredWorkingParticipants = useLingeringWorkingParticipants(workingParticipants);
  const visibleWorkingParticipants =
    workingParticipants.length > 0 ? workingParticipants : disableCompletedWorkingActions(lingeredWorkingParticipants);

  function handleFiles(files: File[]) {
    if (interactionDisabled || files.length === 0) {
      return;
    }
    onAddAttachments(files);
  }

  return (
    <footer className={`composer${visibleWorkingParticipants.length > 0 ? " has-working-status" : ""}`}>
      {slashPickerOpen ? (
        <SlashPicker
          candidates={slashCandidates}
          activeIndex={slashIndex}
          loading={slashPickerLoading}
          t={t}
          onSelect={(name) => onApplySlashCandidate(name)}
        />
      ) : null}
      {mentionCandidates.length > 0 ? (
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
      {visibleWorkingParticipants.length > 0 ? (
        <ComposerWorkingIndicator
          participants={visibleWorkingParticipants}
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
            onInput={onSyncComposer}
            onClick={onSyncComposer}
            onKeyDown={onComposerKeyDown}
            onCompositionStart={onComposerCompositionStart}
            onCompositionEnd={onComposerCompositionEnd}
            onKeyUp={onSyncComposer}
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
              onSyncComposer();
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
            <Tooltip content={isSending ? t("stopSending") : t("send")}>
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

function useLingeringWorkingParticipants(
  participants: readonly ConversationWorkingParticipant[],
): ConversationWorkingParticipant[] {
  const [visibleParticipants, setVisibleParticipants] = useState<ConversationWorkingParticipant[]>([]);

  useEffect(() => {
    if (participants.length > 0) {
      setVisibleParticipants([...participants]);
      return undefined;
    }
    if (visibleParticipants.length === 0) {
      return undefined;
    }
    const timer = window.setTimeout(() => setVisibleParticipants([]), WORKING_STATUS_LINGER_MS);
    return () => window.clearTimeout(timer);
  }, [participants, visibleParticipants.length]);

  return visibleParticipants;
}

function disableCompletedWorkingActions(
  participants: readonly ConversationWorkingParticipant[],
): ConversationWorkingParticipant[] {
  return participants.map((participant) => ({
    ...participant,
    canStop: false,
    stopSending: false,
    stopping: false,
  }));
}

function ComposerWorkingIndicator({
  participants,
  t,
  onAction,
  onStop,
}: {
  participants: readonly ConversationWorkingParticipant[];
  t: TranslateFn;
  onAction?: (participant?: ConversationWorkingParticipant) => void;
  onStop?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
}) {
  const [expanded, setExpanded] = useState(true);
  const [processHeight, setProcessHeight] = useState(WORKING_PROCESS_DEFAULT_HEIGHT);
  const activeCount = participants.length;
  const turnKey = participants.map((participant) => participant.leaseID || participant.requestID || participant.id).join("|");
  const latestSummary = participants
    .map((participant) => participant.activity?.summary?.trim() || latestThinkingLine(participant.thinkingText || ""))
    .find(Boolean);

  useEffect(() => {
    setExpanded(true);
  }, [turnKey]);

  function handleResizePointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    const startY = event.clientY;
    const startHeight = processHeight;
    const handlePointerMove = (moveEvent: PointerEvent) => {
      setProcessHeight(clampWorkingProcessHeight(startHeight + startY - moveEvent.clientY));
    };
    const handlePointerUp = () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
    };
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
  }

  function handleResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowUp" || event.key === "ArrowDown") {
      event.preventDefault();
      setProcessHeight((height) => clampWorkingProcessHeight(height + (event.key === "ArrowUp" ? 16 : -16)));
      return;
    }
    if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      setProcessHeight(event.key === "Home" ? WORKING_PROCESS_MIN_HEIGHT : WORKING_PROCESS_MAX_HEIGHT);
    }
  }

  return (
    <div
      className={`composer-working${expanded ? "" : " is-collapsed"}`}
      style={{ "--composer-thinking-transcript-max-height": `${processHeight}px` } as CSSProperties}
    >
      {expanded ? (
        <>
          <div
            className="composer-working-resize-handle"
            role="separator"
            aria-label={t("conversationWorkingResizeProcess")}
            aria-orientation="horizontal"
            aria-valuemin={WORKING_PROCESS_MIN_HEIGHT}
            aria-valuemax={WORKING_PROCESS_MAX_HEIGHT}
            aria-valuenow={processHeight}
            tabIndex={0}
            onKeyDown={handleResizeKeyDown}
            onPointerDown={handleResizePointerDown}
          />
          <div className="composer-working-header">
            <div className="composer-working-heading">
              <span className="composer-working-toggle-title">{t("conversationWorkingProcessTitle")}</span>
              <span className="composer-working-toggle-subtitle">
                {latestSummary || t("conversationWorkingProcessSubtitle", { count: activeCount })}
              </span>
            </div>
            <div className="composer-working-actions">
              <span className="composer-working-toggle-count">
                {t("conversationWorkingProcessCount", { count: activeCount })}
              </span>
              {onAction ? (
                <button type="button" className="composer-working-activity-button" onClick={() => onAction()}>
                  {t("conversationWorkingActivityDrawer")}
                </button>
              ) : null}
              <button
                type="button"
                className="composer-working-toggle-button"
                aria-expanded="true"
                onClick={() => setExpanded(false)}
              >
                {t("conversationWorkingCollapseProcess")}
                <ChevronDown aria-hidden="true" className="composer-working-toggle-icon" size={16} />
              </button>
            </div>
          </div>
        </>
      ) : null}
      <div className="composer-working-status" role="status" aria-live="polite">
        {participants.map((participant, index) => (
          <ComposerWorkingTurn
            key={participant.leaseID || participant.id || participant.name}
            participant={participant}
            expandControl={
              !expanded && index === 0 ? (
                <button
                  type="button"
                  className="composer-working-toggle-button"
                  aria-expanded="false"
                  onClick={() => setExpanded(true)}
                >
                  {t("conversationWorkingExpandProcess")}
                  <ChevronDown aria-hidden="true" className="composer-working-toggle-icon" size={16} />
                </button>
              ) : null
            }
            showDetails={expanded}
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
  participant,
  expandControl,
  showDetails,
  t,
  onAction,
  onStop,
}: {
  participant: ConversationWorkingParticipant;
  expandControl?: ReactNode;
  showDetails?: boolean;
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
  const stopLabel = participant.stopping
    ? t("conversationWorkingStopping")
    : participant.stopSending
      ? t("conversationWorkingStopSending")
      : t("conversationWorkingStop");
  const summary = participant.activity?.summary?.trim() || "";
  const activityDetail = participant.activity?.detail?.trim() || "";
  const thinkingText = participant.thinkingText;
  const thinkingLatestLine = thinkingText === undefined ? "" : latestThinkingLine(thinkingText);
  const processDetails = processDetailBlocks(thinkingText, activityDetail, thinkingText || activityDetail ? undefined : summary);
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
        {participant.showContextUsage || participant.contextUsage ? (
          <ContextUsageRing usage={participant.contextUsage} t={t} />
        ) : null}
        {participant.canStop && onStop ? (
          <Tooltip content={stopLabel} contentProps={{ side: "top", sideOffset: 6 }}>
            <button
              type="button"
              className="composer-working-stop"
              aria-label={t("conversationWorkingStopAria", { name: participant.name })}
              disabled={participant.stopSending || participant.stopping}
              onClick={() => void onStop(participant)}
            >
              <span className="composer-working-stop-icon" aria-hidden="true" />
            </button>
          </Tooltip>
        ) : null}
        {participant.contextUsage?.compacting || thinkingLatestLine ? (
          <span className="composer-thinking-latest">
            {participant.contextUsage?.compacting ? t("contextCompacting") : thinkingLatestLine}
          </span>
        ) : null}
        {expandControl}
      </div>
      {showDetails && processDetails.length > 0 ? (
        <div className="composer-thinking-transcript">
          {processDetails.map((detail) => (
            <div key={detail} className="composer-thinking-transcript-block">
              {detail}
            </div>
          ))}
        </div>
      ) : null}
      {participant.stopError ? <div className="composer-working-error">{participant.stopError}</div> : null}
    </div>
  );
}

function trimThinkingText(text: string): string {
  return text.replace(/\r\n?/g, "\n").trim();
}

function processDetailBlocks(...values: Array<string | undefined>): string[] {
  const seen = new Set<string>();
  const blocks: string[] = [];
  values.forEach((value) => {
    const normalized = trimThinkingText(value || "");
    if (!normalized || seen.has(normalized)) {
      return;
    }
    seen.add(normalized);
    blocks.push(normalized);
  });
  return blocks;
}

function clampWorkingProcessHeight(height: number): number {
  return Math.min(WORKING_PROCESS_MAX_HEIGHT, Math.max(WORKING_PROCESS_MIN_HEIGHT, height));
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

type ComposerAddMenuProps = {
  disabled: boolean;
  t: TranslateFn;
  onAddFiles: () => void;
};

function ComposerAddMenu({ disabled, t, onAddFiles }: ComposerAddMenuProps) {
  return (
    <PopoverRoot>
      <Tooltip content={t("composerAddContent")}>
        <PopoverTrigger asChild>
          <span>
            <Button
              aria-haspopup="dialog"
              aria-label={t("composerAddContent")}
              className="composer-add-button"
              disabled={disabled}
              iconOnly
              size="lg"
              variant="tertiaryGray"
            >
              <Plus aria-hidden="true" size={24} strokeWidth={1.8} />
            </Button>
          </span>
        </PopoverTrigger>
      </Tooltip>
      <PopoverContent aria-label={t("composerAddContent")} className="composer-add-popover" role="dialog" side="top">
        <section className="composer-add-section" aria-label={t("composerAdd")}>
          <div className="composer-add-section-label">{t("composerAdd")}</div>
          <PopoverClose asChild>
            <button
              type="button"
              className="composer-add-menu-item"
              aria-label={t("addAttachment")}
              title={t("addAttachment")}
              onClick={onAddFiles}
            >
              <Paperclip aria-hidden="true" size={19} />
              <span>{t("addAttachment")}</span>
            </button>
          </PopoverClose>
        </section>
      </PopoverContent>
    </PopoverRoot>
  );
}
