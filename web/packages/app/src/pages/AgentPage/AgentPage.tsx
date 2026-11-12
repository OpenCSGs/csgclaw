import { PanelEmptyState } from "@/components/business/PanelEmptyState";
import { EntityPane } from "@/components/business/EntityDetail";
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
        <EntityPane className={"agent-detail-pane notification-participant-detail-pane"}>
          <PanelEmptyState title={controller.t("noNotificationBots")} />
        </EntityPane>
      );
    }
    return <ConversationView {...controller.conversationViewProps} />;
  }

  return <AgentView {...agentViewProps} />;
}
