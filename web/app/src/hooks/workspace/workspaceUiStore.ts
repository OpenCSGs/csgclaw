import { create } from "zustand";
import { detectInitialLocale } from "@/shared/i18n";
import { detectInitialTheme } from "@/shared/theme/theme";
import { SIDEBAR_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";
import { WorkspacePaneTypes, paneFromLocation, readCollapsedWorkspaceGroups } from "@/models/routing";
import type { LocaleCode } from "@/models/conversations";
import type { CollapsedWorkspaceGroups } from "@/models/routing";
import type { ThemeMode } from "@/shared/theme/theme";

type MaybeUpdater<T> = T | ((current: T) => T);

export type WorkspaceUiState = {
  activeConversationId: string;
  collapsedWorkspaceGroups: CollapsedWorkspaceGroups;
  isSidebarCollapsed: boolean;
  locale: LocaleCode;
  selectedHubTemplateId: string;
  selectedHubWorkspacePath: string;
  showToolCalls: boolean;
  theme: ThemeMode;
  setActiveConversationId: (activeConversationId: string) => void;
  setCollapsedWorkspaceGroups: (value: MaybeUpdater<CollapsedWorkspaceGroups>) => void;
  setIsSidebarCollapsed: (value: MaybeUpdater<boolean>) => void;
  setLocale: (locale: LocaleCode) => void;
  setSelectedHubTemplateId: (value: MaybeUpdater<string>) => void;
  setSelectedHubWorkspacePath: (value: MaybeUpdater<string>) => void;
  setShowToolCalls: (value: MaybeUpdater<boolean>) => void;
  setTheme: (theme: ThemeMode) => void;
};

const initialPane = paneFromLocation();

export const useWorkspaceUiStore = create<WorkspaceUiState>((set) => ({
  locale: detectInitialLocale(),
  theme: detectInitialTheme(),
  showToolCalls: true,
  isSidebarCollapsed: window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === "true",
  collapsedWorkspaceGroups: readCollapsedWorkspaceGroups(),
  activeConversationId: initialPane.type === WorkspacePaneTypes.conversation ? initialPane.id : "",
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
  setCollapsedWorkspaceGroups: (value) =>
    set((state) => ({
      collapsedWorkspaceGroups: typeof value === "function" ? value(state.collapsedWorkspaceGroups) : value,
    })),
  setActiveConversationId: (activeConversationId) => set({ activeConversationId }),
  setSelectedHubTemplateId: (value) =>
    set((state) => ({
      selectedHubTemplateId: typeof value === "function" ? value(state.selectedHubTemplateId) : value,
    })),
  setSelectedHubWorkspacePath: (value) =>
    set((state) => ({
      selectedHubWorkspacePath: typeof value === "function" ? value(state.selectedHubWorkspacePath) : value,
    })),
}));
