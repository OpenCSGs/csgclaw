import { fireEvent, render, screen } from "@testing-library/react";
import { emptyAuthStatus } from "@/models/auth";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { WorkspaceSidebar } from "@/pages/WorkspacePage/components/WorkspaceSidebar";
import type { WorkspaceSidebarProps } from "@/pages/WorkspacePage/components/WorkspaceSidebar/types";
import type { TranslateFn } from "@/models/conversations";

const labels: Record<string, string> = {
  activeNow: "active",
  agentsTab: "Agents",
  appearanceSettings: "Appearance",
  channelsSection: "Rooms",
  collapseSidebar: "Collapse sidebar",
  computerAgentsSection: "Agents",
  computerOverview: "Computer overview",
  computersSection: "Computers",
  configSettingsMenu: "Configuration",
  createRoom: "Create room",
  directMessagesSection: "Direct messages",
  expandSidebar: "Expand sidebar",
  humanSection: "Human",
  localAgentConsole: "Local agent console",
  messagesTab: "Messages",
  modelProviderAdd: "Add model provider",
  noNotificationBots: "No notification bots yet",
  noTeams: "No teams yet.",
  normalTasksTab: "Tasks",
  notificationsSection: "Notifications",
  resourcesModelProvidersSection: "Model Providers",
  resourcesSkillsLabel: "Skills",
  resourcesMCPLabel: "MCP",
  resourcesTab: "Resources",
  resourcesTemplatesSection: "Templates",
  scheduledTasksTab: "Scheduled",
  scheduledTaskCreate: "New scheduled task",
  settings: "Settings",
  tasksTab: "Tasks",
  teamsSection: "Teams",
  themeDark: "Dark",
  themeLight: "Light",
  workspaceSearchNoResults: "No matching results.",
  workspaceSearchPlaceholder: "Search",
};

const t: TranslateFn = (key, params) => {
  const label = labels[key] ?? key;
  if (!params) {
    return label;
  }
  return Object.entries(params).reduce(
    (current, [paramKey, value]) => current.replace(`{${paramKey}}`, String(value)),
    label,
  );
};

function renderSidebar(overrides: Partial<WorkspaceSidebarProps> = {}) {
  const props: WorkspaceSidebarProps = {
    activePane: { type: WorkspacePaneTypes.hub, id: "hub", resourceType: "template" },
    activeThreadRootID: "",
    activeTaskBoardView: "tasks",
    agentItems: [],
    agentsError: "",
    appVersion: "0.0.0",
    authBusy: false,
    authError: "",
    authPending: false,
    authStatus: emptyAuthStatus(),
    channels: [],
    collapsedWorkspaceGroups: {},
    currentUserID: "user-1",
    currentWorkspaceLabel: "Resources",
    directMessages: [],
    hub: {
      loaded: true,
      listError: "",
      selectedHubResourceType: "template",
      selectedHubSkillName: "",
      selectedHubTemplateId: "",
      skills: [{ name: "shell", description: "Shell helpers" }],
      skillsError: "",
      templates: [{ id: "worker-template", name: "Worker Template", role: "worker" }],
      uploadBusy: false,
      uploadError: "",
    } as unknown as WorkspaceSidebarProps["hub"],
    isSidebarCollapsed: false,
    locale: "en",
    modelProviders: {
      builtinProviders: [],
      customProviders: [],
      providers: [
        {
          builtin: true,
          display_name: "OpenAI",
          id: "openai",
          kind: "openai",
          models: ["gpt-4.1"],
          preset: "openai",
          status: "connected",
        },
      ],
    },
    modelProvidersLoaded: true,
    notificationAgentItems: [],
    onCollapseSidebar: vi.fn(),
    onCreateAgent: vi.fn(),
    onCreateModelProvider: vi.fn(),
    onCreateNotificationParticipant: vi.fn(),
    onCreateRoom: vi.fn(),
    onCreateTeam: vi.fn(),
    onExpandSidebar: vi.fn(),
    onLocaleChange: vi.fn(),
    onLogin: vi.fn(),
    onLogout: vi.fn(),
    onOpenConfigSettings: vi.fn(),
    onOpenSettings: vi.fn(),
    onOpenCreateScheduledTask: vi.fn(),
    onOpenCreateTask: vi.fn(),
    onOpenCreateTeam: vi.fn(),
    onOpenUpgrade: vi.fn(),
    onPreviewAgent: vi.fn(),
    onPreviewUser: vi.fn(),
    onSelectAgent: vi.fn(),
    onSelectComputer: vi.fn(),
    onSelectConversation: vi.fn(),
    onSelectHub: vi.fn(),
    onSelectHubSkill: vi.fn(),
    onSelectHubTemplate: vi.fn(),
    onSelectModelProvider: vi.fn(),
    onSelectHuman: vi.fn(),
    onSelectNotificationSection: vi.fn(),
    onSelectTask: vi.fn(),
    onSelectTaskBoardView: vi.fn(),
    onSelectTeam: vi.fn(),
    onSelectTeamSection: vi.fn(),
    onSelectThread: vi.fn(),
    onThemeChange: vi.fn(),
    onToggleWorkspaceGroup: vi.fn(),
    onViewTaskDetails: vi.fn(),
    onWorkspaceTabChange: vi.fn(),
    planningTaskID: "",
    roomCount: 0,
    runningAgentCount: 0,
    scheduledTaskCount: 0,
    showHubNewBadge: false,
    showUpgradeControls: false,
    suppressUpgradeIssue: false,
    taskCount: 0,
    taskItems: [],
    teamActionBusy: false,
    teamActionError: "",
    teams: [],
    t,
    theme: "light",
    threadCount: 0,
    threadGroups: [],
    upgradeBusy: false,
    upgradeError: "",
    upgradePhase: "idle",
    upgradeStatus: null,
    usersById: new Map([["user-1", { id: "user-1", name: "User One" }]]),
    workerAgentItems: [],
    workspaceTab: WorkspaceTabs.hub,
    ...overrides,
  };

  return {
    props,
    ...render(<WorkspaceSidebar {...props} />),
  };
}

