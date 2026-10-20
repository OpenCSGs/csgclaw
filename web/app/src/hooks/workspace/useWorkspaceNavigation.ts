import { useCallback, useEffect } from "react";
import { settingsDialogSearch, profileDialogSearch } from "@/models/workspaceDialogs";
import {
  DefaultWorkspacePaneIds,
  WorkspacePaneTypes,
  paneFromLocation,
  pathForPane,
  pathForAgentApps,
} from "@/models/routing";
import type { WorkspacePane } from "@/models/routing";
import type { NavigatePaneOptions, UseWorkspaceNavigationArgs, WorkspaceNavigationController } from "./types";

export function useWorkspaceNavigation({
  location,
  navigate,
  dataReady,
  setActiveConversationId,
  rooms,
}: UseWorkspaceNavigationArgs): WorkspaceNavigationController {
  const navigatePane = useCallback(
    (pane: WorkspacePane, roomList = rooms, options: NavigatePaneOptions = {}) => {
      const nextPath = pathForPane(pane, roomList);
      const nextSearch = options.search || "";
      if (
        !nextPath ||
        (location.pathname === nextPath && (options.search === undefined || location.search === nextSearch))
      ) {
        return;
      }
      navigate({ pathname: nextPath, search: nextSearch }, { replace: Boolean(options.replace) });
    },
    [location.pathname, location.search, navigate, rooms],
  );

  const selectConversation = useCallback(
    (id: string, options: NavigatePaneOptions = {}) => {
      setActiveConversationId(id);
      const next: WorkspacePane = { type: WorkspacePaneTypes.conversation, id };
      navigatePane(next, options.rooms ?? rooms, options);
    },
    [navigatePane, rooms, setActiveConversationId],
  );

  const selectAgentApps = useCallback(
    (agentID: string) => {
      if (agentID) navigate(pathForAgentApps(agentID));
    },
    [navigate],
  );

  const selectAgent = useCallback(
    (item: { id?: string | null } | null | undefined, options: NavigatePaneOptions = {}) => {
      if (!item?.id) {
        return;
      }
      const next: WorkspacePane = { type: WorkspacePaneTypes.agent, id: item.id };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectTeam = useCallback(
    (item: { id?: string | null } | null | undefined, options: NavigatePaneOptions = {}) => {
      if (!item?.id) {
        return;
      }
      const next: WorkspacePane = { type: WorkspacePaneTypes.team, id: item.id };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectTeamSection = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.team, id: "" };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectNotificationSection = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.notifications, id: "" };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectModelProvider = useCallback(
    (item: { id?: string | null } | null | undefined, options: NavigatePaneOptions = {}) => {
      if (!item?.id) {
        return;
      }
      const next: WorkspacePane = { type: WorkspacePaneTypes.modelProvider, id: item.id };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectApps = useCallback(
    (addAppID?: string) =>
      navigate(addAppID ? `/connectors?add_connector=${encodeURIComponent(addAppID)}` : "/connectors"),
    [navigate],
  );

  const selectHuman = useCallback(
    (item: { id?: string | null } | null | undefined) => {
      if (!item?.id) {
        return;
      }
      navigate({ search: profileDialogSearch(location.search, item.id) }, { replace: true });
    },
    [location.search, navigate],
  );

  const selectComputer = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectHub = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  const selectSettings = useCallback(() => {
    navigate({ search: settingsDialogSearch(location.search) }, { replace: true });
  }, [location.search, navigate]);

  const selectTasks = useCallback(
    (taskID = "", options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.task, id: taskID };
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms],
  );

  useEffect(() => {
    const next = paneFromLocation(location.pathname);
    if (next.type === WorkspacePaneTypes.conversation) {
      setActiveConversationId(next.id || "");
    }
  }, [location.pathname, setActiveConversationId]);

  useEffect(() => {
    if (!dataReady) {
      return;
    }
    const locationPane = paneFromLocation(location.pathname);
    if (!locationPane.id) {
      return;
    }
    const nextPath = pathForPane(locationPane, rooms);
    if (nextPath && location.pathname !== nextPath) {
      navigate(nextPath, { replace: true });
    }
  }, [dataReady, location.pathname, navigate, rooms]);

  return {
    selectAgentApps,
    navigatePane,
    selectConversation,
    selectAgent,
    selectHuman,
    selectModelProvider,
    selectApps,
    selectTeam,
    selectTeamSection,
    selectNotificationSection,
    selectComputer,
    selectHub,
    selectSettings,
    selectTasks,
  };
}
