import { useCallback, useEffect } from "react";
import { DefaultWorkspacePaneIds, WorkspacePaneTypes, paneFromLocation, pathForPane } from "@/models/routing";
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
      if (!nextPath || location.pathname === nextPath) {
        return;
      }
      navigate(nextPath, { replace: Boolean(options.replace) });
    },
    [location.pathname, navigate, rooms],
  );

  const selectConversation = useCallback(
    (id: string, options: NavigatePaneOptions = {}) => {
      setActiveConversationId(id);
      const next: WorkspacePane = { type: WorkspacePaneTypes.conversation, id };
      navigatePane(next, options.rooms ?? rooms, options);
    },
    [navigatePane, rooms, setActiveConversationId],
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
    navigatePane,
    selectConversation,
    selectAgent,
    selectComputer,
    selectHub,
  };
}
