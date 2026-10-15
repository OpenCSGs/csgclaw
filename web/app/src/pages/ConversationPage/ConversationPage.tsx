import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ConversationView } from "@/components/business/ConversationWorkspace";

export function ConversationPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <ConversationView {...controller.conversationViewProps} />;
}
