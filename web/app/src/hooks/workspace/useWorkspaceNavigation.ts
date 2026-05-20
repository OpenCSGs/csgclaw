import { useCallback, useEffect } from "react";
import { DefaultWorkspacePaneIds, WorkspacePaneTypes, paneFromLocation, pathForPane } from "@/models/routing";
import type { NavigateFunction, Location } from "react-router-dom";
import type { Dispatch, SetStateAction } from "react";
import type { IMConversation } from "@/models/conversations";
import type { WorkspacePane } from "@/models/routing";

export type NavigatePaneOptions = {
  replace?: boolean;
  rooms?: IMConversation[];
};

export type UseWorkspaceNavigationArgs = {
  dataReady: boolean;
  location: Location;
  navigate: NavigateFunction;
  rooms: IMConversation[];
  setActiveConversationId: (id: string) => void;
  setShowChannelTools: Dispatch<SetStateAction<boolean>>;
  setShowMemberList: Dispatch<SetStateAction<boolean>>;
};

export function useWorkspaceNavigation({
  location,
  navigate,
  dataReady,
  setActiveConversationId,
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
      setShowMemberList(false);
      setShowChannelTools(false);
      navigatePane(next, options.rooms ?? rooms, options);
    },
    [navigatePane, rooms, setActiveConversationId, setShowChannelTools, setShowMemberList],
  );

  const selectAgent = useCallback(
    (item: { id?: string | null } | null | undefined, options: NavigatePaneOptions = {}) => {
      if (!item?.id) {
        return;
      }
      const next: WorkspacePane = { type: WorkspacePaneTypes.agent, id: item.id };
      setShowMemberList(false);
      setShowChannelTools(false);
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms, setShowChannelTools, setShowMemberList],
  );

  const selectComputer = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
      setShowMemberList(false);
      setShowChannelTools(false);
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms, setShowChannelTools, setShowMemberList],
  );

  const selectHub = useCallback(
    (options: NavigatePaneOptions = {}) => {
      const next: WorkspacePane = { type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub };
      setShowMemberList(false);
      setShowChannelTools(false);
      navigatePane(next, rooms, options);
    },
    [navigatePane, rooms, setShowChannelTools, setShowMemberList],
  );

  useEffect(() => {
    const next = paneFromLocation(location.pathname);
    if (next.type === WorkspacePaneTypes.conversation) {
      setActiveConversationId(next.id || "");
    }
    setShowMemberList(false);
    setShowChannelTools(false);
  }, [location.pathname, setActiveConversationId, setShowChannelTools, setShowMemberList]);

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
