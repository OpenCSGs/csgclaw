import type { AgentLike } from "@/models/agents";
import type {
  IMConversation,
  IMMessage,
  IMUser,
  LocaleCode,
  ThreadView,
  TranslateFn,
  UsersById,
} from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { CollapsedWorkspaceGroups, WorkspacePane, WorkspaceTab } from "@/models/routing";
import type { WorkspaceTask, WorkspaceTeam } from "@/models/tasks";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import type { ThemeMode } from "@/shared/theme/theme";
import type { WorkspaceHubController } from "@/hooks/workspace/useWorkspaceHubController";

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
  directMessages: IMConversation[];
  showUpgradeControls: boolean;
  hub: WorkspaceHubController["hub"];
  isSidebarCollapsed: boolean;
  locale: LocaleCode;
  notificationAgentItems: AgentLike[];
  onCollapseSidebar: () => void;
  onCreateAgent: () => void | Promise<void>;
  onCreateTeam: (payload: { title: string; lead_bot_id: string; member_bot_ids: string[] }) => Promise<void>;
  onOpenCreateTeam: () => void | Promise<void>;
  onOpenCreateTask: () => void | Promise<void>;
  onCreateNotificationBot: () => void | Promise<void>;
  onCreateRoom: () => void;
  onExpandSidebar: () => void;
  onOpenUpgrade: () => void;
  onPreviewAgent: (item: AgentLike | null | undefined, anchor: HTMLElement | null | undefined) => void;
  onPreviewUser: (user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) => void;
  onSelectAgent: (item: AgentLike | null | undefined) => void;
  onSelectComputer: () => void;
  onSelectConversation: (id: string) => void;
  onSelectHub: () => void;
  onSelectHubTemplate: (item: HubTemplate | null | undefined) => void;
  onSelectTeam: (item: WorkspaceTeam | null | undefined) => void;
  onSelectTask: (taskID?: string) => void;
  onSelectThread: (conversationID: string, message: IMMessage | null | undefined) => void | Promise<void>;
  onViewTaskDetails: (taskID?: string) => void;
  onThemeChange: (theme: ThemeMode) => void;
  onToggleWorkspaceGroup: (id: string) => void;
  onWorkspaceTabChange: (tab: WorkspaceTab) => void;
  onLocaleChange: (locale: LocaleCode) => void;
  taskCount: number;
  taskItems: WorkspaceTask[];
  planningTaskID?: string;
  startingTaskID?: string;
  teams: WorkspaceTeam[];
  roomCount: number;
  runningAgentCount: number;
  t: TranslateFn;
  theme: ThemeMode;
  threadCount: number;
  threadGroups: { conversation: IMConversation; threads: ThreadView[] }[];
  upgradeBusy: boolean;
  upgradeError: string;
  upgradePhase: UpgradePhase;
  upgradeStatus: UpgradeStatus | null;
  teamActionBusy: boolean;
  teamActionError: string;
  usersById: UsersById;
  workerAgentItems: AgentLike[];
  workspaceTab: WorkspaceTab;
};
