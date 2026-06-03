import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Logs, RefreshCw, X } from "lucide-react";
import { fetchAgentLogsRequest } from "@/api/agents";
import { errorMessage } from "@/api/client";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import { MessageContent, MessagePreviewText } from "@/components/business/MessageContent";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import { AddUserIcon, IconImage, TrashIcon, UsersIcon, WrenchIcon } from "@/components/ui/Icons";
import {
  type ComposerSegment,
  insertComposerSegmentsAtSelection,
  areComposerSegmentsEqual,
  removeAdjacentMentionToken,
  getComposerMentionState,
  insertComposerLineBreak,
  parseComposerSegments,
  renderComposerSegments,
  insertPlainTextAtSelection,
  replaceMentionQueryWithToken,
  getMentionCandidates,
  normalizeComposerSegmentsForDisplay,
  segmentsToPlainText,
  normalizeTextMentions,
} from "@/models/composer";
import { isAgentRunning, normalizeAuthProviderName, providerNeedsAuth } from "@/models/agents";
import {
  agentMatchesUser,
  formatEventMessage,
  formatMessageTimestampParts,
  formatThreadReplyCount,
  getConversationDescription,
  isDirectConversation,
  isEventMessage,
  isToolCallMessage,
} from "@/models/conversations";
import { localizeRole } from "@/shared/i18n";

type ThreadMentionState = {
  endOffset: number;
  end: number;
  query: string;
  startOffset: number;
  start: number;
  textNode?: Node;
};

