import "@/components/business/ResourceTag";
import { messageResources, resourceMessageText } from "@/models/conversationResources";
import {
  ConversationResourcePicker,
  useComposerResourcePicker,
  type ComposerResourceProps,
} from "@/components/business/ConversationResourcePicker";
import { ComposerAddMenu } from "./ComposerAddMenu";
import { canSendComposer, selectedResources } from "@/models/conversationResources";
import type { ChatFileScope } from "@/components/business/ChatSidePanel/types";
import { useFileMentions } from "./FileMentionPicker";
import type { SkillContinuation } from "@/models/slashCommands";
import { emptyCompletedProgress } from "@/models/turnProgress";
import { messageListScrollKey, useMessageListAutoScroll } from "@/hooks/workspace/useMessageListAutoScroll";
import { DiagnosticMessageAction } from "@/components/business/TurnDiagnostics";
import { isImageAttachment } from "@/models/attachments";
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowUp, X } from "lucide-react";
import { avatarFallbackText } from "@/shared/avatar";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { ImageGenerationStatus } from "./ImageGenerationStatus";
import { MessageContent, MessagePreviewText } from "@/components/business/MessageContent";
import type { CitationSelectHandler, ProgressControlSlots } from "@/components/business/MessageContent/types";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { Button, Tooltip } from "@/components/ui";
import { resolveAgentAvatarFallback, type AgentLike } from "@/models/agents";
import type { AttachmentDraft } from "@/models/attachments";
import {
  areComposerSegmentsEqual,
  getCollapsedSelectionTextOffset,
  getComposerMentionState,
  getComposerSlashQueryAtSelection,
  getMentionCandidates,
  insertComposerLineBreak,
  insertComposerSegmentsAtSelection,
  insertPlainTextAtSelection,
  isComposerKeyboardEventComposing,
  normalizeComposerSegmentsForDisplay,
  normalizeTextMentions,
  parseComposerSegments,
  placeCaretAtEnd,
  removeAdjacentMentionToken,
  removeComposerResourceToken,
  renderComposerSegments,
  updateComposerResourceLabels,
  replaceMentionQueryWithToken,
  segmentsToPlainText,
  type ComposerSegment,
} from "@/models/composer";
import {
  formatMessageTimestampParts,
  groupVideoGenerationMessages,
  isToolCallMessage,
  resolveAgentForUser,
  resolveUserByLocalIdentity,
  type IMMessage,
  type IMUser,
  type LocaleCode,
  type ThreadView,
  type TranslateFn,
  type UsersById,
} from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import type { ThemeMode } from "@/shared/theme/theme";
import { AttachmentDraftStrip, MessageAttachments } from "./ConversationAttachments";
import { ConversationMessageActions } from "./ConversationMessageActions";
import { dataTransferHasFiles, filesFromDataTransfer } from "./attachmentFiles";
import { MentionPicker } from "./MentionPicker";
import { MessageTimestamp } from "./MessageTime";
import { SlashPicker } from "./SlashPicker";
import { handleSlashPickerNavigation } from "./slashPickerNavigation";
import type { MentionPickerUser, VoidOrPromise } from "./types";
import { AgentQuestionComposer } from "./AgentQuestionComposer";
import { VideoGenerationCard } from "./VideoGenerationCard";
import type { QuestionAnswerMode } from "./useQuestionAnswerMode";

type ThreadMentionState = {
  endOffset: number;
  end: number;
  query: string;
  startOffset: number;
  start: number;
  textNode?: Node;
};

