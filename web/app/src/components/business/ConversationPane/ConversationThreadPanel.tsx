import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { X } from "lucide-react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { MessageContent, MessagePreviewText } from "@/components/business/MessageContent";
import { Button } from "@/components/ui";
import { IconImage } from "@/components/ui/Icons";
import {
  areComposerSegmentsEqual,
  getCollapsedSelectionTextOffset,
  getComposerMentionState,
  getMentionCandidates,
  insertComposerLineBreak,
  insertComposerSegmentsAtSelection,
  insertPlainTextAtSelection,
  normalizeComposerSegmentsForDisplay,
  normalizeTextMentions,
  parseComposerSegments,
  placeCaretAtEnd,
  removeAdjacentMentionToken,
  renderComposerSegments,
  replaceMentionQueryWithToken,
  segmentsToPlainText,
  type ComposerSegment,
} from "@/models/composer";
import {
  formatMessageTimestampParts,
  formatThreadReplyCount,
  isToolCallMessage,
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
import { MentionPicker } from "./MentionPicker";
import { MessageTimestamp } from "./MessageTime";
import { SlashPicker } from "./SlashPicker";
import { handleSlashPickerNavigation } from "./slashPickerNavigation";
import type { MentionPickerUser, VoidOrPromise } from "./types";

type ThreadMentionState = {
  endOffset: number;
  end: number;
  query: string;
  startOffset: number;
  start: number;
  textNode?: Node;
};

export type ConversationThreadPanelProps = {
  disabled: boolean;
  draftSegments: ComposerSegment[];
  error: string;
  loading: boolean;
  locale: LocaleCode;
  mentionableUsers?: MentionPickerUser[];
  onApplyThreadSlashCandidate?: (name: string) => void;
  onClose: () => void;
  onDismissThreadSlashPicker?: () => void;
  onDraftChange: (segments: ComposerSegment[]) => void;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onSend: () => VoidOrPromise;
  onSetThreadSlashIndex?: (index: number) => void;
  showToolCalls: boolean;
  t: TranslateFn;
  theme: ThemeMode;
  thread?: ThreadView | null;
  threadSlashCandidates?: SlashPickerCandidate[];
  threadSlashIndex?: number;
  threadSlashPickerLoading?: boolean;
  threadSlashPickerOpen?: boolean;
  usersById: UsersById;
};

export function ConversationThreadPanel({
  thread,
  loading,
  error,
  draftSegments,
  disabled,
  usersById,
  locale,
  theme,
  showToolCalls,
  t,
  onClose,
  onDraftChange,
  threadSlashCandidates = [],
  threadSlashIndex = 0,
  threadSlashPickerLoading = false,
  threadSlashPickerOpen = false,
  onApplyThreadSlashCandidate = (_name) => {},
  onDismissThreadSlashPicker = () => {},
  onSetThreadSlashIndex = (_index) => {},
  onPreviewUser,
  mentionableUsers = [],
  onSend,
}: ConversationThreadPanelProps) {
  const threadBodyRef = useRef<HTMLDivElement | null>(null);
  const threadEditorRef = useRef<HTMLDivElement | null>(null);
  const [mentionState, setMentionState] = useState<ThreadMentionState | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const root = thread?.root ?? null;
  const replies = thread?.replies ?? [];
  const visibleRoot = showToolCalls || !isToolCallMessage(root) ? root : null;
  const visibleReplies = showToolCalls ? replies : replies.filter((message) => !isToolCallMessage(message));
  const latestReplyID = visibleReplies[visibleReplies.length - 1]?.id || "";
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

  useLayoutEffect(() => {
    const threadBody = threadBodyRef.current;
    if (!threadBody || !root) {
      return;
    }
    const scrollToBottom = () => {
      threadBody.scrollTop = threadBody.scrollHeight;
    };
    scrollToBottom();
    const frame = window.requestAnimationFrame(scrollToBottom);
    return () => window.cancelAnimationFrame(frame);
  }, [root, visibleReplies.length, latestReplyID, loading]);

  useLayoutEffect(() => {
    const editor = threadEditorRef.current;
    if (!editor) {
      return;
    }
    const currentSegments = parseComposerSegments(editor);
    if (!areComposerSegmentsEqual(currentSegments, displayDraftSegments)) {
      const currentText = segmentsToPlainText(currentSegments);
      const nextText = segmentsToPlainText(displayDraftSegments);
      const selectionOffset = getCollapsedSelectionTextOffset(editor);
      renderComposerSegments(editor, displayDraftSegments);
      if (currentText === nextText && selectionOffset === currentText.length) {
        placeCaretAtEnd(editor);
      }
    }
  }, [displayDraftSegments]);

  function syncThreadDraft(target = threadEditorRef.current) {
    if (!target) {
      return;
    }
    const segments = normalizeComposerSegmentsForDisplay(parseComposerSegments(target) as ComposerSegment[]);
    onDraftChange(segments);
    syncThreadMentionState(target);
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

  return (
    <aside className="thread-panel" aria-label={t("threadPanelTitle")}>
      <div className="thread-panel-header">
        <div>
          <div className="thread-panel-kicker">{t("threadPanelTitle")}</div>
          <div className="thread-panel-title truncate">
            {visibleRoot ? (
              <MessagePreviewText
                content={thread?.summary?.context_summary?.root_excerpt || visibleRoot.content || ""}
              />
            ) : (
              t("noVisibleMessages")
            )}
          </div>
        </div>
        <Button className="icon-button" aria-label={t("close")} title={t("close")} onClick={onClose}>
          <span className="icon-button-mark" aria-hidden="true">
            <X size={18} strokeWidth={2} />
          </span>
        </Button>
      </div>
      <div ref={threadBodyRef} className="thread-panel-body">
        {loading && !root ? <div className="thread-empty">{t("loading")}</div> : null}
        {error ? <div className="form-error">{error}</div> : null}
        {visibleRoot ? (
          <div className="thread-root">
            <ThreadMessage
              message={visibleRoot}
              usersById={usersById}
              locale={locale}
              theme={theme}
              t={t}
              onPreviewUser={onPreviewUser}
            />
          </div>
        ) : null}
        <div className="thread-replies">
          <div className="thread-section-title">{formatThreadReplyCount(visibleReplies.length, t)}</div>
          {visibleReplies.length > 0 ? (
            visibleReplies.map((message) => (
              <ThreadMessage
                key={message.id}
                message={message}
                usersById={usersById}
                locale={locale}
                theme={theme}
                t={t}
                onPreviewUser={onPreviewUser}
              />
            ))
          ) : (
            <div className="thread-empty">{t("threadNoReplies")}</div>
          )}
        </div>
      </div>
      <div className="thread-composer">
        {threadSlashPickerOpen ? (
          <SlashPicker
            candidates={threadSlashCandidates}
            activeIndex={threadSlashIndex}
            loading={threadSlashPickerLoading}
            className="thread-slash-picker"
            t={t}
            onSelect={(name) => onApplyThreadSlashCandidate(name)}
          />
        ) : null}
        {threadMentionCandidates.length > 0 ? (
          <MentionPicker
            users={threadMentionCandidates}
            activeIndex={mentionIndex}
            className="thread-mention-picker"
            showRole={false}
            t={t}
            onSelect={insertThreadMention}
          />
        ) : null}
        <div
          ref={threadEditorRef}
          contentEditable={!disabled}
          suppressContentEditableWarning={true}
          role="textbox"
          aria-placeholder={disabled ? t("profileIncomplete") : t("threadComposerPlaceholder")}
          aria-label={t("threadComposerPlaceholder")}
          className={`thread-composer-editor ${disabled ? "disabled" : ""}`}
          data-placeholder={disabled ? t("profileIncomplete") : t("threadComposerPlaceholder")}
          onInput={(event) => syncThreadDraft(event.currentTarget)}
          onClick={(event) => syncThreadMentionState(event.currentTarget)}
          onKeyDown={(event) => {
            if (disabled) {
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
          onKeyUp={(event) => syncThreadMentionState(event.currentTarget)}
          onPaste={(event) => {
            event.preventDefault();
            const pasted = event.clipboardData?.getData("text/plain") ?? "";
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
        <Button
          variant="primary"
          className="thread-send-button"
          disabled={disabled || !segmentsToPlainText(draftSegments || []).trim()}
          onClick={onSend}
        >
          <span aria-hidden="true">{IconImage("send")}</span>
          <span>{t("send")}</span>
        </Button>
      </div>
    </aside>
  );
}

type ThreadMessageProps = {
  compact?: boolean;
  locale: LocaleCode;
  message: IMMessage;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  t: TranslateFn;
  theme: ThemeMode;
  usersById: UsersById;
};

function ThreadMessage({ message, usersById, locale, theme, t, onPreviewUser, compact = false }: ThreadMessageProps) {
  const user = resolveUserByLocalIdentity(message.sender_id, usersById);
  const fallbackName = message.sender_id || "";
  const avatar = user?.avatar || fallbackName.slice(0, 1).toUpperCase();
  const name = user?.name || fallbackName;
  const timestampParts = formatMessageTimestampParts(message.created_at, locale, t);

  return (
    <div className={`thread-message ${compact ? "compact" : ""}`.trim()}>
      {user ? (
        <button
          type="button"
          className="thread-message-avatar"
          aria-label={`${t("profilePreview")} ${name}`}
          onClick={(event) => onPreviewUser(user, event.currentTarget)}
        >
          <AgentAvatarContent avatar={avatar} fallback={avatar} />
        </button>
      ) : (
        <div className="thread-message-avatar" aria-hidden="true">
          <AgentAvatarContent avatar={avatar} fallback={avatar} />
        </div>
      )}
      <div className="thread-message-main">
        <div className="message-meta">
          <span className="message-author">{name}</span>
          <MessageTimestamp parts={timestampParts} />
        </div>
        <div className="thread-message-bubble">
          <MessageContent key={`${message.id}:${theme}`} content={message.content} message={message} />
        </div>
      </div>
    </div>
  );
}
