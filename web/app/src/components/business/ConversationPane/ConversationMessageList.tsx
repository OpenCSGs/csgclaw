import { Fragment, memo, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { MessageContent, MessagePreviewText } from "@/components/business/MessageContent";
import type { MessageAction, MessageActionError, MessageLike } from "@/components/business/MessageContent/types";
import { IconImage } from "@/components/ui/Icons";
import { isAgentRunning, resolveAgentAvatarFallback, type AgentLike } from "@/models/agents";
import {
  agentMatchesUser,
  formatEventMessage,
  formatMessageTimestampParts,
  formatThreadReplyCount,
  isEventMessage,
  localIdentitiesMatch,
  resolveUserByLocalIdentity,
  type IMConversation,
  type IMMessage,
  type IMUser,
  type LocaleCode,
  type TranslateFn,
  type UsersById,
} from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import type { ThemeMode } from "@/shared/theme/theme";
import { MessageTimestamp, MessageTimeDivider } from "./MessageTime";
import { shouldShowMessageDateDivider } from "./messageTimeUtils";
import type { VoidOrPromise } from "./types";

export type ConversationMessageListProps = {
  agents?: AgentLike[];
  conversation: IMConversation;
  currentUserID?: string;
  emptyStateSlot?: ReactNode;
  locale: LocaleCode;
  messageActionBusy: string;
  messageActionError: MessageActionError;
  messageListRef: RefObject<HTMLElement | null>;
  onCancelProfilePreviewClose?: () => void;
  onCloseProfilePreview?: () => void;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onMessageAction: (action: MessageAction, message?: MessageLike | null) => VoidOrPromise;
  onOpenThread: (message: IMMessage) => VoidOrPromise;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  t: TranslateFn;
  theme: ThemeMode;
  usersById: UsersById;
  visibleMessages: IMMessage[];
};

export const ConversationMessageList = memo(function ConversationMessageList({
  agents = [],
  conversation,
  currentUserID = "",
  emptyStateSlot,
  locale,
  messageActionBusy,
  messageActionError,
  messageListRef,
  t,
  theme,
  usersById,
  visibleMessages,
  onCancelProfilePreviewClose,
  onCloseProfilePreview,
  onOpenAgentDetail,
  onMessageAction,
  onOpenThread,
  onPreviewUser,
}: ConversationMessageListProps) {
  const [expandedLongMessages, setExpandedLongMessages] = useState<Record<string, boolean>>({});

  return (
    <section ref={messageListRef} className="messages">
      {conversation.messages.length === 0 ? (
        (emptyStateSlot ?? (
          <div className="messages-empty rich-empty">
            <span aria-hidden="true" className="rich-empty-mark">
              {">"}
            </span>
            <strong>{t("noMessages")}</strong>
          </div>
        ))
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
        const user = resolveUserByLocalIdentity(message.sender_id, usersById);
        if (!user) {
          return null;
        }
        const own = localIdentitiesMatch(message.sender_id, currentUserID);
        const isAdmin = user?.role === "admin";
        const messageAgent = resolveMessageAgent(agents, user, message.sender_id);
        const messageAgentRunning = isAgentRunning(messageAgent);
        const messageAvatar = user.avatar || messageAgent?.avatar;
        const messageAvatarFallback = messageAgent
          ? resolveAgentAvatarFallback(messageAgent, usersById)
          : avatarFallbackText(user.avatar, user.name, user.id);
        const threadSummary = message.thread;
        const latestThreadReply = threadSummary?.latest_reply;
        const messageStateKey = longMessageStateKey(conversation, message, index);
        return (
          <Fragment key={message.id || `message-${index}`}>
            {showDivider ? <MessageTimeDivider parts={timestampParts} /> : null}
            <div className={`message-row ${own ? "own" : ""} ${isAdmin ? "admin" : ""}`.trim()}>
              <button
                type="button"
                className="avatar avatar-button"
                aria-label={`${t("profilePreview")} ${user.name}`}
                onBlur={onCloseProfilePreview}
                onClick={(event) => {
                  if (messageAgent && onOpenAgentDetail) {
                    onOpenAgentDetail(messageAgent, event.currentTarget);
                  }
                }}
                onFocus={(event) => onPreviewUser(user, event.currentTarget)}
                onPointerEnter={(event) => {
                  onCancelProfilePreviewClose?.();
                  onPreviewUser(user, event.currentTarget);
                }}
                onPointerLeave={onCloseProfilePreview}
              >
                <AgentAvatarContent avatar={messageAvatar} fallback={messageAvatarFallback} />
                {messageAgent ? (
                  <span className={`message-avatar-status ${messageAgentRunning ? "online" : ""}`} aria-hidden="true" />
                ) : null}
              </button>
              <div className="message-card">
                <div className="message-hover-actions">
                  <button
                    type="button"
                    className="thread-hover-button"
                    aria-label={t("replyInThread")}
                    data-tooltip={t("replyInThread")}
                    data-tooltip-side="top"
                    onClick={() => onOpenThread(message)}
                  >
                    <span className="thread-hover-icon" aria-hidden="true">
                      {IconImage("rooms")}
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
                    enableLongMessageCollapse={own}
                    longMessageExpanded={own ? expandedLongMessages[messageStateKey] === true : undefined}
                    onAction={onMessageAction}
                    onLongMessageExpandedChange={
                      own
                        ? (expanded) =>
                            setExpandedLongMessages((current) => ({
                              ...current,
                              [messageStateKey]: expanded,
                            }))
                        : undefined
                    }
                    t={t}
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
  );
});

function longMessageStateKey(conversation: IMConversation, message: IMMessage, index: number): string {
  const conversationKey = String(conversation.id || "conversation");
  const messageKey = String(message.id || `${message.created_at || "message"}:${index}`);
  return `${conversationKey}:${messageKey}`;
}

function resolveMessageAgent(
  agents: readonly AgentLike[],
  user: IMUser,
  senderID: string | null | undefined,
): AgentLike | null {
  const senderIdentity = String(senderID || "").trim();
  return (
    agents.find((item) => {
      if (agentMatchesUser(item, user)) {
        return true;
      }
      if (!senderIdentity || senderIdentity === user.id) {
        return false;
      }
      return agentMatchesUser(item, { ...user, id: senderIdentity, user_id: user.id });
    }) ?? null
  );
}
