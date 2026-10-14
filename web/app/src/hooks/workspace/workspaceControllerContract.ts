import type { ConversationPaneProps } from "@/components/business/ConversationPane";
import type { HubDetailPaneProps } from "@/components/business/HubDetailPane";
import type { TeamDetailPaneProps } from "@/components/business/TeamDetailPane";
import type { OpenCSGConnectionDialogProps } from "@/components/business/OpenCSGConnectionDialog";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMUser, LocaleCode, TranslateFn } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { ModelProviderCatalog } from "@/models/modelProviders";
import type { WorkspacePane } from "@/models/routing";
import type { WorkspaceSidebarProps } from "./contracts/sidebar";
import type { AgentDetailSidePanelProps } from "./types";
import type { useAgentController } from "./useAgentController";
import type { AuthController } from "./useAuthController";
import type { useConfigController } from "./useConfigController";
import type { useConversationController } from "./useConversationController";
import type { useProfilePreviewController } from "./useProfilePreviewController";
import type { useTaskController } from "./useTaskController";
import type { TaskEntryController } from "./useTaskEntry";
import type { useUpgradeController } from "./useUpgradeController";
import type { useWorkspaceData } from "./useWorkspaceData";
import type { ModelProviderCreationProps } from "./useModelProviderCreation";
import type { bindWorkspaceHub } from "./bindings/hub";

type AgentController = ReturnType<typeof useAgentController>;
type ConversationController = ReturnType<typeof useConversationController>;
type ConfigController = ReturnType<typeof useConfigController>;

export type WorkspaceControllerBase = {
  activePane: WorkspacePane;
  configSettings: ConfigController["settings"];
  loadingText: string;
  mainPanelHasThread: boolean;
  modelProviders: ModelProviderCatalog | null;
  modelProvidersLoaded: boolean;
  refreshWorkspaceModelProviders: ReturnType<typeof useWorkspaceData>["refreshWorkspaceModelProviders"];
  t: TranslateFn;
};

export type WorkspaceControllerReady = WorkspaceControllerBase & {
  ready: true;
  taskEntry: TaskEntryController;
  openAgentSettings: AgentController["openEditAgentModal"];
  openWorkspaceSettings: (section?: string) => void;
  shellClassName: string;
  sidebarProps: WorkspaceSidebarProps;
  authNotice: AuthController["notice"];
  onDismissAuthNotice: AuthController["dismissNotice"];
  openCSGLoginRequiredDialogProps: OpenCSGConnectionDialogProps | null;
  hubViewProps: Omit<HubDetailPaneProps, "hub"> & {
    t: TranslateFn;
    locale: LocaleCode;
    hub: ReturnType<typeof bindWorkspaceHub>;
    onSelectTemplateInPlace: (item: HubTemplate) => void;
  };
  agentViewProps: AgentController["agentViewProps"] & { activeRoom: IMConversation | null };
  computerViewProps: AgentController["computerViewProps"] & {
    channels: IMConversation[];
    directMessages: IMConversation[];
    onSelectAgent: (agent: AgentLike | null | undefined) => void;
  };
  humanViewProps: {
    t: TranslateFn;
    locale: LocaleCode;
    avatarBusy: boolean;
    avatarError: string;
    descriptionBusy: boolean;
    descriptionError: string;
    user: IMUser | null;
    onAvatarChange: (avatar: string) => Promise<void>;
    onDescriptionSave: (description: string) => Promise<void>;
  };
  conversationViewProps: ConversationController["conversationViewProps"] &
    Pick<ConversationPaneProps, "onNewTask" | "onOpenAgentDetail" | "onOpenThread" | "onPreviewUser"> & {
      agents: AgentLike[];
      agentDetailPanelProps: AgentDetailSidePanelProps | null;
    };
  taskViewProps: ReturnType<typeof useTaskController>["taskViewProps"] & { onOpenCreateTaskModal: () => void };
  teamViewProps: TeamDetailPaneProps;
  floatingChatProps: {
    avatar: string | null;
    avatarFallback: string;
    chatProps: ConversationPaneProps | null;
    locale: LocaleCode;
    online: boolean;
    open: boolean;
    t: TranslateFn;
    title: string;
    onOpenChange: (open: boolean) => void;
  };
  profilePreviewProps: ReturnType<typeof useProfilePreviewController>["profilePreviewProps"];
  createRoomModalProps: ConversationController["createRoomModalProps"];
  createTeamModalProps: AgentController["createTeamModalProps"];
  inviteMembersModalProps: ConversationController["inviteMembersModalProps"];
  upgradeModalProps: ReturnType<typeof useUpgradeController>["upgradeModalProps"];
  configModalProps: ConfigController["configModalProps"];
  createModelProviderModalProps: ModelProviderCreationProps | null;
  agentProfileModalProps: AgentController["agentProfileModalProps"];
};

export type WorkspaceControllerLoading = WorkspaceControllerBase & { ready: false } & Partial<
    Record<Exclude<keyof WorkspaceControllerReady, keyof WorkspaceControllerBase | "ready">, never>
  >;

export type WorkspaceController = WorkspaceControllerLoading | WorkspaceControllerReady;
