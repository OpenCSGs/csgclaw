import type { UseQueryResult } from "@tanstack/react-query";
import type { RefObject } from "react";
import type { Location, NavigateFunction } from "react-router-dom";
import type { FetchAgentsOptions } from "@/api/agents";
import type { FetchVersionOptions } from "@/api/app";
import type { MessageAction, MessageActionError, MessageLike } from "@/components/business/MessageContent/types";
import type { AgentLike, AgentProfileLike, RuntimeBootstrapConfig } from "@/models/agents";
import type { IMConversation, IMData, IMUser, LocaleCode, TranslateFn, UsersById } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { CollapsedWorkspaceGroups, WorkspacePane, WorkspaceTab } from "@/models/routing";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import type { ThemeMode } from "@/shared/theme/theme";
import type { CLIProxyAuthStatusMap } from "./useCLIProxyAuthStatuses";
import type { WorkspaceUiState } from "./workspaceUiStore";

export type WorkspaceQueryData<T> = T | ((current: T) => T);
export type WorkspaceQuerySetter<T> = (value: WorkspaceQueryData<T>) => void;

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
};

export type WorkspaceNavigationController = {
  navigatePane: (pane: WorkspacePane, roomList?: IMConversation[], options?: NavigatePaneOptions) => void;
  selectAgent: (item: { id?: string | null } | null | undefined, options?: NavigatePaneOptions) => void;
  selectComputer: (options?: NavigatePaneOptions) => void;
  selectConversation: (id: string, options?: NavigatePaneOptions) => void;
  selectHub: (options?: NavigatePaneOptions) => void;
  selectTeam: (item: { id?: string | null } | null | undefined, options?: NavigatePaneOptions) => void;
  selectTasks: (taskID?: string, options?: NavigatePaneOptions) => void;
};

export type UseWorkspaceShellControllerArgs = {
  activeConversationId: string;
  activePane: WorkspacePane;
  collapsedWorkspaceGroups: CollapsedWorkspaceGroups;
  isSidebarCollapsed: boolean;
  locale: LocaleCode;
  navigatePane: WorkspaceNavigationController["navigatePane"];
  rooms: IMConversation[];
  selectComputer: WorkspaceNavigationController["selectComputer"];
  selectConversation: WorkspaceNavigationController["selectConversation"];
  selectHub: WorkspaceNavigationController["selectHub"];
  selectTasks: WorkspaceNavigationController["selectTasks"];
  setCollapsedWorkspaceGroups: WorkspaceUiState["setCollapsedWorkspaceGroups"];
  setWorkspaceTab: WorkspaceUiState["setWorkspaceTab"];
  t: TranslateFn;
  theme: ThemeMode;
  workspaceTab: WorkspaceTab;
};

export type WorkspaceShellController = {
  currentWorkspaceLabel: string;
  selectWorkspaceTab: (tab: WorkspaceTab) => void;
  shellClassName: string;
  toggleWorkspaceGroup: (id: string) => void;
  workspaceTab: WorkspaceTab;
};

export type UseWorkspaceHubSelectionArgs = {
  loaded: boolean;
  manualError?: string;
  refreshTemplates?: () => Promise<unknown>;
  t: TranslateFn;
  templates: readonly HubTemplate[] | null | undefined;
  templatesQuery?: UseQueryResult<readonly HubTemplate[]>;
};

export type UseWorkspaceHubControllerArgs = {
  hubLoaded: boolean;
  hubTemplates: HubTemplate[];
  hubTemplatesQuery: UseQueryResult<HubTemplate[]>;
  refreshWorkspaceHubTemplates: () => Promise<HubTemplate[]>;
  t: TranslateFn;
};

export type UpgradeModalControllerProps = {
  appVersion: string;
  onApply: () => Promise<void>;
  onClose: () => void;
  t: TranslateFn;
  upgradeBusy: boolean;
  upgradeError: string;
  upgradePhase: UpgradePhase;
  upgradeStatus: UpgradeStatus | null;
};

export type UseUpgradeControllerArgs = {
  appVersion: string;
  refreshWorkspaceAppVersion: (options?: FetchVersionOptions) => Promise<string>;
  refreshWorkspaceUpgradeStatus: () => Promise<UpgradeStatus | null>;
  setAppVersionData: WorkspaceQuerySetter<string>;
  setUpgradeStatusData: WorkspaceQuerySetter<UpgradeStatus | null>;
  t: TranslateFn;
  upgradeStatus: UpgradeStatus | null;
};

