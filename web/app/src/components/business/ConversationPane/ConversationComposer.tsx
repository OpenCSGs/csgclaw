import type { SkillContinuation } from "@/models/slashCommands";
import { WorkingTurnControls } from "./WorkingTurnControls";
import { memo, useEffect, useId, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject } from "react";
import {
  ArrowUp,
  BookOpen,
  ChevronRight,
  Paperclip,
  Plug,
  Plus,
  RotateCcw,
  Square,
  Undo2,
} from "lucide-react";
import { SidebarPuzzlePiece02Icon } from "@/components/ui/Icons";
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
  placeCaretAtEnd,
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
  type ComposerResourceItem,
  type ComposerResourceKind,
  type ComposerSendStatus,
  type ConversationWorkingAction,
  type ConversationWorkingParticipant,
  type MentionPickerUser,
  type VoidOrPromise,
} from "./types";

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
  onApplyResource?: (resource: ComposerResourceItem) => void;
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
  resourceListLoading?: boolean;
  resourceListHasMore?: boolean;
  resourceManageAgentID?: string;
  resources?: ComposerResourceItem[];
  onLoadMoreResources?: () => void;
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
  slashContinuation,
  slashPickerLoading,
  slashPickerOpen,
  t,
  workingParticipants = [],
  onApplyMention,
  onApplySlashCandidate,
  onApplyResource,
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
  onLoadMoreResources,
  onSyncComposer,
  onUndoRemoveAttachment,
  onWorkingAction,
  resourceListHasMore = false,
  resourceListLoading = false,
  resourceManageAgentID = "",
  resources = [],
}: ConversationComposerProps) {
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const composerHelpId = useId();
  const [resourcePickerKind, setResourcePickerKind] = useState<ComposerResourceKind | null>(null);
  const isSending = sendStatus === "sending";
  const interactionDisabled = composerDisabled || isSending;
  const sendDisabled = interactionDisabled || (!draftText.trim() && attachmentDrafts.length === 0);
  const actionSuggestions = useMemo(() => composerActionSuggestions(draftText), [draftText]);
  const selectedResources = useMemo(
    () => resources.filter((resource) => resource.kind === resourcePickerKind),
    [resourcePickerKind, resources],
  );

  function applyResource(resource: ComposerResourceItem) {
    setResourcePickerKind(null);
    onApplyResource?.(resource);
    if (onApplyResource) {
      return;
    }
    editorRef.current?.focus();
    ensureSelectionInEditor(editorRef.current);
    insertComposerSegmentsAtSelection([
      {
        type: "resource",
        displayText: resourceBadgeText(resource),
        kind: resource.kind,
        name: resource.name,
        text: resourceInsertionText(resource).trim(),
      },
      { type: "text", text: " " },
    ]);
    onSyncComposer();
  }

  useEffect(() => {
    if (!resourcePickerKind) {
      return undefined;
    }
    function handleEscape(event: globalThis.KeyboardEvent) {
      if (event.key !== "Escape") {
        return;
      }
      event.preventDefault();
      setResourcePickerKind(null);
    }
    window.addEventListener("keydown", handleEscape);
    return () => window.removeEventListener("keydown", handleEscape);
  }, [resourcePickerKind]);

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
        {resourcePickerKind ? (
          <ComposerResourcePicker
            agentID={resourceManageAgentID}
            hasMore={resourceListHasMore}
            items={selectedResources}
            kind={resourcePickerKind}
            loading={resourceListLoading}
            t={t}
            onLoadMore={onLoadMoreResources}
            onSelect={applyResource}
          />
        ) : null}
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
            disabled={interactionDisabled}
            t={t}
            onAddFiles={() => fileInputRef.current?.click()}
            onOpenResourceKind={setResourcePickerKind}
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

function ensureSelectionInEditor(editor: HTMLElement | null | undefined): void {
  const selection = window.getSelection();
  if (!editor || (selection?.rangeCount && editor.contains(selection.getRangeAt(0).commonAncestorContainer))) {
    return;
  }
  placeCaretAtEnd(editor);
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
        <WorkingTurnControls participant={participant} t={t} onStop={onStop} />
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

type ComposerAddMenuProps = {
  disabled: boolean;
  t: TranslateFn;
  onAddFiles: () => void;
  onOpenResourceKind?: (kind: ComposerResourceKind) => void;
};

function ComposerAddMenu({
  disabled,
  t,
  onAddFiles,
  onOpenResourceKind,
}: ComposerAddMenuProps) {
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
        <ComposerResourceCategorySection t={t} onOpenResourceKind={onOpenResourceKind} />
      </PopoverContent>
    </PopoverRoot>
  );
}

const resourceKinds: ComposerResourceKind[] = ["skill", "connector", "knowledge"];

function ComposerResourceCategorySection({
  t,
  onOpenResourceKind,
}: {
  t: TranslateFn;
  onOpenResourceKind?: (kind: ComposerResourceKind) => void;
}) {
  return (
    <section className="composer-add-section composer-resource-section" aria-label={t("composerResources")}>
      <div className="composer-add-section-label">{t("composerResources")}</div>
      {resourceKinds.map((kind) => (
        <PopoverClose key={kind} asChild>
          <button type="button" className="composer-add-menu-item" onClick={() => onOpenResourceKind?.(kind)}>
            {resourceKindIcon(kind)}
            <span>{resourceKindLabel(kind, t)}</span>
          </button>
        </PopoverClose>
      ))}
    </section>
  );
}

function ComposerResourcePicker({
  agentID,
  hasMore,
  kind,
  loading,
  items,
  t,
  onLoadMore,
  onSelect,
}: {
  agentID: string;
  hasMore: boolean;
  kind: ComposerResourceKind;
  loading: boolean;
  items: ComposerResourceItem[];
  t: TranslateFn;
  onLoadMore?: () => void;
  onSelect?: (resource: ComposerResourceItem) => void;
}) {
  return (
    <div className="mention-picker composer-resource-picker" role="listbox" aria-label={resourceKindLabel(kind, t)}>
      <div className="composer-resource-picker-title">
        <span>{resourceKindLabel(kind, t)}</span>
        <a className="btn btn-sm btn-link-gray composer-resource-manage-button" href={resourceManageHref(agentID, kind)}>
          {t("manage")}
        </a>
      </div>
      {items.length ? (
        <div className="composer-resource-list">
          {items.map((resource) => (
            <button
              key={`${resource.kind}:${resource.id}`}
              type="button"
              className="composer-resource-item"
              role="option"
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => onSelect?.(resource)}
            >
              <span className="composer-resource-icon" aria-hidden="true">
                {resourceKindIcon(resource.kind)}
              </span>
              <span className="composer-resource-copy">
                <span className="composer-resource-name">{resource.name}</span>
                {resource.description ? (
                  <span className="composer-resource-description">{resource.description}</span>
                ) : null}
              </span>
            </button>
          ))}
        </div>
      ) : (
        <div className="composer-resource-empty">{loading ? t("loading") : t("composerResourceEmpty")}</div>
      )}
      {hasMore ? (
        <div className="composer-resource-picker-footer">
          <button type="button" className="composer-resource-load-more" onClick={onLoadMore}>
            {loading ? t("loading") : t("loadMore")}
          </button>
        </div>
      ) : null}
    </div>
  );
}