export function ConversationPane({
  conversation,
  visibleMessages,
  currentUserID,
  usersById,
  agents = [],
  locale,
  t,
  theme,
  selectedMessageCount,
  logAgent,
  conversationMembers,
  showMemberList,
  onToggleMemberList,
  showChannelTools,
  onToggleChannelTools,
  showToolCalls,
  onToggleToolCalls,
  memberMenuRef,
  channelToolsRef,
  messageListRef,
  editorRef,
  onPreviewUser,
  onDeleteRoom,
  inviteActionLabel,
  onInviteAction,
  mentionCandidates,
  mentionIndex,
  onApplyMention,
  skillCandidates = [],
  skillIndex = 0,
  skillLoading = false,
  skillPickerOpen = false,
  onApplySkillCandidate = (_name) => {},
  threadSkillCandidates = [],
  threadSkillIndex = 0,
  threadSkillLoading = false,
  threadSkillPickerOpen = false,
  onApplyThreadSkillCandidate = (_name) => {},
  onDismissThreadSkillPicker = () => {},
  onSetThreadSkillIndex = (_index) => {},
  managerProfile,
  managerProfileIncomplete,
  authStatuses,
  authBusyProvider,
  onProviderLogin,
  draftSegments,
  draftText,
  mentionableUsersByHandle,
  onSyncComposer,
  onComposerKeyDown,
  onComposerCompositionStart,
  onComposerCompositionEnd,
  onSendMessage,
  composerError,
  messageActionBusy,
  messageActionError,
  onMessageAction,
  activeThreadRootID,
  activeThreadView,
  threadLoading,
  threadError,
  threadDraftSegments,
  onOpenThread,
  onCloseThread,
  onThreadDraftChange,
  onSendThreadReply,
}) {
  const description = getConversationDescription(conversation, currentUserID, usersById, locale, t);
  const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [logContent, setLogContent] = useState("");
  const [logError, setLogError] = useState("");
  const [logLoading, setLogLoading] = useState(false);
  const logAgentID = logAgent?.id || "";
  const logAgentName = logAgent?.name || conversation.title;

  useEffect(() => {
    setLogModalOpen(false);
    setLogContent("");
    setLogError("");
    setLogLoading(false);
  }, [conversation.id, logAgentID]);

  useLayoutEffect(() => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    const currentSegments = parseComposerSegments(editor);
    if (!areComposerSegmentsEqual(currentSegments, draftSegments)) {
      renderComposerSegments(editor, draftSegments);
    }
  }, [draftSegments]);

  async function refreshAgentLogs() {
    if (!logAgentID) {
      return;
    }
    setLogLoading(true);
    setLogError("");
    try {
      setLogContent(await fetchAgentLogsRequest(logAgentID, { lines: 400 }));
    } catch (err) {
      setLogError(errorMessage(err, t("agentLogsLoadFailed")));
    } finally {
      setLogLoading(false);
    }
  }

  function openAgentLogs() {
    setLogModalOpen(true);
    void refreshAgentLogs();
  }

  return (
    <>
      <header className="chat-header">
        <div className="chat-header-main">
          <div className="chat-title-bar">
            <div className="chat-title-row">
              <div className="chat-title-group">
                <div className="chat-kicker">
                  <span>
                    {isDirectConversation(conversation) ? t("directMessagesSection") : t("conversationLabel")}
                  </span>
                  <strong>{selectedMessageCount}</strong>
                </div>
                <div className="chat-title truncate">{conversation.title}</div>
                <div ref={memberMenuRef} className="header-menu">
                  <Button
                    className="member-badge-button"
                    active={showMemberList}
                    aria-label={t("membersTitle")}
                    aria-pressed={showMemberList}
                    title={t("membersTitle")}
                    onClick={() => {
                      onToggleMemberList((value) => !value);
                      onToggleChannelTools(false);
                    }}
                  >
                    <span className="icon-button-mark" aria-hidden="true">
                      <UsersIcon />
                    </span>
                    <span className="member-badge-count">{conversationMembers.length}</span>
                  </Button>
                  {showMemberList ? (
                    <div className="header-popover members-popover">
                      <div className="header-popover-title">{t("membersTitle")}</div>
                      <div className="members-popover-list">
                        {conversationMembers.map((user) => (
                          <div key={user.id} className="member-row">
                            <button
                              type="button"
                              className="avatar avatar-button"
                              aria-label={`${t("profilePreview")} ${user.name}`}
                              onClick={(event) => onPreviewUser(user, event.currentTarget)}
                            >
                              <AgentAvatarContent
                                avatar={user.avatar}
                                fallback={avatarFallbackText(user.avatar, user.name, user.handle, user.id)}
                              />
                            </button>
                            <div className="member-row-main">
                              <div className="member-row-name">{user.name}</div>
                              <div className="member-row-meta">
                                @{user.handle} · {localizeRole(user.role, t)}
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
            <div className="chat-title-actions">
              {logAgent ? (
                <Button
                  className="icon-button"
                  active={logModalOpen}
                  aria-label={t("agentLogs")}
                  title={t("agentLogs")}
                  onClick={openAgentLogs}
                >
                  <span className="icon-button-mark" aria-hidden="true">
                    <Logs size={18} strokeWidth={2} />
                  </span>
                </Button>
              ) : null}
              <div ref={channelToolsRef} className="header-menu tools-menu">
                <Button
                  className="icon-button"
                  active={showChannelTools}
                  aria-label={t("channelTools")}
                  aria-expanded={showChannelTools}
                  title={t("channelTools")}
                  onClick={() => {
                    onToggleChannelTools((value) => !value);
                    onToggleMemberList(false);
                  }}
                >
                  <span className="icon-button-mark">
                    <WrenchIcon />
                  </span>
                </Button>
                {showChannelTools ? (
                  <div className="header-popover tools-popover">
                    <div className="header-popover-title">{t("channelTools")}</div>
                    <Button className="tool-menu-row" onClick={() => onToggleToolCalls((value) => !value)}>
                      <span>{showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}</span>
                      <strong>{showToolCalls ? t("enabled") : t("disabled")}</strong>
                    </Button>
                    {!isDirectConversation(conversation) ? (
                      <Button
                        variant="outlineDanger"
                        className="tool-menu-row danger"
                        onClick={() => {
                          onToggleChannelTools(false);
                          onDeleteRoom(conversation.id);
                        }}
                      >
                        <span>{t("deleteRoom")}</span>
                        <span className="tool-menu-icon" aria-hidden="true">
                          <TrashIcon />
                        </span>
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <Button
                className="icon-button"
                aria-label={inviteActionLabel}
                title={inviteActionLabel}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  onInviteAction();
                }}
              >
                <span className="icon-button-mark">
                  <AddUserIcon />
                </span>
              </Button>
            </div>
          </div>
          {description ? <div className="chat-subtitle">{description}</div> : null}
        </div>
      </header>

      <section ref={messageListRef} className="messages">
        {conversation.messages.length === 0 ? (
          <div className="messages-empty rich-empty">
            <span aria-hidden="true" className="rich-empty-mark">
              {">"}
            </span>
            <strong>{t("noMessages")}</strong>
          </div>
        ) : visibleMessages.length === 0 ? (
          <div className="messages-empty rich-empty">
            <span aria-hidden="true" className="rich-empty-mark">
              #
            </span>
            <strong>{t("noVisibleMessages")}</strong>
          </div>
        ) : null}
        {visibleMessages.map((message, index) => {
          const timestampParts = formatMessageTimestampParts(message.created_at, locale, t);
          const previousMessage = visibleMessages[index - 1];
          const showDivider = shouldShowMessageDateDivider(previousMessage, message);

          if (isEventMessage(message)) {
            return (
              <Fragment key={message.id || `event-${index}`}>
                {showDivider ? <MessageTimeDivider parts={timestampParts} /> : null}
                <div className="message-event-row">
                  <div className="message-event-text">{formatEventMessage(message, usersById, locale)}</div>
                </div>
              </Fragment>
            );
          }
          const user = usersById.get(message.sender_id);
          if (!user) {
            return null;
          }
          const own = message.sender_id === currentUserID;
          const isAdmin = user?.role === "admin";
          const messageAgent = agents.find((item) => agentMatchesUser(item, user));
          const messageAgentRunning = isAgentRunning(messageAgent);
          const threadSummary = message.thread;
          const latestThreadReply = threadSummary?.latest_reply;
          return (
            <Fragment key={message.id || `message-${index}`}>
              {showDivider ? <MessageTimeDivider parts={timestampParts} /> : null}
              <div className={`message-row ${own ? "own" : ""} ${isAdmin ? "admin" : ""}`.trim()}>
                <button
                  type="button"
                  className="avatar avatar-button"
                  aria-label={`${t("profilePreview")} ${user.name}`}
                  onClick={(event) => onPreviewUser(user, event.currentTarget)}
                >
                  <AgentAvatarContent
                    avatar={user.avatar}
                    fallback={avatarFallbackText(user.avatar, user.name, user.handle, user.id)}
                  />
                  {messageAgent ? (
                    <span
                      className={`message-avatar-status ${messageAgentRunning ? "online" : ""}`}
                      aria-hidden="true"
                    />
                  ) : null}
                </button>
                <div className="message-card">
                  <div className="message-hover-actions">
                    <button
                      type="button"
                      className="thread-hover-button"
                      aria-label={t("replyInThread")}
                      onClick={() => onOpenThread(message)}
                    >
                      <span className="thread-hover-icon" aria-hidden="true">
                        {IconImage("rooms")}
                      </span>
                      <span className="thread-action-tooltip" aria-hidden="true">
                        {t("replyInThread")}
                      </span>
                    </button>
                  </div>
                  <div className="message-meta">
                    <span className="message-author">{user.name}</span>
                    <MessageTimestamp parts={timestampParts} />
                  </div>
                  <div className="message-bubble">
                    <MessageContent
                      key={`${message.id}:${theme}`}
                      content={message.content}
                      message={message}
                      actionBusy={messageActionBusy}
                      actionError={messageActionError}
                      onAction={onMessageAction}
                    />
                  </div>
                  {threadSummary ? (
                    <div className="message-thread-actions has-thread-summary">
                      <button type="button" className="thread-action-button" onClick={() => onOpenThread(message)}>
                        <span aria-hidden="true">{IconImage("rooms")}</span>
                        <span>{formatThreadReplyCount(threadSummary.reply_count, t)}</span>
                      </button>
                      {latestThreadReply ? (
                        <button type="button" className="thread-latest-reply" onClick={() => onOpenThread(message)}>
                          <span>{t("latestThreadReply")}</span>
                          <strong className="truncate">
                            <MessagePreviewText content={latestThreadReply.content} />
                          </strong>
                        </button>
                      ) : (
                        <button type="button" className="thread-latest-reply" onClick={() => onOpenThread(message)}>
                          <span>{t("threadStarted")}</span>
                          <strong>{formatThreadReplyCount(threadSummary.reply_count, t)}</strong>
                        </button>
                      )}
                    </div>
                  ) : null}
                </div>
              </div>
            </Fragment>
          );
        })}
      </section>

      <footer className="composer">
        {skillPickerOpen ? (
          <SkillPicker
            candidates={skillCandidates}
            activeIndex={skillIndex}
            loading={skillLoading}
            t={t}
            onSelect={(name) => onApplySkillCandidate?.(name)}
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
        <div className="composer-box">
          <div className="composer-input-wrap">
            {draftSegments.length === 0 ? (
              <div className="composer-placeholder" aria-hidden="true">
                {managerProfileIncomplete ? t("profileIncomplete") : t("inputPlaceholder")}
              </div>
            ) : null}
            <div
              ref={editorRef}
              className={`composer-editor ${managerProfileIncomplete ? "disabled" : ""}`}
              contentEditable={managerProfileIncomplete ? "false" : "true"}
              suppressContentEditableWarning={true}
              aria-label={t("inputPlaceholder")}
              onInput={onSyncComposer}
              onClick={onSyncComposer}
              onKeyDown={onComposerKeyDown}
              onCompositionStart={onComposerCompositionStart}
              onCompositionEnd={onComposerCompositionEnd}
              onKeyUp={onSyncComposer}
              onPaste={(event) => {
                event.preventDefault();
                const pasted = event.clipboardData?.getData("text/plain") ?? "";
                const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByHandle);
                if (segments.some((segment) => segment.type === "mention")) {
                  insertComposerSegmentsAtSelection(segments);
                } else {
                  insertPlainTextAtSelection(pasted);
                }
                onSyncComposer();
              }}
            />
            <Button
              variant="primary"
              className="composer-send-button"
              aria-label={t("send")}
              title={t("send")}
              disabled={managerProfileIncomplete || !draftText.trim()}
              onClick={onSendMessage}
            >
              <span className="composer-send-main" aria-hidden="true">
                {IconImage("send")}
              </span>
            </Button>
          </div>
        </div>
        {composerError ? <div className="form-error composer-error">{composerError}</div> : null}
        <div className="composer-tip">{t("composerTip")}</div>
      </footer>
      {activeThreadRootID ? (
        <ThreadPanel
          thread={activeThreadView}
          loading={threadLoading}
          error={threadError}
          draftSegments={threadDraftSegments}
          disabled={managerProfileIncomplete}
          usersById={usersById}
          locale={locale}
          theme={theme}
          showToolCalls={showToolCalls}
          t={t}
          onClose={onCloseThread}
          onDraftChange={onThreadDraftChange}
          threadSkillCandidates={threadSkillCandidates}
          threadSkillIndex={threadSkillIndex}
          threadSkillLoading={threadSkillLoading}
          threadSkillPickerOpen={threadSkillPickerOpen}
          onApplyThreadSkillCandidate={onApplyThreadSkillCandidate}
          onDismissThreadSkillPicker={onDismissThreadSkillPicker}
          onSetThreadSkillIndex={onSetThreadSkillIndex}
          mentionableUsers={conversationMembers}
          onPreviewUser={onPreviewUser}
          onSend={onSendThreadReply}
        />
      ) : null}
      {logModalOpen && logAgent ? (
        <AgentLogsDialog
          agentName={logAgentName}
          content={logContent}
          error={logError}
          loading={logLoading}
          t={t}
          onClose={() => setLogModalOpen(false)}
          onRefresh={refreshAgentLogs}
        />
      ) : null}
    </>
  );
}

type SlashPickerNavigationInput = {
  event: ReactKeyboardEvent<HTMLElement>;
  candidates: string[];
  activeIndex: number;
  pickerOpen: boolean;
  onIndexChange: (index: number) => void;
  onApply: (value: string) => void;
  onDismiss: () => void;
  onPrepareNavigation?: () => void;
};

function handleSlashPickerNavigation({
  event,
  candidates,
  activeIndex,
  pickerOpen,
  onIndexChange,
  onApply,
  onDismiss,
  onPrepareNavigation,
}: SlashPickerNavigationInput): boolean {
  if (!pickerOpen) {
    return false;
  }
  if (event.key === "ArrowDown" && candidates.length > 0) {
    event.preventDefault();
    onPrepareNavigation?.();
    onIndexChange((activeIndex + 1) % candidates.length);
    return true;
  }
  if (event.key === "ArrowUp" && candidates.length > 0) {
    event.preventDefault();
    onPrepareNavigation?.();
    onIndexChange((activeIndex - 1 + candidates.length) % candidates.length);
    return true;
  }
  if (event.key === "Enter" && !event.shiftKey && candidates.length > 0) {
    event.preventDefault();
    onApply(candidates[activeIndex] ?? candidates[0]);
    return true;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    onDismiss();
    return true;
  }
  return false;
}

function MentionPicker({ users = [], activeIndex = 0, className = "", showRole = true, t, onSelect }) {
  const activeOptionRef = useRef<HTMLButtonElement | null>(null);
  const activeUserID = users[activeIndex]?.id || "";

  useLayoutEffect(() => {
    activeOptionRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, activeUserID, users.length]);

  return (
    <div className={`mention-picker ${className}`.trim()}>
      {users.map((user, index) => (
        <button
          key={user.id}
          ref={index === activeIndex ? activeOptionRef : null}
          className={`mention-option ${index === activeIndex ? "active" : ""}`}
          onMouseDown={(event) => {
            event.preventDefault();
            onSelect(user);
          }}
        >
          <span className="avatar">
            <AgentAvatarContent
              avatar={user.avatar}
              fallback={avatarFallbackText(user.avatar, user.name, user.handle, user.id)}
            />
          </span>
          <div>
            <div className="message-author">{user.name}</div>
            <div className="conversation-preview">
              @{user.handle}
              {showRole ? ` · ${localizeRole(user.role, t)}` : ""}
            </div>
          </div>
        </button>
      ))}
    </div>
  );
}

function SkillPicker({ candidates = [], activeIndex = 0, loading = false, className = "", t, onSelect }) {
  const activeOptionRef = useRef<HTMLButtonElement | null>(null);
  const activeSkill = candidates[activeIndex] || "";

  useLayoutEffect(() => {
    activeOptionRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, activeSkill, candidates.length]);

  return (
    <div className={`mention-picker skill-picker ${className}`.trim()}>
      {loading ? <div className="skill-picker-empty">{t("agentWorkspaceLoading")}</div> : null}
      {!loading && candidates.length === 0 ? <div className="skill-picker-empty">{t("skillPickerEmpty")}</div> : null}
      {candidates.map((name, index) => (
        <button
          key={name}
          ref={index === activeIndex ? activeOptionRef : null}
          className={`mention-option skill-option ${index === activeIndex ? "active" : ""}`}
          onMouseDown={(event) => {
            event.preventDefault();
            onSelect(name);
          }}
        >
          <span className="skill-option-mark" aria-hidden="true">
            /
          </span>
          <div>
            <div className="message-author">{name}</div>
          </div>
        </button>
      ))}
    </div>
  );
}

function AgentLogsDialog({ agentName, content, error, loading, t, onClose, onRefresh }) {
  const logsViewerRef = useRef<HTMLPreElement | null>(null);
  const displayContent = content || (loading ? t("agentLogsLoading") : t("agentLogsEmpty"));

  useLayoutEffect(() => {
    const viewer = logsViewerRef.current;
    if (!viewer) {
      return;
    }
    viewer.scrollTop = viewer.scrollHeight;
  }, [content, error, loading]);

  return (
    <DialogRoot
      open={true}
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent className="agent-logs-modal" overlayClassName="agent-logs-backdrop">
        <DialogHeader className="agent-logs-header">
          <div>
            <DialogTitle>{t("agentLogsTitle")}</DialogTitle>
            <DialogDescription>{agentName}</DialogDescription>
          </div>
          <div className="agent-logs-header-actions">
            <Button
              className="icon-button agent-logs-refresh"
              aria-label={t("refreshLogs")}
              title={t("refreshLogs")}
              loading={loading}
              loadingLabel={t("agentLogsLoading")}
              onClick={onRefresh}
            >
              <span className="icon-button-mark" aria-hidden="true">
                <RefreshCw size={18} strokeWidth={2} />
              </span>
            </Button>
            <DialogCloseButton className="icon-button" label={t("close")} />
          </div>
        </DialogHeader>
        <DialogBody className="agent-logs-body">
          {error ? <div className="form-error agent-logs-error">{error}</div> : null}
          <pre ref={logsViewerRef} className="agent-logs-viewer">
            {displayContent}
          </pre>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

function ThreadPanel({
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
  threadSkillCandidates = [],
  threadSkillIndex = 0,
  threadSkillLoading = false,
  threadSkillPickerOpen = false,
  onApplyThreadSkillCandidate = (_name) => {},
  onDismissThreadSkillPicker = () => {},
  onSetThreadSkillIndex = (_index) => {},
  onPreviewUser,
  mentionableUsers = [],
  onSend,
}) {
  const threadBodyRef = useRef<HTMLDivElement | null>(null);
  const threadEditorRef = useRef<HTMLDivElement | null>(null);
  const [mentionState, setMentionState] = useState<ThreadMentionState | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const root = thread?.root ?? null;
  const replies = thread?.replies ?? [];
  const visibleRoot = showToolCalls || !isToolCallMessage(root) ? root : null;
  const visibleReplies = showToolCalls ? replies : replies.filter((message) => !isToolCallMessage(message));
  const latestReplyID = visibleReplies[visibleReplies.length - 1]?.id || "";
  const mentionableUsersByHandle = useMemo(() => {
    const result = new Map<string, (typeof mentionableUsers)[number]>();
    mentionableUsers.forEach((user) => {
      const handle = String(user.handle || user.name || user.id || "").trim().toLowerCase();
      if (!handle) {
        return;
      }
      if (!result.has(handle)) {
        result.set(handle, user);
      }
    });
    return result;
  }, [mentionableUsers]);
  const displayDraftSegments = useMemo(
    () => normalizeComposerSegmentsForDisplay(draftSegments || []),
    [draftSegments],
  );
  const threadMentionCandidates = useMemo(() => {
    if (!mentionState) {
      return [];
    }
    return getMentionCandidates(mentionableUsers, mentionState.query);
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
      renderComposerSegments(editor, displayDraftSegments);
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

  function insertThreadMention(user) {
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
              <MessagePreviewText content={thread?.summary?.context_summary?.root_excerpt || visibleRoot.content || ""} />
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
        {threadSkillPickerOpen ? (
          <SkillPicker
            candidates={threadSkillCandidates}
            activeIndex={threadSkillIndex}
            loading={threadSkillLoading}
            className="thread-skill-picker"
            t={t}
            onSelect={(name) => onApplyThreadSkillCandidate(name)}
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
                candidates: threadSkillCandidates,
                activeIndex: threadSkillIndex,
                pickerOpen: threadSkillPickerOpen,
                onIndexChange: (value) => onSetThreadSkillIndex(value),
                onApply: (value) => onApplyThreadSkillCandidate(value),
                onDismiss: () => {
                  onDismissThreadSkillPicker();
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
            const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByHandle);
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

function ThreadMessage({ message, usersById, locale, theme, t, onPreviewUser, compact = false }) {
  const user = usersById.get(message.sender_id);
  const fallbackName = message.sender_id || "";
  const avatar = user?.avatar || fallbackName.slice(0, 1).toUpperCase();
  const name = user?.name || user?.handle || fallbackName;
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

function MessageTimestamp({ parts }) {
  if (!parts.shortLabel) {
    return null;
  }
  return (
    <time
      className="message-timestamp"
      dateTime={parts.dateTime}
      title={parts.tooltip}
      aria-label={parts.tooltip}
      data-tooltip={parts.tooltip}
      tabIndex={0}
    >
      {parts.shortLabel}
    </time>
  );
}

function MessageTimeDivider({ parts }) {
  if (!parts.dividerLabel) {
    return null;
  }
  return (
    <div className="message-time-divider">
      <time
        className="message-time-divider-label"
        dateTime={parts.dateTime}
        title={parts.tooltip}
        data-tooltip={parts.tooltip}
        tabIndex={0}
      >
        {parts.dividerLabel}
      </time>
    </div>
  );
}

function shouldShowMessageDateDivider(previousMessage, currentMessage) {
  if (!previousMessage) {
    return hasValidMessageTime(currentMessage);
  }
  return !isSameMessageDate(previousMessage, currentMessage);
}

function isSameMessageDate(previousMessage, currentMessage) {
  const previousAt = Date.parse(previousMessage?.created_at || "");
  const currentAt = Date.parse(currentMessage?.created_at || "");
  if (!Number.isFinite(previousAt) || !Number.isFinite(currentAt)) {
    return false;
  }
  const previousDate = new Date(previousAt);
  const currentDate = new Date(currentAt);
  return (
    previousDate.getFullYear() === currentDate.getFullYear() &&
    previousDate.getMonth() === currentDate.getMonth() &&
    previousDate.getDate() === currentDate.getDate()
  );
}

function hasValidMessageTime(message) {
  return Number.isFinite(Date.parse(message?.created_at || ""));
}
