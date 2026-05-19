import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ConversationPage } from "@/pages/ConversationPage";
import { AgentView } from "./components";

export function AgentPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  if (!controller.agentViewProps.item) {
    return <ConversationPage />;
  }

  return <AgentView {...controller.agentViewProps} />;
}
