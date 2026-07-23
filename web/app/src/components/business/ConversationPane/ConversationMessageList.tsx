import { Fragment, memo, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { MessageContent, MessagePreviewText } from "@/components/business/MessageContent";
import type { MessageAction, MessageActionFeedback, MessageLike } from "@/components/business/MessageContent/types";
import { IconImage } from "@/components/ui/Icons";
import { isAgentRunning, resolveAgentAvatarFallback, type AgentLike } from "@/models/agents";
import {
  formatEventMessage,
  formatMessageTimestampParts,
  formatThreadReplyCount,
  isEventMessage,
  localIdentitiesMatch,
  resolveAgentForUser,
  resolveUserByLocalIdentity,
  threadHasReplies,
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
import { MessageAttachments } from "./ConversationAttachments";
import { ConversationMessageActions } from "./ConversationMessageActions";
import { shouldShowMessageDateDivider } from "./messageTimeUtils";
import type { VoidOrPromise } from "./types";

export type ConversationMessageListProps = {
  agents?: AgentLike[];
  conversation: IMConversation;
  currentUserID?: string;
  emptyStateSlot?: ReactNode;
  locale: LocaleCode;
  messageActionBusy: string;
  messageActionFeedback: MessageActionFeedback;
  messageListRef: RefObject<HTMLElement | null>;
  onCancelProfilePreviewClose?: () => void;
  onCloseProfilePreview?: () => void;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onMessageAction: (action: MessageAction, message?: MessageLike | null) => VoidOrPromise;
  onOpenThread: (message: IMMessage) => VoidOrPromise;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
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
  messageActionFeedback,
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
  onQuestionSelect,
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
        const threadSummary = threadHasReplies(message.thread) ? message.thread : null;
        const latestThreadReply = threadSummary?.latest_reply;
        const messageStateKey = longMessageStateKey(conversation, message, index);
        return (
          <Fragment key={message.id || `message-${index}`}>
            {showDivider ? <MessageTimeDivider parts={timestampParts} /> : null}
            <div
              className={`message-row ${own ? "own" : ""} ${isAdmin ? "admin" : ""}`.trim()}
              data-message-id={message.id || undefined}
            >
              <button
                type="button"
                className="avatar avatar-button"
                aria-label={`${t("profilePreview")} ${user.name}`}
                onBlur={onCloseProfilePreview}
                onClick={(event) => {
                  if (messageAgent && onOpenAgentDetail) {
                    onOpenAgentDetail(messageAgent, event.currentTarget);
                    return;
                  }
                  onPreviewUser(user, event.currentTarget);
                }}
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
                <div className="message-meta">
                  <span className="message-author">{user.name}</span>
                  <MessageTimestamp parts={timestampParts} />
                </div>
                {message.content ? (
                  <div className="message-bubble">
                    <MessageContent
                      key={`${message.id}:${theme}`}
                      content={message.content}
                      message={message}
                      actionBusy={messageActionBusy}
                      actionFeedback={messageActionFeedback}
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
                      onQuestionSelect={onQuestionSelect}
                      t={t}
                    />
                  </div>
                ) : null}
                <MessageAttachments attachments={message.attachments} t={t} />
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
                          <MessagePreviewText
                            content={
                              latestThreadReply.content || latestThreadReply.attachments?.[0]?.name || t("attachment")
                            }
                          />
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
                <ConversationMessageActions
                  className="message-hover-actions"
                  content={message.content}
                  onOpenThread={() => onOpenThread(message)}
                  t={t}
                />
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
  const alternateUser =
    senderIdentity && senderIdentity !== user.id ? { ...user, id: senderIdentity, user_id: user.id } : null;
  return resolveAgentForUser(agents, user, alternateUser ? [alternateUser] : []);
}
