import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { WorkspacePaneTypes } from "@/models/routing";
import { ConversationPage } from "@/pages/ConversationPage";
import { AgentView } from "./components";

export function AgentPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  const agentViewProps = controller.agentViewProps;
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
    return <ConversationPage />;
  }

  return <AgentView {...agentViewProps} item={agentViewProps.item} />;
}
