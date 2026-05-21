import { useLayoutEffect, useRef } from "react";
import { X } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import { MessageContent } from "@/components/business/MessageContent";
import { Button } from "@/components/ui";
import { AddUserIcon, IconImage, TrashIcon, UsersIcon, WrenchIcon } from "@/components/ui/Icons";
import {
  insertComposerSegmentsAtSelection,
  insertPlainTextAtSelection,
  normalizeTextMentions,
} from "@/models/composer";
import { normalizeAuthProviderName, providerNeedsAuth } from "@/models/agents";
import {
  formatEventMessage,
  formatMessagePreviewText,
  formatThreadReplyCount,
  formatTime,
  getConversationDescription,
  isDirectConversation,
  isEventMessage,
} from "@/models/conversations";
import { localizeRole } from "@/shared/i18n";

export function ConversationPane({
  conversation,
  visibleMessages,
  currentUserID,
  usersById,
  locale,
  t,
  theme,
  selectedMessageCount,
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
  threadDraft,
  onOpenThread,
  onCloseThread,
  onThreadDraftChange,
  onSendThreadReply,
}) {
  const description = getConversationDescription(conversation, currentUserID, usersById, locale, t);
  const managerProvider = normalizeAuthProviderName(managerProfile?.provider);

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
                              style={{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}
                              aria-label={`${t("profilePreview")} ${user.name}`}
                              onClick={(event) => onPreviewUser(user, event.currentTarget)}
                            >
                              {user.avatar}
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
        {visibleMessages.map((message) => {
          if (isEventMessage(message)) {
            return (
              <div key={message.id} className="message-event-row">
                <div className="message-event-text">{formatEventMessage(message, usersById, locale)}</div>
              </div>
            );
          }
          const user = usersById.get(message.sender_id);
          if (!user) {
            return null;
          }
          const own = message.sender_id === currentUserID;
          const isAdmin = user?.role === "admin";
          const threadSummary = message.thread;
          const latestThreadReply = threadSummary?.latest_reply;
          return (
            <div key={message.id} className={`message-row ${own ? "own" : ""} ${isAdmin ? "admin" : ""}`.trim()}>
              <button
                type="button"
                className="avatar avatar-button"
                style={{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}
                aria-label={`${t("profilePreview")} ${user.name}`}
                onClick={(event) => onPreviewUser(user, event.currentTarget)}
              >
                {user.avatar}
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
                  <span>{formatTime(message.created_at, locale)}</span>
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
                        <strong className="truncate">{formatMessagePreviewText(latestThreadReply.content)}</strong>
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
          );
        })}
      </section>

      <footer className="composer">
        {mentionCandidates.length > 0 ? (
          <div className="mention-picker">
            {mentionCandidates.map((user, index) => (
              <button
                key={user.id}
                className={`mention-option ${index === mentionIndex ? "active" : ""}`}
                onMouseDown={(event) => {
                  event.preventDefault();
                  onApplyMention(user);
                }}
              >
                <span className="avatar" style={{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}>
                  {user.avatar}
                </span>
                <div>
                  <div className="message-author">{user.name}</div>
                  <div className="conversation-preview">
                    @{user.handle} · {localizeRole(user.role, t)}
                  </div>
                </div>
              </button>
            ))}
          </div>
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
          draft={threadDraft}
          disabled={managerProfileIncomplete}
          usersById={usersById}
          locale={locale}
          theme={theme}
          t={t}
          onClose={onCloseThread}
          onDraftChange={onThreadDraftChange}
          onSend={onSendThreadReply}
        />
      ) : null}
    </>
  );
}

function ThreadPanel({
  thread,
  loading,
  error,
  draft,
  disabled,
  usersById,
  locale,
  theme,
  t,
  onClose,
  onDraftChange,
  onSend,
}) {
  const threadBodyRef = useRef<HTMLDivElement | null>(null);
  const root = thread?.root ?? null;
  const replies = thread?.replies ?? [];
  const latestReplyID = replies[replies.length - 1]?.id || "";

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
  }, [root?.id, replies.length, latestReplyID, loading]);

  return (
    <aside className="thread-panel" aria-label={t("threadPanelTitle")}>
      <div className="thread-panel-header">
        <div>
          <div className="thread-panel-kicker">{t("threadPanelTitle")}</div>
          <div className="thread-panel-title truncate">
            {formatMessagePreviewText(thread?.summary?.context_summary?.root_excerpt || root?.content || "")}
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
        {root ? (
          <div className="thread-root">
            <ThreadMessage message={root} usersById={usersById} locale={locale} theme={theme} />
          </div>
        ) : null}
        <div className="thread-replies">
          <div className="thread-section-title">{formatThreadReplyCount(replies.length, t)}</div>
          {replies.length > 0 ? (
            replies.map((message) => (
              <ThreadMessage key={message.id} message={message} usersById={usersById} locale={locale} theme={theme} />
            ))
          ) : (
            <div className="thread-empty">{t("threadNoReplies")}</div>
          )}
        </div>
      </div>
      <div className="thread-composer">
        <textarea
          value={draft}
          placeholder={disabled ? t("profileIncomplete") : t("threadComposerPlaceholder")}
          disabled={disabled}
          onChange={(event) => onDraftChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              onSend();
            }
          }}
        />
        <Button
          variant="primary"
          className="thread-send-button"
          disabled={disabled || !String(draft || "").trim()}
          onClick={onSend}
        >
          <span aria-hidden="true">{IconImage("send")}</span>
          <span>{t("send")}</span>
        </Button>
      </div>
    </aside>
  );
}

function ThreadMessage({ message, usersById, locale, theme, compact = false }) {
  const user = usersById.get(message.sender_id);
  const fallbackName = message.sender_id || "";
  const avatar = user?.avatar || fallbackName.slice(0, 1).toUpperCase();
  const name = user?.name || user?.handle || fallbackName;

  return (
    <div className={`thread-message ${compact ? "compact" : ""}`.trim()}>
      <div
        className="thread-message-avatar"
        style={{ background: `linear-gradient(135deg, ${user?.accent_hex || "#4d6ad6"}, #10233f)` }}
        aria-hidden="true"
      >
        {avatar}
      </div>
      <div className="thread-message-main">
        <div className="message-meta">
          <span className="message-author">{name}</span>
          <span>{formatTime(message.created_at, locale)}</span>
        </div>
        <div className="thread-message-bubble">
          <MessageContent key={`${message.id}:${theme}`} content={message.content} message={message} />
        </div>
      </div>
    </div>
  );
}
