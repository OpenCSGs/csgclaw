import { ConversationPane } from "../ConversationPane";
import type { ConversationPaneProps } from "../ConversationPane";

type ConversationViewProps = Partial<Omit<ConversationPaneProps, "conversation" | "t">> & {
  conversation?: ConversationPaneProps["conversation"] | null;
  t?: ConversationPaneProps["t"];
};

export function ConversationView({ conversation, t, ...props }: ConversationViewProps) {
  const translate = t ?? ((key: string) => key);
  if (!conversation) {
    return (
      <div className="empty-state shell-empty-state">
        <span className="rich-empty-mark" aria-hidden="true">
          {">"}
        </span>
        <strong>{translate("emptyConversation")}</strong>
      </div>
    );
  }

  return (
    <ConversationPane
      {...(props as Omit<ConversationPaneProps, "conversation" | "t">)}
      conversation={conversation}
      t={translate}
    />
  );
}
