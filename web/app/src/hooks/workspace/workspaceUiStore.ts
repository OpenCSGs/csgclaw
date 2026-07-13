import { create } from "zustand";
import { detectInitialLocale } from "@/shared/i18n";
import { detectInitialTheme } from "@/shared/theme/theme";
import { SIDEBAR_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";
import {
  WorkspacePaneTypes,
  paneFromLocation,
  readCollapsedWorkspaceGroups,
  workspaceTabForPane,
} from "@/models/routing";
import type { LocaleCode } from "@/models/conversations";
import type { CollapsedWorkspaceGroups, WorkspaceTab } from "@/models/routing";
import type { ThemeMode } from "@/shared/theme/theme";

type MaybeUpdater<T> = T | ((current: T) => T);

export const HubResourceTypes = {
  mcp: "mcp",
  skill: "skill",
  template: "template",
} as const;

export type HubResourceType = (typeof HubResourceTypes)[keyof typeof HubResourceTypes];

export type WorkspaceUiState = {
  activeConversationId: string;
  collapsedWorkspaceGroups: CollapsedWorkspaceGroups;
  floatingChatOpen: boolean;
  isSidebarCollapsed: boolean;
  locale: LocaleCode;
  selectedHubResourceType: HubResourceType;
  selectedMCPServerName: string;
  selectedHubSkillName: string;
  selectedHubSkillPath: string;
  selectedHubTemplateId: string;
  selectedHubWorkspacePath: string;
  showToolCalls: boolean;
  theme: ThemeMode;
  workspaceTab: WorkspaceTab;
  setActiveConversationId: (activeConversationId: string) => void;
  setCollapsedWorkspaceGroups: (value: MaybeUpdater<CollapsedWorkspaceGroups>) => void;
  setFloatingChatOpen: (value: MaybeUpdater<boolean>) => void;
  setIsSidebarCollapsed: (value: MaybeUpdater<boolean>) => void;
  setLocale: (locale: LocaleCode) => void;
  setSelectedHubResourceType: (value: MaybeUpdater<HubResourceType>) => void;
  setSelectedMCPServerName: (value: MaybeUpdater<string>) => void;
  setSelectedHubSkillName: (value: MaybeUpdater<string>) => void;
  setSelectedHubSkillPath: (value: MaybeUpdater<string>) => void;
  setSelectedHubTemplateId: (value: MaybeUpdater<string>) => void;
  setSelectedHubWorkspacePath: (value: MaybeUpdater<string>) => void;
  setShowToolCalls: (value: MaybeUpdater<boolean>) => void;
  setTheme: (theme: ThemeMode) => void;
  setWorkspaceTab: (workspaceTab: WorkspaceTab) => void;
};

const initialPane = paneFromLocation();

export const useWorkspaceUiStore = create<WorkspaceUiState>((set) => ({
  locale: detectInitialLocale(),
  theme: detectInitialTheme(),
  showToolCalls: false,
  isSidebarCollapsed: window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === "true",
  collapsedWorkspaceGroups: readCollapsedWorkspaceGroups(),
  activeConversationId: initialPane.type === WorkspacePaneTypes.conversation ? String(initialPane.id ?? "") : "",
  floatingChatOpen: false,
  workspaceTab: workspaceTabForPane(initialPane),
  selectedHubResourceType: HubResourceTypes.template,
  selectedMCPServerName: "",
  selectedHubSkillName: "",
  selectedHubSkillPath: "",
  selectedHubTemplateId: "",
  selectedHubWorkspacePath: "",

  setLocale: (locale) => set({ locale }),
  setTheme: (theme) => set({ theme }),
  setFloatingChatOpen: (value) =>
    set((state) => ({
      floatingChatOpen: typeof value === "function" ? value(state.floatingChatOpen) : value,
    })),
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
  setWorkspaceTab: (workspaceTab) => set({ workspaceTab }),
  setSelectedHubResourceType: (value) =>
    set((state) => ({
      selectedHubResourceType: typeof value === "function" ? value(state.selectedHubResourceType) : value,
    })),
  setSelectedMCPServerName: (value) =>
    set((state) => ({
      selectedMCPServerName: typeof value === "function" ? value(state.selectedMCPServerName) : value,
    })),
  setSelectedHubSkillName: (value) =>
    set((state) => ({
      selectedHubSkillName: typeof value === "function" ? value(state.selectedHubSkillName) : value,
    })),
  setSelectedHubSkillPath: (value) =>
    set((state) => ({
      selectedHubSkillPath: typeof value === "function" ? value(state.selectedHubSkillPath) : value,
    })),
  setSelectedHubTemplateId: (value) =>
    set((state) => ({
      selectedHubTemplateId: typeof value === "function" ? value(state.selectedHubTemplateId) : value,
    })),
  setSelectedHubWorkspacePath: (value) =>
    set((state) => ({
      selectedHubWorkspacePath: typeof value === "function" ? value(state.selectedHubWorkspacePath) : value,
    })),
}));
