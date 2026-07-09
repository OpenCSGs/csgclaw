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
import { resolveThemeMode } from "@/shared/theme/theme";
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

function currentWorkspaceLabelForPane(
  activePane: UseWorkspaceShellControllerArgs["activePane"],
  t: UseWorkspaceShellControllerArgs["t"],
) {
  switch (activePane.type) {
    case WorkspacePaneTypes.agent:
      return t("agentOverview");
    case WorkspacePaneTypes.notifications:
      return t("notificationsSection");
    case WorkspacePaneTypes.human:
      return t("humanDetailTitle");
    case WorkspacePaneTypes.team:
      return t("teamOverview");
    case WorkspacePaneTypes.computer:
      return t("computerOverview");
    case WorkspacePaneTypes.hub:
      return t("resourcesOverview");
    case WorkspacePaneTypes.task:
      return t("tasksOverview");
    case WorkspacePaneTypes.settings:
      return t("settings");
    default:
      return t("conversationOverview");
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
  setWorkspaceTab,
  t,
  theme,
  workspaceTab,
}: UseWorkspaceShellControllerArgs): WorkspaceShellController {
  const [showHubNewBadge, setShowHubNewBadge] = useState(() => !readHubNewBadgeSeen());
  const currentWorkspaceLabel = currentWorkspaceLabelForPane(activePane, t);
  const resolvedWorkspaceTab = useMemo(
    () => workspaceTab ?? workspaceTabForPane(activePane),
    [activePane, workspaceTab],
  );

  useEffect(() => {
    if (activePane.type === WorkspacePaneTypes.settings) {
      return;
    }
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
    function applyTheme() {
      const resolvedTheme = resolveThemeMode(theme);
      document.documentElement.dataset.theme = resolvedTheme;
      document.documentElement.style.colorScheme = resolvedTheme;
      initializeMermaidTheme(resolvedTheme);
    }

    applyTheme();
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);

    if (theme !== "system" || typeof window.matchMedia !== "function") {
      return undefined;
    }

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    mediaQuery.addEventListener("change", applyTheme);
    return () => mediaQuery.removeEventListener("change", applyTheme);
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
    if (activePane.type !== WorkspacePaneTypes.settings && tab === resolvedWorkspaceTab) {
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
    shellClassName: `app-shell ${isSidebarCollapsed ? "sidebar-collapsed" : ""} ${
      activePane.type === WorkspacePaneTypes.task || activePane.type === WorkspacePaneTypes.settings
        ? "sidebar-no-context"
        : "sidebar-has-context"
    }`,
    workspaceTab: resolvedWorkspaceTab,
    selectWorkspaceTab,
    toggleWorkspaceGroup,
  };
}