describe("WorkspaceSidebar", () => {
  it("keeps room creation only in the Rooms section", () => {
    const onCreateRoom = vi.fn();

    renderSidebar({
      activePane: { type: WorkspacePaneTypes.conversation, id: "" },
      currentWorkspaceLabel: "Messages",
      onCreateRoom,
      workspaceTab: WorkspaceTabs.messages,
    });

    const createRoomButtons = screen.getAllByRole("button", { name: "Create room" });
    expect(createRoomButtons).toHaveLength(1);
    expect(createRoomButtons[0].parentElement).toHaveAttribute("data-tooltip", "Create room");

    fireEvent.click(createRoomButtons[0]);
    expect(onCreateRoom).toHaveBeenCalledTimes(1);
  });

  it("selects the MCP resource type when the MCP list is empty", () => {
    const onSelectMCPServer = vi.fn();

    renderSidebar({
      hub: {
        loaded: true,
        listError: "",
        mcpServers: [],
        selectedHubResourceType: "skill",
        selectedHubSkillName: "shell",
        selectedHubTemplateId: "",
        skills: [{ name: "shell", description: "Shell helpers" }],
        skillsError: "",
        templates: [],
        uploadBusy: false,
        uploadError: "",
      } as unknown as WorkspaceSidebarProps["hub"],
      onSelectMCPServer,
    });

    fireEvent.click(screen.getByRole("button", { name: "MCP" }));

    expect(onSelectMCPServer).toHaveBeenCalledWith(null);
  });

  it("uses primary resource navigation to show the selected resource list", () => {
    const onToggleWorkspaceGroup = vi.fn();
    const onSelectModelProvider = vi.fn();

    const { props } = renderSidebar({
      collapsedWorkspaceGroups: { models: true },
      onSelectModelProvider,
      onToggleWorkspaceGroup,
    });

    expect(document.querySelector('[data-workspace-section="hub-templates"]')).toBeInTheDocument();
    expect(document.querySelector('[data-workspace-section="models"]')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Model Providers" }));

    expect(onSelectModelProvider).toHaveBeenCalledWith(props.modelProviders?.providers[0]);
    expect(document.querySelector('[data-workspace-section="models"]')).toBeInTheDocument();
    expect(onToggleWorkspaceGroup).toHaveBeenCalledWith("models");
  });

  it("shows counts for every resource navigation item", () => {
    renderSidebar({
      hub: {
        loaded: true,
        listError: "",
        selectedHubResourceType: "template",
        selectedHubSkillName: "",
        selectedHubTemplateId: "",
        skills: [
          { name: "shell", description: "Shell helpers" },
          { name: "review", description: "Review helpers" },
          { name: "ship", description: "Release helpers" },
        ],
        skillsError: "",
        templates: [
          { id: "worker-template", name: "Worker Template", role: "worker" },
          { id: "manager-template", name: "Manager Template", role: "manager" },
        ],
        uploadBusy: false,
        uploadError: "",
      } as unknown as WorkspaceSidebarProps["hub"],
      modelProviders: {
        builtinProviders: [],
        customProviders: [],
        providers: [
          {
            builtin: true,
            display_name: "OpenAI",
            id: "openai",
            kind: "openai",
            models: ["gpt-4.1"],
            preset: "openai",
            status: "connected",
          },
          {
            builtin: true,
            display_name: "Claude Code",
            id: "claude-code",
            kind: "anthropic",
            models: ["claude-sonnet-4"],
            preset: "custom",
            status: "connected",
          },
          {
            builtin: false,
            display_name: "Local",
            id: "local",
            kind: "openai-compatible",
            models: ["local-model"],
            preset: "custom",
            status: "connected",
          },
          {
            builtin: false,
            display_name: "Test",
            id: "test",
            kind: "openai-compatible",
            models: [],
            preset: "custom",
            status: "disconnected",
          },
        ],
      },
    });

    expect(screen.getByRole("button", { name: "Templates" })).toHaveTextContent("2");
    expect(screen.getByRole("button", { name: "Skills" })).toHaveTextContent("3");
    expect(screen.getByRole("button", { name: "Model Providers" })).toHaveTextContent("4");
  });

  it("keeps the contextual sidebar visible when the primary rail is collapsed", () => {
    renderSidebar({ isSidebarCollapsed: true });

    const searchInput = screen.getByRole("searchbox", { name: "Search" });
    const contextAside = searchInput.closest("aside");

    expect(searchInput).toBeInTheDocument();
    expect(contextAside).not.toHaveAttribute("aria-hidden");
    expect(contextAside).not.toHaveAttribute("inert");
  });

  it("opens the scheduled task view from the task navigation", () => {
    const onSelectTask = vi.fn();
    const onSelectTaskBoardView = vi.fn();

    renderSidebar({
      activePane: { type: WorkspacePaneTypes.task, id: "" },
      activeTaskBoardView: "scheduled",
      currentWorkspaceLabel: "Task overview",
      onSelectTask,
      onSelectTaskBoardView,
      scheduledTaskCount: 2,
      workspaceTab: WorkspaceTabs.tasks,
    });

    fireEvent.click(screen.getByRole("button", { name: "Scheduled" }));

    expect(onSelectTaskBoardView).toHaveBeenCalledWith("scheduled");
    expect(onSelectTask).toHaveBeenCalledTimes(1);
  });

  it("renders the selected Human section as entity rows without a nested Human group", () => {
    renderSidebar({
      activePane: { type: WorkspacePaneTypes.human, id: "user-1" },
      currentWorkspaceLabel: "Human",
      workspaceTab: WorkspaceTabs.agents,
    });

    expect(document.querySelector('[data-workspace-section="humans"]')).toBeInTheDocument();
    expect(document.querySelector('button[aria-controls="workspace-group-items-humans"]')).not.toBeInTheDocument();
    expect(screen.getByText("User One")).toBeInTheDocument();
  });

  it("opens the notification section when there are no notification bots", () => {
    const onSelectAgent = vi.fn();
    const onSelectNotificationSection = vi.fn();

    renderSidebar({
      onSelectAgent,
      onSelectNotificationSection,
      workspaceTab: WorkspaceTabs.agents,
    });

    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));

    expect(onSelectNotificationSection).toHaveBeenCalledTimes(1);
    expect(onSelectAgent).not.toHaveBeenCalled();
  });

  it("opens the team section when there are no teams", () => {
    const onSelectTeam = vi.fn();
    const onSelectTeamSection = vi.fn();

    renderSidebar({
      onSelectTeam,
      onSelectTeamSection,
      workspaceTab: WorkspaceTabs.agents,
    });

    fireEvent.click(screen.getByRole("button", { name: "Teams" }));

    expect(onSelectTeamSection).toHaveBeenCalledTimes(1);
    expect(onSelectTeam).not.toHaveBeenCalled();
  });

  it("opens the settings page from the bottom settings entry without changing workspace tabs", () => {
    const onOpenSettings = vi.fn();
    const onWorkspaceTabChange = vi.fn();

    renderSidebar({ onOpenSettings, onWorkspaceTabChange });

    fireEvent.click(screen.getByRole("button", { name: "Settings" }));

    expect(onOpenSettings).toHaveBeenCalledTimes(1);
    expect(onWorkspaceTabChange).not.toHaveBeenCalled();
  });
});
