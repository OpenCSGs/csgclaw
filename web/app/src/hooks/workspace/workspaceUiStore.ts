// @ts-nocheck
import { create } from "zustand";
import { detectInitialLocale } from "@/shared/i18n";
import { detectInitialTheme } from "@/shared/theme/theme";
import { SIDEBAR_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";
import { paneFromLocation, readCollapsedWorkspaceGroups, workspaceTabForPane } from "@/models/routing";

const initialPane = paneFromLocation();

export const useWorkspaceUiStore = create((set) => ({
  locale: detectInitialLocale(),
  theme: detectInitialTheme(),
  showToolCalls: true,
  isSidebarCollapsed: window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === "true",
  workspaceTab: workspaceTabForPane(initialPane),
  collapsedWorkspaceGroups: readCollapsedWorkspaceGroups(),
  activeConversationId: initialPane.type === "conversation" ? initialPane.id : "",
  activePane: initialPane,
  selectedHubTemplateId: "",
  selectedHubWorkspacePath: "",

  setLocale: (locale) => set({ locale }),
  setTheme: (theme) => set({ theme }),
  setShowToolCalls: (value) =>
    set((state) => ({
      showToolCalls: typeof value === "function" ? value(state.showToolCalls) : value,
    })),
  setIsSidebarCollapsed: (value) =>
    set((state) => ({
      isSidebarCollapsed: typeof value === "function" ? value(state.isSidebarCollapsed) : value,
    })),
  setWorkspaceTab: (workspaceTab) => set({ workspaceTab }),
  setCollapsedWorkspaceGroups: (value) =>
    set((state) => ({
      collapsedWorkspaceGroups: typeof value === "function" ? value(state.collapsedWorkspaceGroups) : value,
    })),
  setActiveConversationId: (activeConversationId) => set({ activeConversationId }),
  setActivePane: (activePane) => set({ activePane }),
  setSelectedHubTemplateId: (value) =>
    set((state) => ({
      selectedHubTemplateId: typeof value === "function" ? value(state.selectedHubTemplateId) : value,
    })),
  setSelectedHubWorkspacePath: (value) =>
    set((state) => ({
      selectedHubWorkspacePath: typeof value === "function" ? value(state.selectedHubWorkspacePath) : value,
    })),
}));
