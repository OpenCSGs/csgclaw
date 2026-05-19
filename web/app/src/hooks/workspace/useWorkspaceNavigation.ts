// @ts-nocheck
import { useCallback, useEffect } from "react";
import { WORKSPACE_TAB_AGENTS, WORKSPACE_TAB_HUB, WORKSPACE_TAB_MESSAGES } from "@/bootstrap/constants";
import { paneFromLocation, pathForPane, workspaceTabForPane } from "@/models/routing";

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
}) {
  const navigatePane = useCallback(
    (pane, roomList = rooms, options = {}) => {
      const nextPath = pathForPane(pane, roomList);
      if (!nextPath || location.pathname === nextPath) {
        return;
      }
      navigate(nextPath, { replace: Boolean(options.replace) });
    },
    [location.pathname, navigate, rooms],
  );

  const selectConversation = useCallback(
    (id, options = {}) => {
      setActiveConversationId(id);
      const next = { type: "conversation", id };
      setActivePane(next);
      setWorkspaceTab(WORKSPACE_TAB_MESSAGES);
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
    (item, options = {}) => {
      if (!item?.id) {
        return;
      }
      const next = { type: "agent", id: item.id };
      setActivePane(next);
      setWorkspaceTab(WORKSPACE_TAB_AGENTS);
      setShowMemberList(false);
      setShowChannelTools(false);
      if (options.updateURL !== false) {
        navigatePane(next, rooms, options);
      }
    },
    [navigatePane, rooms, setActivePane, setShowChannelTools, setShowMemberList, setWorkspaceTab],
  );

  const selectComputer = useCallback(
    (options = {}) => {
      const next = { type: "computer", id: "local" };
      setActivePane(next);
      setWorkspaceTab(WORKSPACE_TAB_AGENTS);
      setShowMemberList(false);
      setShowChannelTools(false);
      if (options.updateURL !== false) {
        navigatePane(next, rooms, options);
      }
    },
    [navigatePane, rooms, setActivePane, setShowChannelTools, setShowMemberList, setWorkspaceTab],
  );

  const selectHub = useCallback(
    (options = {}) => {
      const next = { type: "hub", id: "hub" };
      setActivePane(next);
      setWorkspaceTab(WORKSPACE_TAB_HUB);
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
    if (next.type === "conversation") {
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
