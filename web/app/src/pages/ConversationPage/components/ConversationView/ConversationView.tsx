// @ts-nocheck
import { ConversationPane } from "../ConversationPane";

export function ConversationView({ conversation, t, ...props }) {
  if (!conversation) {
    return (
      <div className="empty-state shell-empty-state">
        <span className="rich-empty-mark" aria-hidden="true">
          {">"}
        </span>
        <strong>{t("emptyConversation")}</strong>
      </div>
    );
  }

  return <ConversationPane conversation={conversation} t={t} {...props} />;
}