export type ConversationThreadPanelProps = ComposerResourceProps & {
  currentUserID?: string;
  fileScope?: ChatFileScope;
  renderTurnControls?: (message: IMMessage) => ProgressControlSlots | null;
  agents?: AgentLike[];
  disabled: boolean;
  draftSegments: ComposerSegment[];
  attachmentDrafts?: AttachmentDraft[];
  error: string;
  loading: boolean;
  locale: LocaleCode;
  mentionableUsers?: MentionPickerUser[];
  onApplyThreadSlashCandidate?: (name: string) => void;
  onClose: () => void;
  onDismissThreadSlashPicker?: () => void;
  onDraftChange: (segments: ComposerSegment[]) => void;
  onSlashQueryChange?: (query: string | null) => void;
  onAddAttachments?: (files: File[]) => void;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  onCitationSelect?: CitationSelectHandler;
  questionMode?: QuestionAnswerMode;
  onRemoveAttachment?: (id: string) => void;
  onSend: () => VoidOrPromise;
  onSetThreadSlashIndex?: (index: number) => void;
  showToolCalls: boolean;
  t: TranslateFn;
  theme: ThemeMode;
  thread?: ThreadView | null;
  threadSlashCandidates?: SlashPickerCandidate[];
  threadSlashIndex?: number;
  threadSlashContinuation?: SkillContinuation;
  threadSlashPickerLoading?: boolean;
  threadSlashPickerOpen?: boolean;
  usersById: UsersById;
};

