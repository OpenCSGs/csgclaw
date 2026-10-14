import { useLocation, useNavigate } from "react-router-dom";
import { useWorkspaceControllerContext } from "./useWorkspaceControllerContext";

export function useAgentViewBindings() {
  const controller = useWorkspaceControllerContext();
  const location = useLocation();
  const navigate = useNavigate();
  if (!controller.ready || !controller.agentViewProps.item) return null;
  const search = new URLSearchParams(location.search);
  return {
    ...controller.agentViewProps,
    item: controller.agentViewProps.item,
    requestedProfileTab: search.get("tab") === "connectors" ? "apps" : search.get("tab") || undefined,
    requestedAppID: search.get("connector") || undefined,
    requestedAddAppID: search.get("add_connector") || undefined,
    onProfileTabChange: (tab: string, appID?: string) => {
      const next = new URLSearchParams(location.search);
      next.set("tab", tab === "apps" || tab === "mcp" ? "connectors" : tab);
      next.delete("add_connector");
      if (appID) next.set("connector", appID);
      else next.delete("connector");
      void navigate({ pathname: location.pathname, search: next.toString() });
    },
  };
}
