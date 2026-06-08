import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ConversationPage } from "@/pages/ConversationPage";
import { AgentView } from "./components";

export function AgentPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  const agentViewProps = controller.agentViewProps;
  if (!agentViewProps?.item) {
    return <ConversationPage />;
  }

  return <AgentView {...agentViewProps} item={agentViewProps.item} />;
}