export function ConversationThreadPanel({
  resourceAgentID,
  onPickResources,
  fileScope,
  renderTurnControls,
  thread,
  currentUserID,
  agents = [],
  loading,
  error,
  draftSegments,
  attachmentDrafts = [],
  disabled,
  usersById,
  locale,
  theme,
  showToolCalls,
  t,
  onClose,
  onDraftChange,
  onSlashQueryChange = (_query) => {},
  onAddAttachments = () => {},
  threadSlashCandidates = [],
  threadSlashIndex = 0,
  threadSlashContinuation,
  threadSlashPickerLoading = false,
  threadSlashPickerOpen = false,
  onApplyThreadSlashCandidate = (_name) => {},
  onDismissThreadSlashPicker = () => {},
  onSetThreadSlashIndex = (_index) => {},
  onOpenAgentDetail,
  onPreviewUser,
  onPreviewAttachment,
  onQuestionSelect,
  onCitationSelect,
  questionMode,
  onRemoveAttachment = () => {},
  mentionableUsers = [],
  onSend,
}: ConversationThreadPanelProps) {
  const threadBodyRef = useRef<HTMLDivElement | null>(null);
  const threadEditorRef = useRef<HTMLDivElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const resourcePicker = useComposerResourcePicker({
    resourceAgentID,
    onPickResources,
    editorRef: threadEditorRef,
    selected: selectedResources(draftSegments),
    disabled,
  });
  const [mentionState, setMentionState] = useState<ThreadMentionState | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const root = thread?.root ?? null;
  const replies = useMemo(() => thread?.replies ?? [], [thread?.replies]);
  const videoGroups = useMemo(() => groupVideoGenerationMessages(root ? [root, ...replies] : replies), [root, replies]);
  const visibleRoot =
    !emptyCompletedProgress(root) &&
    (showToolCalls || !isToolCallMessage(root)) &&
    (!root?.id || !videoGroups.childMessageIDs.has(root.id))
      ? root
      : null;
  const visibleReplies = replies.filter(
    (message) => !emptyCompletedProgress(message) && (showToolCalls || !isToolCallMessage(message)),
  );
  const displayReplies = visibleReplies.filter(
    (message) => !message.id || !videoGroups.childMessageIDs.has(message.id),
  );
  const mentionableUsersByName = useMemo(() => {
    const result = new Map<string, (typeof mentionableUsers)[number]>();
    const duplicateNames = new Set<string>();
    mentionableUsers.forEach((user) => {
      const name = String(user.name || "")
        .trim()
        .toLowerCase();
      if (!name || duplicateNames.has(name)) {
        return;
      }
      if (result.has(name)) {
        result.delete(name);
        duplicateNames.add(name);
        return;
      }
      result.set(name, user);
    });
    return result;
  }, [mentionableUsers]);
  const displayDraftSegments = useMemo(() => normalizeComposerSegmentsForDisplay(draftSegments || []), [draftSegments]);
  const threadMentionCandidates = useMemo(() => {
    if (!mentionState) {
      return [];
    }
    return getMentionCandidates(mentionableUsers, mentionState.query) as MentionPickerUser[];
  }, [mentionState, mentionableUsers]);

  const fileMentions = useFileMentions({
    scope: fileScope,
    editorRef: threadEditorRef,
    onSync: () => syncThreadDraft(),
    people: threadMentionCandidates,
    personIndex: mentionIndex,
    onPerson: insertThreadMention,
    t,
  });
  const threadScrollKey = useMemo(
    () => messageListScrollKey([...(visibleRoot ? [visibleRoot] : []), ...displayReplies]),
    [visibleRoot, displayReplies],
  );
  useMessageListAutoScroll({
    active: Boolean(root) && !loading,
    conversationId: root?.id || "",
    messageListRef: threadBodyRef,
    visibleMessagesKey: threadScrollKey,
  });

  useLayoutEffect(() => {
    const editor = threadEditorRef.current;
    if (!editor) {
      return;
    }
    updateComposerResourceLabels(editor, t("remove"));
    const currentSegments = parseComposerSegments(editor);
    if (!areComposerSegmentsEqual(currentSegments, displayDraftSegments)) {
      const currentText = segmentsToPlainText(currentSegments);
      const nextText = segmentsToPlainText(displayDraftSegments);
      const selectionOffset = getCollapsedSelectionTextOffset(editor);
      renderComposerSegments(editor, displayDraftSegments, t("remove"));
      if (currentText === nextText && selectionOffset === currentText.length) {
        placeCaretAtEnd(editor);
      }
    }
  }, [displayDraftSegments, t]);

  function syncThreadDraft(target = threadEditorRef.current) {
    if (!target) {
      return;
    }
    const segments = normalizeComposerSegmentsForDisplay(parseComposerSegments(target) as ComposerSegment[]);
    onDraftChange(segments);
    syncThreadMentionState(target);
    syncThreadSlashQuery(target);
    fileMentions.sync();
  }

  function syncThreadSlashQuery(target = threadEditorRef.current) {
    onSlashQueryChange(getComposerSlashQueryAtSelection(target));
  }

  function syncThreadQueryState(target = threadEditorRef.current) {
    syncThreadMentionState(target);
    syncThreadSlashQuery(target);
    fileMentions.sync();
  }

  function syncThreadMentionState(target = threadEditorRef.current) {
    if (!target) {
      setMentionState(null);
      return;
    }
    const nextMentionState = getComposerMentionState(target);
    if (!nextMentionState) {
      setMentionState(null);
      setMentionIndex(0);
      return;
    }
    const normalized: ThreadMentionState = {
      end: nextMentionState.endOffset,
      endOffset: nextMentionState.endOffset,
      query: nextMentionState.query,
      start: nextMentionState.startOffset,
      startOffset: nextMentionState.startOffset,
      textNode: nextMentionState.textNode,
    };
    const mentionChanged =
      !mentionState ||
      mentionState.start !== normalized.start ||
      mentionState.end !== normalized.end ||
      mentionState.query !== normalized.query;
    setMentionState(normalized);
    if (mentionChanged) {
      setMentionIndex(0);
    }
  }

  function insertThreadMention(user: MentionPickerUser | null | undefined) {
    const target = threadEditorRef.current;
    if (!target || !mentionState || !user) {
      return;
    }
    if (!replaceMentionQueryWithToken(target, mentionState, user)) {
      return;
    }
    syncThreadDraft(target);
    setMentionState(null);
    setMentionIndex(0);
    requestAnimationFrame(() => {
      if (threadEditorRef.current !== target) {
        return;
      }
      target.focus();
    });
  }

  function handleFiles(files: File[]) {
    if (disabled || files.length === 0) {
      return;
    }
    onAddAttachments(files);
  }

  return (
    <aside className="thread-panel" aria-label={t("threadPanelTitle")}>
      <div className="thread-panel-header">
        <div>
          <div className="thread-panel-kicker">{t("threadPanelTitle")}</div>
          <div className="thread-panel-title truncate">
            {visibleRoot ? (
              <MessagePreviewText
                content={
                  thread?.summary?.context_summary?.root_excerpt ||
                  resourceMessageText(visibleRoot.content, visibleRoot.metadata) ||
                  visibleRoot.attachments?.[0]?.name ||
                  t("attachment")
                }
              />
            ) : (
              t("noVisibleMessages")
            )}
          </div>
        </div>
        <Tooltip content={t("close")}>
          <Button className="icon-button" aria-label={t("close")} onClick={onClose}>
            <span className="icon-button-mark" aria-hidden="true">
              <X size={18} strokeWidth={2} />
            </span>
          </Button>
        </Tooltip>
      </div>
      <div ref={threadBodyRef} className="thread-panel-body">
        {loading && !root ? <div className="thread-empty">{t("loading")}</div> : null}
        {error ? <div className="form-error">{error}</div> : null}
        {visibleRoot ? (
          <div className="thread-root">
            <ThreadMessage
              message={visibleRoot}
              currentUserID={currentUserID}
              progressControls={renderTurnControls?.(visibleRoot)}
              agents={agents}
              usersById={usersById}
              locale={locale}
              theme={theme}
              t={t}
              onOpenAgentDetail={onOpenAgentDetail}
              onPreviewUser={onPreviewUser}
              onPreviewAttachment={onPreviewAttachment}
              onQuestionSelect={onQuestionSelect}
              onCitationSelect={onCitationSelect}
              embeddedVideos={visibleRoot.id ? videoGroups.childrenByParentID.get(visibleRoot.id) : undefined}
            />
          </div>
        ) : null}
        <div className="thread-replies">
          {displayReplies.length > 0 ? (
            displayReplies.map((message) => (
              <ThreadMessage
                key={message.id}
                message={message}
                currentUserID={currentUserID}
                progressControls={renderTurnControls?.(message)}
                agents={agents}
                usersById={usersById}
                locale={locale}
                theme={theme}
                t={t}
                onOpenAgentDetail={onOpenAgentDetail}
                onPreviewUser={onPreviewUser}
                onPreviewAttachment={onPreviewAttachment}
                onQuestionSelect={onQuestionSelect}
                onCitationSelect={onCitationSelect}
                embeddedVideos={message.id ? videoGroups.childrenByParentID.get(message.id) : undefined}
              />
            ))
          ) : (
            <div className="thread-empty">{t("threadNoReplies")}</div>
          )}
        </div>
      </div>
      {questionMode?.pending.length ? (
        <AgentQuestionComposer mode={questionMode} t={t} usersById={usersById} />
      ) : (
        <div
          className="thread-composer"
          onDragOver={(event) => {
            if (disabled || !dataTransferHasFiles(event.dataTransfer)) {
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
          {!resourcePicker.active && threadSlashPickerOpen ? (
            <SlashPicker
              candidates={threadSlashCandidates}
              activeIndex={threadSlashIndex}
              continuation={threadSlashContinuation}
              loading={threadSlashPickerLoading}
              className="thread-slash-picker"
              t={t}
              onSelect={(name) => onApplyThreadSlashCandidate(name)}
            />
          ) : null}
          {resourcePicker.active ? null : fileScope ? (
            fileMentions.picker
          ) : threadMentionCandidates.length > 0 ? (
            <MentionPicker
              users={threadMentionCandidates}
              activeIndex={mentionIndex}
              className="thread-mention-picker"
              showRole={false}
              t={t}
              onSelect={insertThreadMention}
            />
          ) : null}
          <div className="thread-composer-box">
            <AttachmentDraftStrip
              drafts={attachmentDrafts}
              t={t}
              onPreviewAttachment={onPreviewAttachment}
              onRemove={onRemoveAttachment}
            />
            <div
              ref={threadEditorRef}
              hidden={resourcePicker.active}
              contentEditable={!disabled}
              suppressContentEditableWarning={true}
              role="textbox"
              aria-placeholder={disabled ? t("profileIncomplete") : t("threadComposerPlaceholder")}
              aria-label={t("threadComposerPlaceholder")}
              className={`thread-composer-editor ${disabled ? "disabled" : ""}`}
              data-placeholder={disabled ? t("profileIncomplete") : t("threadComposerPlaceholder")}
              onInput={(event) => syncThreadDraft(event.currentTarget)}
              onClick={(event) => {
                if (!disabled && removeComposerResourceToken(event.currentTarget, event.target)) {
                  syncThreadDraft(event.currentTarget);
                } else {
                  syncThreadQueryState(event.currentTarget);
                }
              }}
              onKeyDown={(event) => {
                if (
                  disabled ||
                  isComposerKeyboardEventComposing(event) ||
                  (event.target instanceof Element && event.target.closest("[data-composer-resource-remove]")) ||
                  fileMentions.handleKeyDown(event)
                ) {
                  return;
                }
                if (event.key === "Backspace" && removeAdjacentMentionToken(threadEditorRef.current, "backward")) {
                  event.preventDefault();
                  syncThreadDraft(event.currentTarget);
                  return;
                }
                if (event.key === "Delete" && removeAdjacentMentionToken(threadEditorRef.current, "forward")) {
                  event.preventDefault();
                  syncThreadDraft(event.currentTarget);
                  return;
                }
                if (
                  handleSlashPickerNavigation({
                    event,
                    candidates: threadSlashCandidates,
                    activeIndex: threadSlashIndex,
                    pickerOpen: threadSlashPickerOpen,
                    continuation: threadSlashContinuation,
                    onIndexChange: (value) => onSetThreadSlashIndex(value),
                    onApply: (value) => onApplyThreadSlashCandidate(value),
                    onDismiss: () => {
                      onDismissThreadSlashPicker();
                      setMentionState(null);
                      setMentionIndex(0);
                    },
                    onPrepareNavigation: () => {
                      setMentionState(null);
                      setMentionIndex(0);
                    },
                  })
                ) {
                  return;
                }
                if (threadMentionCandidates.length > 0) {
                  if (event.key === "ArrowDown") {
                    event.preventDefault();
                    setMentionIndex((value) => (value + 1) % threadMentionCandidates.length);
                    return;
                  }
                  if (event.key === "ArrowUp") {
                    event.preventDefault();
                    setMentionIndex(
                      (value) => (value - 1 + threadMentionCandidates.length) % threadMentionCandidates.length,
                    );
                    return;
                  }
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    insertThreadMention(threadMentionCandidates[mentionIndex]);
                    return;
                  }
                  if (event.key === "Escape") {
                    event.preventDefault();
                    setMentionState(null);
                    setMentionIndex(0);
                    return;
                  }
                }
                if (event.key === "Enter" && event.shiftKey) {
                  event.preventDefault();
                  insertComposerLineBreak(threadEditorRef.current);
                  syncThreadDraft(threadEditorRef.current);
                  return;
                }
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  onSend();
                }
              }}
              onKeyUp={(event) => syncThreadQueryState(event.currentTarget)}
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
                syncThreadDraft(threadEditorRef.current);
              }}
              onCompositionEnd={() => {
                syncThreadDraft(threadEditorRef.current);
              }}
            />
            {resourcePicker.pickerProps ? (
              <ConversationResourcePicker
                key={`${resourceAgentID}:${resourcePicker.pickerProps.kind}`}
                {...resourcePicker.pickerProps}
                t={t}
              />
            ) : null}
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
            <div className="thread-composer-toolbar">
              <ComposerAddMenu
                onOpenResourcePicker={resourcePicker.open}
                disabled={disabled}
                t={t}
                onAddFiles={() => fileInputRef.current?.click()}
              />
              <Button
                variant="primary"
                className="composer-send-button"
                iconOnly
                size="lg"
                aria-label={t("send")}
                disabled={disabled || resourcePicker.active || !canSendComposer(draftSegments, attachmentDrafts.length)}
                onClick={onSend}
              >
                <ArrowUp aria-hidden="true" size={22} strokeWidth={1.5} />
              </Button>
            </div>
          </div>
        </div>
      )}
    </aside>
  );
}

