import { memo, useId, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject } from "react";
import { ArrowUp, ChevronRight, Paperclip, Plus, RotateCcw, Square, Undo2 } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { Button, PopoverClose, PopoverContent, PopoverRoot, PopoverTrigger, Tooltip } from "@/components/ui";
import { ConnectorGitLabIcon, IconImage } from "@/components/ui/Icons";
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
import { emptyGitHubConnectorStatus } from "@/models/connectors";
import type { ConnectorConfigDraft, ConnectorStatus } from "@/models/connectors";
import type { TranslateFn } from "@/models/conversations";
import { composerActionSuggestions, type SlashPickerCandidate } from "@/models/slashCommands";
import { classNames } from "@/shared/lib/classNames";
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
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  connectorBusyAction?: string;
  connectorBusyProvider?: string;
  connectorError?: string;
  connectorPending?: boolean;
  connectorStatus?: ConnectorStatus;
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
  onConnectConnector?: () => VoidOrPromise;
  onDisconnectConnector?: () => VoidOrPromise;
  onManageConnector?: () => VoidOrPromise;
  onManageApps?: () => void;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  onRetrySend?: () => VoidOrPromise;
  onSaveConnectorConfig?: (draft: ConnectorConfigDraft) => VoidOrPromise;
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
  connectorBusyAction = "",
  connectorBusyProvider = "",
  connectorError = "",
  connectorPending = false,
  connectorStatus,
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
  onConnectConnector,
  onDisconnectConnector,
  onManageConnector,
  onManageApps,
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
  const defaultConnectorStatus = useMemo(() => emptyGitHubConnectorStatus(), []);
  const githubStatus = connectorStatus ?? defaultConnectorStatus;
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
      {workingParticipants.length > 0 ? (
        <ComposerWorkingIndicator
          participants={workingParticipants}
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
          <ComposerAddMenu
            busyAction={connectorBusyAction}
            busyProvider={connectorBusyProvider}
            disabled={composerDisabled || isSending}
            error={connectorError}
            pending={connectorPending}
            status={githubStatus}
            t={t}
            onAddFiles={() => fileInputRef.current?.click()}
            onConnect={onConnectConnector}
            onDisconnect={onDisconnectConnector}
            onManage={onManageConnector}
            onManageApps={onManageApps}
          />
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
  return (
    <div className="composer-working">
      <div className="composer-working-status" role="status" aria-live="polite">
        {participants.map((participant) => (
          <ComposerWorkingTurn
            key={participant.leaseID || participant.id || participant.name}
            participant={participant}
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
  t,
  onAction,
  onStop,
}: {
  participant: ConversationWorkingParticipant;
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
        {thinkingLatestLine ? <span className="composer-thinking-latest">{thinkingLatestLine}</span> : null}
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

type ComposerAddMenuProps = {
  busyAction: string;
  busyProvider: string;
  disabled: boolean;
  error: string;
  pending: boolean;
  status: ConnectorStatus;
  t: TranslateFn;
  onAddFiles: () => void;
  onConnect?: () => VoidOrPromise;
  onDisconnect?: () => VoidOrPromise;
  onManage?: () => VoidOrPromise;
  onManageApps?: () => void;
};

function ComposerAddMenu({
  busyAction,
  busyProvider,
  disabled,
  error,
  pending,
  status,
  t,
  onAddFiles,
  onConnect,
  onDisconnect,
  onManage,
  onManageApps,
}: ComposerAddMenuProps) {
  const [popoverOpen, setPopoverOpen] = useState(false);
  const accountLabel = status.account?.login || status.account?.name || "";
  const connectorStateLabel =
    status.connected && accountLabel
      ? accountLabel
      : status.connected
        ? t("connectorConnected")
        : t("connectorNotConnected");
  const hasConnectedConnector = status.connected;
  const githubBusy = pending || (busyProvider !== "gitlab" && busyAction === "connect");

  function handleConnectGitHub() {
    void onConnect?.();
  }

  function handleDisconnectGitHub() {
    void onDisconnect?.();
  }

  function handleManageGitHub() {
    void onManage?.();
  }

  return (
    <>
      <PopoverRoot open={popoverOpen} onOpenChange={setPopoverOpen}>
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
        <PopoverContent
          aria-label={t("composerAddContent")}
          className={classNames("composer-add-popover", hasConnectedConnector ? "is-wide" : "is-compact")}
          role="dialog"
          side="top"
        >
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
          <div className="composer-add-separator" />
          <section className="composer-add-section" aria-label={t("composerConnectors")}>
            <div className="composer-add-section-label">{t("composerConnectors")}</div>
            <div className="connector-provider-row">
              <div className="connector-provider-main">
                <span className="connector-provider-icon" aria-hidden="true">
                  {IconImage("github")}
                </span>
                <div className="connector-provider-copy">
                  <strong>{t("connectorGitHub")}</strong>
                  <span>{connectorStateLabel}</span>
                </div>
              </div>
              {status.connected ? (
                <div className="connector-provider-actions">
                  <span className="connector-connected-state">{t("connectorConnected")}</span>
                  <div className="connector-provider-action-buttons">
                    {status.app_manageable ? (
                      <Button
                        aria-busy={busyAction === "manage" ? true : undefined}
                        className="connector-manage-button"
                        loading={busyAction === "manage"}
                        size="sm"
                        variant="secondaryGray"
                        onClick={handleManageGitHub}
                      >
                        {t("connectorManage")}
                      </Button>
                    ) : null}
                    <Button
                      aria-busy={busyAction === "disconnect" ? true : undefined}
                      className="connector-disconnect-button connector-disconnect-button-danger"
                      loading={busyAction === "disconnect"}
                      size="sm"
                      variant="outlineDanger"
                      onClick={handleDisconnectGitHub}
                    >
                      {t("connectorDisconnect")}
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  aria-busy={githubBusy ? true : undefined}
                  className="connector-connect-button"
                  loading={githubBusy}
                  size="sm"
                  variant="tertiaryGray"
                  onClick={handleConnectGitHub}
                >
                  {t("connectorConnect")}
                </Button>
              )}
            </div>
            <div className="connector-provider-row">
              <div className="connector-provider-main">
                <span className="connector-provider-icon" aria-hidden="true">
                  <ConnectorGitLabIcon size={16} />
                </span>
                <div className="connector-provider-copy">
                  <strong>{t("connectorGitLab")}</strong>
                  <span>{t("appManageInAgent")}</span>
                </div>
              </div>
              <Button
                className="connector-connect-button"
                size="sm"
                variant="tertiaryGray"
                disabled={!onManageApps}
                onClick={() => {
                  setPopoverOpen(false);
                  onManageApps?.();
                }}
              >
                {t("appOpenApps")}
              </Button>
            </div>
            {pending ? (
              <div className="connector-pending" role="status">
                {t("connectorOAuthPending")}
              </div>
            ) : null}
            {error ? <div className="form-error connector-form-error">{error}</div> : null}
          </section>
        </PopoverContent>
      </PopoverRoot>
    </>
  );
}
