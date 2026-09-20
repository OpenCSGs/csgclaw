import { useLocation, useNavigate } from "react-router-dom";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { WorkspacePaneTypes } from "@/models/routing";
import { ConversationPage } from "@/pages/ConversationPage";
import { AgentView } from "./components";

export function AgentPage() {
  const controller = useWorkspaceControllerContext();
  const location = useLocation();
  const navigate = useNavigate();
  const search = new URLSearchParams(location.search);

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

  return (
    <AgentView
      {...agentViewProps}
      item={agentViewProps.item}
      requestedProfileTab={search.get("tab") || undefined}
      requestedAppID={search.get("app") || undefined}
      requestedAddAppID={search.get("add_app") || undefined}
      onProfileTabChange={(tab, appID) => {
        const next = new URLSearchParams(location.search);
        next.set("tab", tab);
        next.delete("add_app");
        if (appID) next.set("app", appID);
        else next.delete("app");
        void navigate({ pathname: location.pathname, search: next.toString() });
      }}
    />
  );
}
