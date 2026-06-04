import {
  formatConversationPreview,
  formatTime,
  isDirectConversation,
  resolveConversationUser,
} from "@/models/conversations";
import { MessagePreviewText } from "@/components/business/MessageContent";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { RoomAvatar, resolveRoomAvatarMembers } from "@/components/business/RoomAvatar";
import { TrashIcon } from "@/components/ui/Icons";

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
}) {
  if (!items.length) {
    return null;
  }

  return (
    <section className="conversation-section">
      {items.map((conversation) => {
        const lastMessage = conversation.messages[conversation.messages.length - 1];
        const displayUser = isDirectConversation(conversation)
          ? resolveConversationUser(conversation, currentUserID, usersById)
          : null;
        const roomAvatarMembers = resolveRoomAvatarMembers(conversation, usersById, currentUserID);
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
                    fallback={avatarFallbackText(displayUser.avatar, displayUser.name, displayUser.handle, displayUser.id)}
                  />
                </div>
              ) : (
                <RoomAvatar members={roomAvatarMembers} count={conversation.members.length} size={48} />
              )}
              <div className="conversation-main">
                <div className="conversation-head">
                  <div className="conversation-name truncate">{conversation.title}</div>
                  <div className="section-label">{formatTime(lastMessage?.created_at, locale)}</div>
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
              title={`${t("deleteRoom")} ${conversation.title}`}
              onClick={(event) => {
                event.stopPropagation();
                onDelete(conversation.id);
              }}
            >
              <span className="conversation-delete-icon" aria-hidden="true">
                <TrashIcon />
              </span>
            </button>
          </div>
        );
      })}
    </section>
  );
}
