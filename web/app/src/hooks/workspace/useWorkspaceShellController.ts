import { useCallback, useEffect, useMemo, useState } from "react";
import { initializeMermaidTheme } from "@/components/business/MessageContent";
import { messages } from "@/shared/i18n/messages";
import {
  HUB_NEW_BADGE_SEEN_STORAGE_KEY,
  LOCALE_STORAGE_KEY,
  SIDEBAR_COLLAPSED_STORAGE_KEY,
  THEME_STORAGE_KEY,
  WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY,
} from "@/shared/storage/keys";
import { WorkspacePaneTypes, WorkspaceTabs, workspaceTabForPane } from "@/models/routing";
import type { WorkspaceTab } from "@/models/routing";
import type { UseWorkspaceShellControllerArgs, WorkspaceShellController } from "./types";

function readHubNewBadgeSeen(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return window.localStorage.getItem(HUB_NEW_BADGE_SEEN_STORAGE_KEY) === "seen";
  } catch {
    return false;
  }
}

function writeHubNewBadgeSeen() {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(HUB_NEW_BADGE_SEEN_STORAGE_KEY, "seen");
  } catch {
    // Local storage can be unavailable in restricted browser contexts.
  }
}

export function useWorkspaceShellController({
  activeConversationId,
  activePane,
  collapsedWorkspaceGroups,
  isSidebarCollapsed,
  locale,
  navigatePane,
  rooms,
  selectComputer,
  selectConversation,
  selectHub,
  selectTasks,
  setCollapsedWorkspaceGroups,
  setIsSidebarCollapsed,
  setWorkspaceTab,
  t,
  theme,
  workspaceTab,
}: UseWorkspaceShellControllerArgs): WorkspaceShellController {
  const [showHubNewBadge, setShowHubNewBadge] = useState(() => !readHubNewBadgeSeen());
  const currentWorkspaceLabel =
    activePane.type === WorkspacePaneTypes.agent
      ? t("agentOverview")
      : activePane.type === WorkspacePaneTypes.human
        ? t("humanDetailTitle")
        : activePane.type === WorkspacePaneTypes.team
          ? t("teamOverview")
          : activePane.type === WorkspacePaneTypes.computer
            ? t("computerOverview")
            : activePane.type === WorkspacePaneTypes.hub
              ? t("hubOverview")
              : activePane.type === WorkspacePaneTypes.task
                ? t("tasksOverview")
                : t("conversationOverview");
  const resolvedWorkspaceTab = useMemo(
    () => workspaceTab ?? workspaceTabForPane(activePane),
    [activePane, workspaceTab],
  );

  useEffect(() => {
    const routeTab = workspaceTabForPane(activePane);
    if (routeTab === WorkspaceTabs.messages) {
      if (workspaceTab !== WorkspaceTabs.messages && workspaceTab !== WorkspaceTabs.threads) {
        setWorkspaceTab(WorkspaceTabs.messages);
      }
      return;
    }
    if (workspaceTab !== routeTab) {
      setWorkspaceTab(routeTab);
    }
  }, [activePane, setWorkspaceTab, workspaceTab]);

  useEffect(() => {
    const messageLocale = locale === "zh" ? "zh" : "en";
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    document.title = messages[messageLocale].pageTitle;
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  }, [locale]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    initializeMermaidTheme(theme);
  }, [theme]);

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, String(isSidebarCollapsed));
  }, [isSidebarCollapsed]);

  useEffect(() => {
    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, JSON.stringify(collapsedWorkspaceGroups));
  }, [collapsedWorkspaceGroups]);

  const dismissHubNewBadge = useCallback(() => {
    setShowHubNewBadge(false);
    writeHubNewBadgeSeen();
  }, []);

  useEffect(() => {
    if (activePane.type !== WorkspacePaneTypes.hub || !showHubNewBadge) {
      return;
    }
    dismissHubNewBadge();
  }, [activePane.type, dismissHubNewBadge, showHubNewBadge]);

  function selectWorkspaceTab(tab: WorkspaceTab) {
    setIsSidebarCollapsed(false);
    if (tab === resolvedWorkspaceTab) {
      return;
    }
    setWorkspaceTab(tab);
    if (tab === WorkspaceTabs.hub) {
      dismissHubNewBadge();
      selectHub();
      return;
    }
    if (tab === WorkspaceTabs.agents) {
      selectComputer();
      return;
    }
    if (tab === WorkspaceTabs.tasks) {
      selectTasks();
      return;
    }
    const nextID = activeConversationId || rooms[0]?.id || "";
    if (nextID) {
      selectConversation(nextID);
      return;
    }
    navigatePane({ type: WorkspacePaneTypes.conversation, id: "" });
  }

  function toggleWorkspaceGroup(id: string) {
    setCollapsedWorkspaceGroups((current) => ({
      ...current,
      [id]: !current[id],
    }));
  }

  return {
    currentWorkspaceLabel,
    showHubNewBadge,
    shellClassName: `app-shell ${isSidebarCollapsed ? "sidebar-collapsed" : ""}`,
    workspaceTab: resolvedWorkspaceTab,
    selectWorkspaceTab,
    toggleWorkspaceGroup,
  };
}
