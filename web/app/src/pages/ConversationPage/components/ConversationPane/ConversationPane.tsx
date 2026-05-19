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
    </>
  );
}