export type UpgradeController = {
  handleUpgradeStatusChange: (payload: unknown) => void;
  openUpgradeModal: () => void;
  refreshUpgradeStatus: () => Promise<UpgradeStatus | null>;
  showUpgradeModal: boolean;
  upgradeBusy: boolean;
  upgradeError: string;
  upgradeModalProps: UpgradeModalControllerProps | null;
  upgradePhase: UpgradePhase;
};

export type ProfilePreviewAnchorRect = {
  bottom: number;
  left: number;
  right: number;
  top: number;
};

export type ProfilePreviewControllerProps = {
  agent: AgentLike | null;
  anchorRect: ProfilePreviewAnchorRect;
  busyKey: string;
  inDirectConversation: boolean;
  onClose: () => void;
  onDelete: (item: AgentLike) => Promise<void>;
  onOpenAgent: (item: AgentLike) => void;
  onOpenDM: (item: AgentLike) => Promise<void>;
  previewRef: RefObject<HTMLElement | null>;
  t: TranslateFn;
  user: IMUser | null;
};

export type UseProfilePreviewControllerArgs = {
  agentActionBusy: string;
  agentItems: AgentLike[];
  closeConversationTools: () => void;
  deletePreviewBot: (item: AgentLike | null | undefined) => Promise<boolean>;
  openAgentDirectMessage: (item: AgentLike | null | undefined) => Promise<void>;
  selectedConversation: IMConversation | null;
  selectAgent: WorkspaceNavigationController["selectAgent"];
  t: TranslateFn;
  usersById: UsersById;
};

export type ProfilePreviewController = {
  closeProfilePreview: () => void;
  openAgentPreview: (item: AgentLike | null | undefined, anchor: HTMLElement | null | undefined) => void;
  openParticipantPreview: (user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) => void;
  profilePreviewProps: ProfilePreviewControllerProps | null;
};

export type UseConversationControllerArgs = {
  activeConversationId: string;
  activePane: WorkspacePane;
  agents: AgentLike[];
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  data: IMData | null;
  locale: LocaleCode;
  managerProfile: AgentProfileLike | null;
  managerProfileIncomplete: boolean | null;
  messageActionBusy: string;
  messageActionError: MessageActionError;
  navigatePane: WorkspaceNavigationController["navigatePane"];
  onMessageAction: (
    action: MessageAction | null | undefined,
    message: MessageLike | null | undefined,
  ) => void | Promise<void>;
  onProviderLogin: (provider: string | null | undefined) => Promise<void>;
  onUpgradeStatusChange: (payload: unknown) => void;
  rooms: IMConversation[];
  selectComputer: WorkspaceNavigationController["selectComputer"];
  selectConversation: WorkspaceNavigationController["selectConversation"];
  setActiveConversationId: (id: string) => void;
  setBootstrapData: WorkspaceQuerySetter<IMData | null>;
  setShowToolCalls: WorkspaceUiState["setShowToolCalls"];
  showToolCalls: boolean;
  t: TranslateFn;
  theme: ThemeMode;
};

export type UseAgentControllerArgs = {
  activeConversationId: string;
  activePane: WorkspacePane;
  agents: AgentLike[];
  agentsLoaded: boolean;
  agentsQuery: UseQueryResult<AgentLike[]>;
  bootstrapConfig: RuntimeBootstrapConfig | null;
  data: IMData | null;
  hubTemplates: HubTemplate[];
  localRuntimeImages: string[];
  locale: LocaleCode;
  managerProfile: AgentProfileLike | null;
  refreshHubTemplates: () => Promise<void>;
  refreshWorkspaceAgents: (options?: FetchAgentsOptions) => Promise<AgentLike[]>;
  refreshWorkspaceBootstrap: () => Promise<IMData | null>;
  refreshWorkspaceBootstrapConfig: () => Promise<RuntimeBootstrapConfig | null>;
  refreshWorkspaceManagerProfile: () => Promise<AgentProfileLike | null>;
  rooms: IMConversation[];
  selectComputer: WorkspaceNavigationController["selectComputer"];
  selectConversation: WorkspaceNavigationController["selectConversation"];
  selectHub: WorkspaceNavigationController["selectHub"];
  setAgentsData: WorkspaceQuerySetter<AgentLike[]>;
  setManagerProfileData: WorkspaceQuerySetter<AgentProfileLike | null>;
  setSelectedHubTemplateId: WorkspaceUiState["setSelectedHubTemplateId"];
  t: TranslateFn;
};
