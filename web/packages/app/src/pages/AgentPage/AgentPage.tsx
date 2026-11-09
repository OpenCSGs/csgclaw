import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { WorkspacePaneTypes } from "@/models/routing";
import { ConversationView } from "@/components/business/ConversationWorkspace";
import { useAgentViewBindings } from "@/hooks/workspace/useAgentViewBindings";
import { AgentView } from "@/components/business/AgentDetails";

export function AgentPage() {
  const controller = useWorkspaceControllerContext();
  const agentViewProps = useAgentViewBindings();

  if (!controller.ready) {
    return null;
  }

  if (!agentViewProps?.item) {
    if (controller.activePane.type === WorkspacePaneTypes.notifications) {
      return (
        <section className="entity-pane agent-detail-pane notification-participant-detail-pane">
          <div className="empty-state shell-empty-state">
            <strong>{controller.t("noNotificationBots")}</strong>
          </div>
        </section>
      );
    }
    return <ConversationView {...controller.conversationViewProps} />;
  }

  return <AgentView {...agentViewProps} />;
}