function resourceInsertionText(resource: ComposerResourceItem): string {
  const prefix =
    resource.kind === "skill" ? "/skill" : resource.kind === "knowledge" ? "@knowledge" : "@connector";
  return `${prefix}:${resource.name} `;
}

function resourceBadgeText(resource: ComposerResourceItem): string {
  if (resource.kind === "skill") {
    return `/${resource.name}`;
  }
  return resourceInsertionText(resource).trim();
}

function resourceKindLabel(kind: ComposerResourceKind, t: TranslateFn): string {
  if (kind === "skill") return t("skills");
  if (kind === "knowledge") return t("knowledgeBases");
  return t("connectors");
}

function resourceKindIcon(kind: ComposerResourceKind) {
  if (kind === "skill") return <SidebarPuzzlePiece02Icon size={14} />;
  if (kind === "knowledge") return <BookOpen size={14} />;
  return <Plug size={14} />;
}

function resourceManageHref(agentID: string, kind: ComposerResourceKind): string {
  if (!agentID) {
    if (kind === "skill") return "#/resources";
    if (kind === "knowledge") return "#/knowledge-bases";
    return "#/connectors";
  }
  const tab = kind === "skill" ? "skills" : kind === "knowledge" ? "mcp" : "connectors";
  return `#/agents/${encodeURIComponent(agentID)}?tab=${encodeURIComponent(tab)}`;
}
