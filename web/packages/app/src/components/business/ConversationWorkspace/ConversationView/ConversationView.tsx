import { PanelEmptyState } from "@/components/business/PanelEmptyState";
import type { ConversationPaneProps } from "@/components/business/ConversationPane";
import { ConversationPane } from "../ConversationPane";

type ConversationViewProps = Partial<Omit<ConversationPaneProps, "conversation" | "t">> & {
  conversation?: ConversationPaneProps["conversation"] | null;
  t?: ConversationPaneProps["t"];
};

export function ConversationView({ conversation, t, ...props }: ConversationViewProps) {
  const translate = t ?? ((key: string) => key);
  if (!conversation) {
    return <PanelEmptyState title={translate("emptyConversation")} mark={">"} />;
  }

  return (
    <ConversationPane
      {...(props as Omit<ConversationPaneProps, "conversation" | "t">)}
      conversation={conversation}
      t={translate}
    />
  );
}
