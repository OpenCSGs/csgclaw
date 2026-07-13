import type { AgentLike } from "@/models/agents";
import type { CreateTeamPayload } from "@/api/tasks";
import type {
  IMConversation,
  IMMessage,
  IMUser,
  LocaleCode,
  ThreadView,
  TranslateFn,
  UsersById,
} from "@/models/conversations";
import type { AuthStatus } from "@/models/auth";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { MCPServer } from "@/models/mcp";
import type { ModelProvider, ModelProviderCatalog } from "@/models/modelProviders";
import type { SkillSummary } from "@/models/skillhub";
import type { CollapsedWorkspaceGroups, WorkspacePane, WorkspaceTab } from "@/models/routing";
import type { WorkspaceTask, WorkspaceTeam } from "@/models/tasks";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import type { ThemeMode } from "@/shared/theme/theme";
import type { WorkspaceHubController } from "@/hooks/workspace/useWorkspaceHubController";

type ValueOf<T> = T[keyof T];

export const WorkspaceContextSectionIds = {
  messages: "messages",
  rooms: "rooms",
  directMessages: "direct-messages",
  threads: "threads",
  agents: "agents",
  humans: "humans",
  computers: "computers",
  notifications: "notifications",
  teams: "teams",
  hubTemplates: "hub-templates",
  mcpServers: "mcp-servers",
  hubSkills: "hub-skills",
  models: "models",
  tasks: "tasks",
} as const;

export type WorkspaceContextSectionId = ValueOf<typeof WorkspaceContextSectionIds>;

export type WorkspaceSidebarProps = {
  activePane: WorkspacePane;
  activeThreadRootID: string;
  agentItems: AgentLike[];
  agentsError: string;
  appVersion: string;
  channels: IMConversation[];
  collapsedWorkspaceGroups: CollapsedWorkspaceGroups;
  currentUserID: string;
  currentWorkspaceLabel: string;
  authBusy: boolean;
  authError: string;
  authPending: boolean;
  authStatus: AuthStatus;
  directMessages: IMConversation[];
  showUpgradeControls: boolean;
  hub: WorkspaceHubController["hub"];
  isSidebarCollapsed: boolean;
  locale: LocaleCode;
  modelProviders?: ModelProviderCatalog | null;
  modelProvidersLoaded?: boolean;
  notificationAgentItems: AgentLike[];
  onCollapseSidebar: () => void;
  onCreateAgent: () => void | Promise<void>;
  onCreateModelProvider?: () => void | Promise<void>;
  onCreateTeam: (payload: CreateTeamPayload) => Promise<void>;
  onOpenCreateTeam: () => void | Promise<void>;
  onOpenCreateTask: () => void | Promise<void>;
  onOpenCreateScheduledTask?: () => void | Promise<void>;
  onCreateNotificationParticipant: () => void | Promise<void>;
  onCreateRoom: () => void;
  onExpandSidebar: () => void;
  onOpenUpgrade: () => void;
  onOpenConfigSettings: () => void;
  onOpenSettings: () => void;
  onLogin: (environment?: AuthEnvironmentDraft) => void | Promise<void>;
  onLogout: () => void | Promise<void>;
  onPreviewAgent: (item: AgentLike | null | undefined, anchor: HTMLElement | null | undefined) => void;
  onPreviewUser: (user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) => void;
  onSelectAgent: (item: AgentLike | null | undefined) => void;
  onSelectComputer: () => void;
  onSelectConversation: (id: string) => void;
  onSelectHuman: (user: IMUser | null | undefined) => void;
  onSelectHub: () => void;
  onSelectMCPServer?: (item: MCPServer | null | undefined) => void;
  onSelectHubSkill: (item: SkillSummary | null | undefined) => void;
  onSelectHubTemplate: (item: HubTemplate | null | undefined) => void;
  onSelectModelProvider?: (item: ModelProvider | null | undefined) => void;
  onSelectNotificationSection: () => void;
  onSelectTeam: (item: WorkspaceTeam | null | undefined) => void;
  onSelectTeamSection: () => void;
  onSelectTaskBoardView?: (view: "tasks" | "scheduled") => void;
  onSelectTask: (taskID?: string) => void;
  onSelectThread: (conversationID: string, message: IMMessage | null | undefined) => void | Promise<void>;
  onViewTaskDetails: (taskID?: string) => void;
  onThemeChange: (theme: ThemeMode) => void;
  onToggleWorkspaceGroup: (id: string) => void;
  onWorkspaceTabChange: (tab: WorkspaceTab) => void;
  onLocaleChange: (locale: LocaleCode) => void;
  taskCount: number;
  scheduledTaskCount?: number;
  activeTaskBoardView?: "tasks" | "scheduled";
  taskItems: WorkspaceTask[];
  planningTaskID?: string;
  startingTaskID?: string;
  teams: WorkspaceTeam[];
  roomCount: number;
  runningAgentCount: number;
  showHubNewBadge: boolean;
  t: TranslateFn;
  theme: ThemeMode;
  threadCount: number;
  threadGroups: { conversation: IMConversation; threads: ThreadView[] }[];
  upgradeBusy: boolean;
  upgradeError: string;
  upgradePhase: UpgradePhase;
  upgradeStatus: UpgradeStatus | null;
  suppressUpgradeIssue?: boolean;
  teamActionBusy: boolean;
  teamActionError: string;
  usersById: UsersById;
  workerAgentItems: AgentLike[];
  workspaceTab: WorkspaceTab;
};
