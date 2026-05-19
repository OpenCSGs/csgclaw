import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ConversationView } from "./components";

export function ConversationPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <ConversationView {...controller.conversationViewProps} />;
}
