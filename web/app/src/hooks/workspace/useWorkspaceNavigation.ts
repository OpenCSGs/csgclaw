import { useCallback, useEffect } from "react";
import {
  DefaultWorkspacePaneIds,
  WorkspacePaneTypes,
  WorkspaceTabs,
  paneFromLocation,
  pathForPane,
  workspaceTabForPane,
} from "@/models/routing";
import type { NavigateFunction, Location } from "react-router-dom";
import type { Dispatch, SetStateAction } from "react";
import type { IMConversation } from "@/models/conversations";
import type { WorkspacePane, WorkspaceTab } from "@/models/routing";

export type NavigatePaneOptions = {
  replace?: boolean;
  rooms?: IMConversation[];
  updateURL?: boolean;
};

export type UseWorkspaceNavigationArgs = {
  activeConversationId: string;
  activePane: WorkspacePane;
  dataReady: boolean;
  location: Location;
  navigate: NavigateFunction;
  rooms: IMConversation[];
  setActiveConversationId: (id: string) => void;
  setActivePane: (pane: WorkspacePane) => void;
  setShowChannelTools: Dispatch<SetStateAction<boolean>>;
  setShowMemberList: Dispatch<SetStateAction<boolean>>;
  setWorkspaceTab: (tab: WorkspaceTab) => void;
};

export function useWorkspaceNavigation({
  location,
  navigate,
  dataReady,
  activePane,
  setActivePane,
  activeConversationId,
  setActiveConversationId,
  setWorkspaceTab,
  setShowMemberList,
  setShowChannelTools,
  rooms,
}: UseWorkspaceNavigationArgs) {
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
      setActivePane(next);
      setWorkspaceTab(WorkspaceTabs.messages);
      setShowMemberList(false);
      setShowChannelTools(false);
      if (options.updateURL !== false) {
        navigatePane(next, options.rooms ?? rooms, options);
      }
    },
    [
      navigatePane,
      rooms,
      setActiveConversationId,
      setActivePane,
      setShowChannelTools,
      setShowMemberList,
      setWorkspaceTab,
    ],
  );

  const selectAgent = useCallback(
    (item: { id?: string | null } | null | undefined, options: NavigatePaneOptions = {}) => {
      if (!item?.id) {
        return;
      }
      const next: WorkspacePane = { type: WorkspacePaneTypes.agent, id: item.id };
      setActivePane(next);
      setWorkspaceTab(WorkspaceTabs.agents);
      setShowMemberList(false);
      setShowChannelTools(false);
      if (options.updateURL !== false) {
        navigatePane(next, rooms, options);
      }
    },
    [navigatePane, rooms, setActivePane, setShowChannelTools, setShowMemberList, setWorkspaceTab],
  );

  const selectComputer = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
      setActivePane(next);
      setWorkspaceTab(WorkspaceTabs.agents);
      setShowMemberList(false);
      setShowChannelTools(false);
      if (options.updateURL !== false) {
        navigatePane(next, rooms, options);
      }
    },
    [navigatePane, rooms, setActivePane, setShowChannelTools, setShowMemberList, setWorkspaceTab],
  );

  const selectHub = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub };
      setActivePane(next);
      setWorkspaceTab(WorkspaceTabs.hub);
      setShowMemberList(false);
      setShowChannelTools(false);
      if (options.updateURL !== false) {
        navigatePane(next, rooms, options);
      }
    },
    [navigatePane, rooms, setActivePane, setShowChannelTools, setShowMemberList, setWorkspaceTab],
  );

  useEffect(() => {
    setWorkspaceTab(workspaceTabForPane(activePane));
  }, [activePane?.type, setWorkspaceTab]);

  useEffect(() => {
    const next = paneFromLocation(location.pathname);
    setActivePane(next);
    if (next.type === WorkspacePaneTypes.conversation) {
      setActiveConversationId(next.id);
    }
    setShowMemberList(false);
    setShowChannelTools(false);
  }, [location.pathname, setActiveConversationId, setActivePane, setShowChannelTools, setShowMemberList]);

  useEffect(() => {
    if (!dataReady || !activePane?.id) {
      return;
    }
    const nextPath = pathForPane(activePane, rooms);
    if (nextPath && location.pathname !== nextPath) {
      navigate(nextPath, { replace: true });
    }
  }, [activePane?.id, activePane?.type, dataReady, location.pathname, navigate, rooms]);

  return {
    navigatePane,
    selectConversation,
    selectAgent,
    selectComputer,
    selectHub,
  };
}
