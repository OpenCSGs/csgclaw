import { Button } from "@/components/ui";
import { useConversationMemory, conversationMemoryAgent } from "./useConversationMemory";
import memoryStyles from "./ConversationMemory.module.css";
import { PanelEmptyState } from "@/components/business/PanelEmptyState";
import avatarLayoutStyles from "@/components/business/AgentAvatar/AvatarLayout.module.css";
import messageLayoutStyles from "@/components/business/ConversationPane/MessageLayout.module.css";
import conversationMessageListStyles from "./ConversationMessageList.module.css";
import conversationPaneStyles from "./ConversationPane.module.css";
import conversationThreadsStyles from "./ConversationThreads.module.css";
import { messageResources } from "@/models/conversationResources";
import { useEventCallback } from "@/hooks/useEventCallback";
import { emptyCompletedProgress } from "@/models/turnProgress";
import { DiagnosticMessageAction } from "@/components/business/TurnDiagnostics";
import { isImageAttachment } from "@/models/attachments";
import { ImageGenerationStatus } from "./ImageGenerationStatus";
import { Fragment, memo, useMemo, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { MessageContent, MessagePreviewText } from "@/components/business/MessageContent";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import type {
  ProgressControlSlots,
  CitationSelectHandler,
  MessageAction,
  MessageActionFeedback,
  MessageLike,
} from "@/components/business/MessageContent/types";
import { IconImage } from "@/components/business/Icons";
import { isAgentAvailable, resolveAgentAvatarFallback, type AgentLike } from "@/models/agents";
import {
  formatEventMessage,
  formatMessageTimestampParts,
  formatThreadReplyCount,
  groupVideoGenerationMessages,
  isEventMessage,
  localIdentitiesMatch,
  resolveUserByLocalIdentity,
  threadHasReplies,
  userDisplayName,
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
import { VideoGenerationCard } from "./VideoGenerationCard";
import { MessageTimeline, buildMessageTimelineItems, resolveMessageAgent } from "./MessageTimeline";
import timelineStyles from "./MessageTimeline/MessageTimeline.module.css";

export type ConversationMessageListProps = {
  agents?: AgentLike[];
  conversation: IMConversation;
  currentUserID?: string;
  emptyStateSlot?: ReactNode;
  headerSlot?: ReactNode;
  footerSlot?: ReactNode;
  hasNewMessages?: boolean;
  renderTurnControls?: (message: IMMessage) => ProgressControlSlots | null;
  renderMessageFooter?: (message: IMMessage) => ReactNode;
  locale: LocaleCode;
  messageActionBusy: string;
  messageActionFeedback: MessageActionFeedback;
  messageListRef: RefObject<HTMLElement | null>;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onMessageAction: (action: MessageAction, message?: MessageLike | null) => VoidOrPromise;
  onOpenThread: (message: IMMessage) => VoidOrPromise;
  onFollowLatest?: () => void;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  onCitationSelect?: CitationSelectHandler;
  t: TranslateFn;
  theme: ThemeMode;
  usersById: UsersById;
  visibleMessages: IMMessage[];
};

export function ConversationMessageList(props: ConversationMessageListProps) {
  const onMessageAction = useEventCallback(props.onMessageAction);
  const onOpenThread = useEventCallback(props.onOpenThread);
  const onPreviewUser = useEventCallback(props.onPreviewUser);
  const onOpenAgentDetail = useEventCallback(props.onOpenAgentDetail);
  const onPreviewAttachment = useEventCallback(props.onPreviewAttachment);
  const onQuestionSelect = useEventCallback(props.onQuestionSelect);
  const onCitationSelect = useEventCallback(props.onCitationSelect);
  return (
    <MessageListContent
      {...props}
      onMessageAction={onMessageAction}
      onOpenThread={onOpenThread}
      onPreviewUser={onPreviewUser}
      onOpenAgentDetail={props.onOpenAgentDetail ? onOpenAgentDetail : undefined}
      onPreviewAttachment={props.onPreviewAttachment ? onPreviewAttachment : undefined}
      onQuestionSelect={props.onQuestionSelect ? onQuestionSelect : undefined}
      onCitationSelect={props.onCitationSelect ? onCitationSelect : undefined}
    />
  );
}

const MessageListContent = memo(function MessageListContent({
  agents = [],
  conversation,
  currentUserID = "",
  emptyStateSlot,
  headerSlot,
  footerSlot,
  hasNewMessages,
  renderMessageFooter,
  renderTurnControls,
  locale,
  messageActionBusy,
  messageActionFeedback,
  messageListRef,
  t,
  theme,
  usersById,
  visibleMessages,
  onOpenAgentDetail,
  onMessageAction,
  onOpenThread,
  onFollowLatest,
  onPreviewUser,
  onPreviewAttachment,
  onQuestionSelect,
  onCitationSelect,
}: ConversationMessageListProps) {
  const memory = useConversationMemory(
    conversationMemoryAgent(conversation, agents, currentUserID, usersById),
    conversation.id,
  );
  const [memoryEnableState, setMemoryEnableState] = useState<"idle" | "busy" | "error">("idle");
  const [expandedLongMessages, setExpandedLongMessages] = useState<Record<string, boolean>>({});
  const videoGroups = useMemo(() => groupVideoGenerationMessages(visibleMessages), [visibleMessages]);
  const displayMessages = useMemo(
    () => visibleMessages.filter((message) => !message.id || !videoGroups.childMessageIDs.has(message.id)),
    [visibleMessages, videoGroups],
  );

  const timelineItems = useMemo(
    () => buildMessageTimelineItems(displayMessages, agents, usersById, t("attachment")),
    [displayMessages, agents, usersById, t],
  );

  return (
    <div className={`conversation-message-area ${timelineStyles.area}`}>
      <section ref={messageListRef} className={`messages ${messageLayoutStyles["messages"]}`}>
        {headerSlot}
        {memory.available && !memory.enabled ? (
          <div className={memoryStyles.enable}>
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={memoryEnableState === "busy"}
              onClick={async () => {
                setMemoryEnableState("busy");
                try {
                  await memory.enable();
                  setMemoryEnableState("idle");
                } catch {
                  setMemoryEnableState("error");
                }
              }}
            >
              {t("agentMemoryEnabled")}
            </Button>
            {memoryEnableState === "error" ? <span role="alert">{t("agentMemoryUpdateFailed")}</span> : null}
          </div>
        ) : null}
        {conversation.messages.length === 0 ? (
          (emptyStateSlot ?? (
            <PanelEmptyState
              title={t("noMessages")}
              mark={">"}
              className={conversationMessageListStyles["emptyCard"]}
            />
          ))
        ) : displayMessages.length === 0 ? (
          <PanelEmptyState
            title={t("noVisibleMessages")}
            mark="#"
            className={conversationMessageListStyles["emptyCard"]}
          />
        ) : null}
        {displayMessages.map((message, index) => {
          if (emptyCompletedProgress(message)) return null;
          const timestampParts = formatMessageTimestampParts(message.created_at, locale, t);
          const previousMessage = displayMessages[index - 1];
          const showDivider = shouldShowMessageDateDivider(previousMessage, message);

          if (isEventMessage(message)) {
            return (
              <Fragment key={message.id || `event-${index}`}>
                {showDivider ? <MessageTimeDivider parts={timestampParts} /> : null}
                <div className="message-event-row">
                  <div className={conversationMessageListStyles["message-event-text"]}>
                    {formatEventMessage(message, usersById, locale)}
                  </div>
                </div>
              </Fragment>
            );
          }
          const user = resolveUserByLocalIdentity(message.sender_id, usersById);
          if (!user) {
            return null;
          }
          const authorName = user.role === "cloud" ? userDisplayName(message.sender_id, usersById) : user.name;
          const own = localIdentitiesMatch(message.sender_id, currentUserID);
          const isAdmin = user?.role === "admin";
          const messageAgent = resolveMessageAgent(agents, user, message.sender_id);
          const messageAgentRunning = isAgentAvailable(messageAgent);
          const messageAvatar = user.avatar || messageAgent?.avatar;
          const messageAvatarFallback = messageAgent
            ? resolveAgentAvatarFallback(messageAgent, usersById)
            : avatarFallbackText(user.avatar, user.name, user.id);
          const threadSummary = threadHasReplies(message.thread) ? message.thread : null;
          const latestThreadReply = threadSummary?.latest_reply;
          const messageStateKey = longMessageStateKey(conversation, message, index);
          const embeddedVideos = message.id ? videoGroups.childrenByParentID.get(message.id) || [] : [];
          const isVideoGeneration = Boolean(message.metadata?.video_generation);
          const memorySource = memory.sourceFor(message);
          return (
            <Fragment key={message.id || `message-${index}`}>
              {showDivider ? <MessageTimeDivider parts={timestampParts} /> : null}
              <div
                className={`message-row ${own ? "own" : ""} ${isAdmin ? "admin" : ""} ${message.metadata?.image_generation || isVideoGeneration ? conversationPaneStyles["image-generation-message"] : ""}`.trim()}
                data-message-id={message.id || undefined}
              >
                <button
                  type="button"
                  className={`avatar ${avatarLayoutStyles["avatar"]} avatar-button ${avatarLayoutStyles["avatar-button"]}`}
                  aria-label={`${t(messageAgent ? "agentDetailPanel" : "profilePreview")} ${authorName}`}
                  onClick={(event) => {
                    if (messageAgent && onOpenAgentDetail) {
                      onOpenAgentDetail(messageAgent, event.currentTarget);
                      return;
                    }
                    onPreviewUser(user, event.currentTarget);
                  }}
                >
                  <AgentAvatarContent avatar={messageAvatar} fallback={messageAvatarFallback} />
                  {messageAgent ? (
                    <span
                      className={`message-avatar-status ${messageAgentRunning ? "online" : ""}`}
                      aria-hidden="true"
                    />
                  ) : null}
                </button>
                <div className={`message-card ${messageLayoutStyles["message-card"]}`}>
                  <div className={`message-meta ${messageLayoutStyles["message-meta"]}`}>
                    <span className={`message-author ${messageLayoutStyles["message-author"]}`}>
                      {authorName}
                      {user.agent_state && (
                        <small>
                          {" "}
                          · {t(user.agent_state === "archived" ? "agentStateArchived" : "agentStateDeleted")}
                        </small>
                      )}
                    </span>
                    <MessageTimestamp parts={timestampParts} />
                  </div>
                  {(message.content || messageResources(message.metadata).length > 0) &&
                  !message.metadata?.image_generation ? (
                    <div className={`message-bubble ${messageLayoutStyles["message-bubble"]}`}>
                      <MessageContent
                        key={`${message.id}:${theme}`}
                        content={message.content}
                        message={message}
                        allowLegacyToolFormat={Boolean(messageAgent)}
                        progressControls={renderTurnControls?.(message)}
                        videoMessages={embeddedVideos}
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
                        onCitationSelect={onCitationSelect}
                        t={t}
                      />
                    </div>
                  ) : null}
                  <DiagnosticMessageAction metadata={message.metadata} t={t} />
                  <ImageGenerationStatus message={message} roomID={conversation.id} t={t} />
                  {isVideoGeneration ? (
                    <VideoGenerationCard message={message} t={t} onPreviewAttachment={onPreviewAttachment} />
                  ) : (
                    <MessageAttachments
                      attachments={message.attachments}
                      t={t}
                      onPreviewAttachment={onPreviewAttachment}
                    />
                  )}
                  {embeddedVideos.map((video) => (
                    <VideoGenerationCard
                      key={video.id || String(video.created_at || video.content)}
                      message={video}
                      t={t}
                      onPreviewAttachment={onPreviewAttachment}
                    />
                  ))}
                  {!conversation.cloud_agent && threadSummary ? (
                    <div className="message-thread-actions has-thread-summary">
                      <button
                        type="button"
                        className={conversationThreadsStyles["thread-action-button"]}
                        onClick={() => onOpenThread(message)}
                      >
                        <span aria-hidden="true">{IconImage("rooms")}</span>
                        <span>{formatThreadReplyCount(threadSummary.reply_count, t)}</span>
                      </button>
                      {latestThreadReply ? (
                        <button
                          type="button"
                          className={conversationThreadsStyles["thread-latest-reply"]}
                          onClick={() => onOpenThread(message)}
                        >
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
                        <button
                          type="button"
                          className={conversationThreadsStyles["thread-latest-reply"]}
                          onClick={() => onOpenThread(message)}
                        >
                          <span>{t("threadStarted")}</span>
                          <strong>{formatThreadReplyCount(threadSummary.reply_count, t)}</strong>
                        </button>
                      )}
                    </div>
                  ) : null}
                  <ConversationMessageActions
                    metadata={message.metadata}
                    className="message-hover-actions"
                    leading={renderMessageFooter?.(message)}
                    content={message.metadata?.image_generation ? null : message.content}
                    image={
                      message.metadata?.image_generation ? message.attachments?.find(isImageAttachment) : undefined
                    }
                    onForgetMemory={memorySource ? () => memory.forget(memorySource) : undefined}
                    onOpenThread={conversation.cloud_agent ? undefined : () => onOpenThread(message)}
                    t={t}
                  />
                </div>
              </div>
            </Fragment>
          );
        })}
        {footerSlot}
      </section>
      {hasNewMessages ? (
        <button
          type="button"
          className={conversationMessageListStyles["message-list-new-messages"]}
          onClick={onFollowLatest}
        >
          {t("newMessages")} · {t("progressLatest")}
        </button>
      ) : null}
      <MessageTimeline key={conversation.id} items={timelineItems} scrollRef={messageListRef} t={t} />
    </div>
  );
});

function longMessageStateKey(conversation: IMConversation, message: IMMessage, index: number): string {
  const conversationKey = String(conversation.id || "conversation");
  const messageKey = String(message.id || `${message.created_at || "message"}:${index}`);
  return `${conversationKey}:${messageKey}`;
}
