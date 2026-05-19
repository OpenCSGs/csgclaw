// @ts-nocheck
import {
  formatConversationPreview,
  formatTime,
  isDirectConversation,
  resolveConversationUser,
} from "@/models/conversations";
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
        const displayUser = resolveConversationUser(conversation, currentUserID, usersById);
        const isDirect = isDirectConversation(conversation);
        const avatar = isDirect && displayUser ? displayUser.avatar : conversation.title.slice(0, 2).toUpperCase();
        const color = isDirect && displayUser ? displayUser.accent_hex : "#2563eb";
        return (
          <div
            key={conversation.id}
            className={`conversation-item ${conversation.id === activeConversationId ? "active" : ""}`}
          >
            <button className="conversation-item-main" onClick={() => onSelect(conversation.id)}>
              <div className="avatar" style={{ background: `linear-gradient(135deg, ${color}, #10233f)` }}>
                {avatar}
              </div>
              <div className="conversation-main">
                <div className="conversation-head">
                  <div className="conversation-name truncate">{conversation.title}</div>
                  <div className="section-label">{formatTime(lastMessage?.created_at, locale)}</div>
                </div>
                <div className="conversation-preview truncate">
                  {formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t)}
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
