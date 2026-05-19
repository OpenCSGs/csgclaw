import { ConversationPane } from "../ConversationPane";

type ConversationViewProps = Parameters<typeof ConversationPane>[0] & {
  conversation: Parameters<typeof ConversationPane>[0]["conversation"] | null;
};

export function ConversationView({ conversation, t, ...props }: ConversationViewProps) {
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