type ThreadMessageProps = {
  currentUserID?: string;
  progressControls?: ProgressControlSlots | null;
  agents?: AgentLike[];
  compact?: boolean;
  embeddedVideos?: IMMessage[];
  locale: LocaleCode;
  message: IMMessage;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  onCitationSelect?: CitationSelectHandler;
  t: TranslateFn;
  theme: ThemeMode;
  usersById: UsersById;
};

function ThreadMessage({
  currentUserID,
  progressControls,
  message,
  agents = [],
  usersById,
  locale,
  theme,
  t,
  onOpenAgentDetail,
  onPreviewUser,
  onPreviewAttachment,
  onQuestionSelect,
  onCitationSelect,
  compact = false,
  embeddedVideos = [],
}: ThreadMessageProps) {
  const user = resolveUserByLocalIdentity(message.sender_id, usersById);
  const messageAgent = user ? resolveThreadMessageAgent(agents, user, message.sender_id) : null;
  const fallbackName = message.sender_id || "";
  const fallbackAvatar = fallbackName.slice(0, 1).toUpperCase();
  const avatar = user?.avatar || messageAgent?.avatar || fallbackAvatar;
  const own = Boolean(currentUserID && (user?.id === currentUserID || message.sender_id === currentUserID));
  const fallback = messageAgent
    ? resolveAgentAvatarFallback(messageAgent, usersById)
    : avatarFallbackText(avatar, user?.name, user?.id);
  const name = user?.name || fallbackName;
  const timestampParts = formatMessageTimestampParts(message.created_at, locale, t);
  const isVideoGeneration = Boolean(message.metadata?.video_generation);

  return (
    <div
      className={`thread-message ${own ? "own" : ""} ${compact ? "compact" : ""} ${message.metadata?.image_generation || isVideoGeneration ? "image-generation-message" : ""}`.trim()}
      data-message-id={message.id || undefined}
    >
      {user ? (
        <button
          type="button"
          className="thread-message-avatar"
          aria-label={`${t(messageAgent ? "agentDetailPanel" : "profilePreview")} ${name}`}
          onClick={(event) => {
            if (messageAgent && onOpenAgentDetail) {
              onOpenAgentDetail(messageAgent, event.currentTarget);
              return;
            }
            onPreviewUser(user, event.currentTarget);
          }}
        >
          <AgentAvatarContent avatar={avatar} fallback={fallback} />
        </button>
      ) : (
        <div className="thread-message-avatar" aria-hidden="true">
          <AgentAvatarContent avatar={avatar} fallback={fallback} />
        </div>
      )}
      <div className="thread-message-main">
        <div className="message-meta">
          <span className="message-author">{name}</span>
          <MessageTimestamp parts={timestampParts} />
        </div>
        {(message.content || messageResources(message.metadata).length > 0) && !message.metadata?.image_generation ? (
          <div className="thread-message-bubble">
            <MessageContent
              key={`${message.id}:${theme}`}
              content={message.content}
              message={message}
              progressControls={progressControls}
              videoMessages={embeddedVideos}
              onQuestionSelect={onQuestionSelect}
              onCitationSelect={onCitationSelect}
              t={t}
            />
          </div>
        ) : null}
        <DiagnosticMessageAction metadata={message.metadata} t={t} />
        <ImageGenerationStatus message={message} t={t} />
        {isVideoGeneration ? (
          <VideoGenerationCard message={message} t={t} onPreviewAttachment={onPreviewAttachment} />
        ) : (
          <MessageAttachments attachments={message.attachments} t={t} onPreviewAttachment={onPreviewAttachment} />
        )}
        {embeddedVideos.map((video) => (
          <VideoGenerationCard
            key={video.id || String(video.created_at || video.content)}
            message={video}
            t={t}
            onPreviewAttachment={onPreviewAttachment}
          />
        ))}
        <ConversationMessageActions
          metadata={message.metadata}
          className="thread-message-actions"
          content={message.metadata?.image_generation ? null : message.content}
          image={message.metadata?.image_generation ? message.attachments?.find(isImageAttachment) : undefined}
          t={t}
        />
      </div>
    </div>
  );
}

function resolveThreadMessageAgent(
  agents: readonly AgentLike[],
  user: IMUser,
  senderID: string | null | undefined,
): AgentLike | null {
  const senderIdentity = String(senderID || "").trim();
  const alternateUser =
    senderIdentity && senderIdentity !== user.id ? { ...user, id: senderIdentity, user_id: user.id } : null;
  return resolveAgentForUser(agents, user, alternateUser ? [alternateUser] : []);
}
