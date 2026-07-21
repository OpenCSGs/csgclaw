import {
  formatConversationPreview,
  formatSidebarTime,
  isDirectConversation,
  resolveConversationUser,
} from "@/models/conversations";
import { MessagePreviewText } from "@/components/business/MessageContent";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { RoomAvatar, resolveRoomAvatarMembers } from "@/components/business/RoomAvatar";
import { Tooltip } from "@/components/ui";
import { TrashIcon } from "@/components/ui/Icons";
import type { IMConversation, LocaleCode, TranslateFn, UsersById } from "@/models/conversations";
import { conversationPendingQuestionCount } from "@/models/agentActivity";

export function ConversationSection({
  title,
  items,
  activeConversationId,
  currentUserID,
  usersById,
  locale,
  t,
  onSelect,
  onDelete,
}: {
  activeConversationId: string;
  currentUserID: string;
  items: IMConversation[];
  locale: LocaleCode;
  onDelete: (id: string) => void | Promise<void>;
  onSelect: (id: string) => void;
  t: TranslateFn;
  title: string;
  usersById: UsersById;
}) {
  if (!items.length) {
    return null;
  }

  return (
    <section className="conversation-section" aria-label={title}>
      {items.map((conversation) => {
        const lastMessage = conversation.messages[conversation.messages.length - 1];
        const displayUser = isDirectConversation(conversation)
          ? resolveConversationUser(conversation, currentUserID, usersById)
          : null;
        const roomAvatarMembers = resolveRoomAvatarMembers(conversation, usersById, currentUserID);
        const questionCount = conversationPendingQuestionCount(conversation);
        return (
          <div
            key={conversation.id}
            className={`conversation-item ${conversation.id === activeConversationId ? "active" : ""}`}
          >
            <button className="conversation-item-main" onClick={() => onSelect(conversation.id)}>
              {displayUser ? (
                <div className="avatar" aria-hidden="true">
                  <AgentAvatarContent
                    avatar={displayUser.avatar}
                    fallback={avatarFallbackText(displayUser.avatar, displayUser.name, displayUser.id)}
                  />
                </div>
              ) : (
                <RoomAvatar members={roomAvatarMembers} count={conversation.members.length} size={48} />
              )}
              <div className="conversation-main">
                <div className="conversation-head">
                  <div className="conversation-name truncate">{conversation.title}</div>
                  {questionCount > 0 ? (
                    <span
                      className="pending-question-badge"
                      aria-label={t("questionPendingCount", { count: questionCount })}
                    >
                      ? {questionCount}
                    </span>
                  ) : null}
                  <div className="section-label">{formatSidebarTime(lastMessage?.created_at, locale, t)}</div>
                </div>
                <div className="conversation-preview truncate">
                  <MessagePreviewText
                    content={formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t)}
                  />
                </div>
              </div>
            </button>
            <button
              className="btn btn-outline-danger btn-sm conversation-delete-button"
              aria-label={`${t("deleteRoom")} ${conversation.title}`}
              onClick={(event) => {
                event.stopPropagation();
                onDelete(conversation.id);
              }}
            >
              <Tooltip content={`${t("deleteRoom")} ${conversation.title}`}>
                <span className="conversation-delete-icon" aria-hidden="true">
                  <TrashIcon />
                </span>
              </Tooltip>
            </button>
          </div>
        );
      })}
    </section>
  );
}
