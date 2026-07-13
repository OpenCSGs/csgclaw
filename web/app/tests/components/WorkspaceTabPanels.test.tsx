import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi, type Mock } from "vitest";
import { WorkspaceTabPanels } from "@/pages/WorkspacePage/components/WorkspaceSidebar/WorkspaceTabPanels";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { WORKSPACE_SECTION_ORDER_STORAGE_KEY } from "@/shared/storage/keys";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTask } from "@/models/tasks";
import type { WorkspaceSidebarProps } from "@/pages/WorkspacePage/components/WorkspaceSidebar/types";

const labels: Record<string, string> = {
  agentsTab: "Agents",
  computerAgentsSection: "Agents",
  computersSection: "Computers",
  resourcesMCPAdd: "Add MCP",
  resourcesMCPEmpty: "No MCP servers",
  resourcesMCPLabel: "MCP",
  resourcesSkillsEmpty: "No skills",
  resourcesSkillsLabel: "Skills",
  resourcesModelProvidersSection: "Model Providers",
  resourcesSkillRemoteInstallFailed: "Install failed",
  resourcesSkillRemoteInstallAction: "Install",
  resourcesSkillRemoteInstallTab: "Community install",
  resourcesSkillRemoteInstalling: "Installing",
  resourcesSkillRemoteReplaceAction: "Replace",
  resourcesSkillRemoteSearchPlaceholder: "Search community skills",
  resourcesSkillRemoteSkillsEmpty: "No community skills",
  resourcesSkillRemoteSkillsLoading: "Loading community skills",
  resourcesSkillUpload: "Upload skill",
  resourcesSkillUploadDropHint: "Only zip",
  resourcesSkillUploadDropTitle: "Choose zip",
  resourcesSkillUploadSubmit: "Upload",
  resourcesSkillUploadSubtitle: "Upload or remote",
  resourcesSkillUploadZipTab: "Upload zip",
  resourcesTab: "Resources",
  resourcesTemplatesSection: "Templates",
  close: "Close",
  humanSection: "Human",
  localComputer: "Local computer",
  noAgents: "No workers yet.",
  noTeams: "No teams yet.",
  notificationsSection: "Notifications",
  profilePreview: "Profile preview",
  taskCreate: "New task",
  tasksTab: "Tasks",
  normalTasksTab: "Normal tasks",
  scheduledTaskCreate: "New scheduled task",
  scheduledTasksTab: "Scheduled",
  teamsSection: "Teams",
};

const t: TranslateFn = (key, params = {}) => {
  if (key === "teamMembersCount") {
    return `${params.count ?? 0} members`;
  }
  return labels[key] ?? key;
};

const managerAgent: AgentLike = {
  id: "manager",
  name: "manager",
  role: "manager",
  provider: "api",
  model_id: "gpt-5-high",
  available: true,
};

const hub: WorkspaceSidebarProps["hub"] = {
  listError: "",
  loaded: true,
  selectedHubTemplateId: "",
  selectedHubResourceType: "skill",
  selectedMCPServerName: "filesystem",
  selectedHubSkillName: "demo-skill",
  mcpServers: [
    {
      name: "filesystem",
      config: { command: "npx", args: ["-y", "@modelcontextprotocol/server-filesystem"] },
      description: "npx -y @modelcontextprotocol/server-filesystem",
    },
    {
      name: "github",
      config: { url: "https://github.example/mcp" },
      description: "https://github.example/mcp",
    },
  ],
  remoteSkills: [{ name: "agent-builder", description: "Build agents", remotePath: "AIWizards/agent-builder" }],
  remoteSkillsError: "",
  remoteSkillsLoading: false,
  skills: [{ name: "demo-skill", description: "Demo skill" }],
  templates: [{ id: "builtin/demo", name: "demo-template", description: "Demo template" }],
} as unknown as WorkspaceSidebarProps["hub"];

describe("WorkspaceTabPanels", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  function renderAgentsPanel(options: { activePane?: WorkspaceSidebarProps["activePane"]; onSelectHuman?: Mock } = {}) {
    const onSelectHuman = options.onSelectHuman ?? vi.fn();
    const { container } = render(
      <WorkspaceTabPanels
        activePane={options.activePane ?? { type: WorkspacePaneTypes.computer, id: "local" }}
        activeThreadRootID=""
        agentItems={[managerAgent]}
        agentsError=""
        channels={[]}
        collapsedWorkspaceGroups={{}}
        currentUserID="u-admin"
        directMessages={[]}
        hub={hub}
        locale="en"
        notificationAgentItems={[]}
        onCreateAgent={() => {}}
        onCreateNotificationParticipant={() => {}}
        onCreateRoom={() => {}}
        onOpenCreateTask={() => {}}
        onOpenCreateTeam={() => {}}
        onPreviewAgent={() => {}}
        onPreviewUser={() => {}}
        onSelectAgent={() => {}}
        onSelectComputer={() => {}}
        onSelectConversation={() => {}}
        onSelectHuman={onSelectHuman}
        onSelectHubSkill={() => {}}
        onSelectHubTemplate={() => {}}
        onSelectTask={() => {}}
        onSelectTeam={() => {}}
        onSelectThread={() => {}}
        onToggleWorkspaceGroup={() => {}}
        onViewTaskDetails={() => {}}
        t={t}
        taskCount={0}
        taskItems={[]}
        teams={[]}
        threadGroups={[]}
        usersById={
          new Map([
            ["u-admin", { id: "u-admin", name: "admin", role: "admin", avatar: "A" }],
            ["manager", { id: "manager", name: "manager", role: "agent" }],
          ])
        }
        workerAgentItems={[managerAgent]}
        workspaceTab={WorkspaceTabs.agents}
      />,
    );
    const panel = screen.getByRole("tabpanel", { name: "Agents" });
    return { container, onSelectHuman, panel };
  }

  it("renders the current human user between agents and computers", () => {
    const { container, panel } = renderAgentsPanel();

    const agentsGroup = container.querySelector<HTMLButtonElement>(
      'button[aria-controls="workspace-group-items-agents"]',
    );
    const humanGroup = container.querySelector<HTMLButtonElement>(
      'button[aria-controls="workspace-group-items-humans"]',
    );
    const computersGroup = container.querySelector<HTMLButtonElement>(
      'button[aria-controls="workspace-group-items-computers"]',
    );

    if (!agentsGroup || !humanGroup || !computersGroup) {
      throw new Error("Expected Agents, Human, and Computers group headers to render");
    }

    expect(within(panel).getByText("admin").closest("button")).toHaveTextContent("u-admin · admin");
    expect(agentsGroup).toHaveTextContent("Agents");
    expect(agentsGroup).toHaveTextContent("1");
    expect(humanGroup).toHaveTextContent("Human");
    expect(humanGroup).toHaveTextContent("1");
    expect(computersGroup).toHaveTextContent("Computers");
    expect(agentsGroup.compareDocumentPosition(humanGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(humanGroup.compareDocumentPosition(computersGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("opens the Human detail page from the human row", () => {
    const { onSelectHuman, panel } = renderAgentsPanel();

    const humanRow = within(panel).getByText("admin").closest("button");
    if (!humanRow) {
      throw new Error("Expected the human row to render");
    }

    fireEvent.click(humanRow);

    expect(onSelectHuman).toHaveBeenCalledWith(expect.objectContaining({ id: "u-admin", role: "admin" }));
  });

  it("marks the current human row active when the human pane is selected", () => {
    const { panel } = renderAgentsPanel({ activePane: { type: WorkspacePaneTypes.human, id: "u-admin" } });

    expect(within(panel).getByText("admin").closest("button")?.className).toContain("active");
  });

  it("moves older default agent section order to the current Models and Human placement", () => {
    window.localStorage.setItem(
      WORKSPACE_SECTION_ORDER_STORAGE_KEY,
      JSON.stringify({ agents: ["agents", "teams", "computers", "notifications"] }),
    );

    const { container } = renderAgentsPanel();
    const groupLabels = Array.from(
      container.querySelectorAll<HTMLButtonElement>('button[aria-controls^="workspace-group-items-"]'),
    ).map((button) => button.textContent);

    expect(groupLabels).toEqual(["Agents1", "Human1", "Computers1", "Notifications0", "Teams0"]);
  });

  it("renders hub templates, MCP servers, and skills in separate workspace groups", () => {
    const onSelectMCPServer = vi.fn();
    render(
      <WorkspaceTabPanels
        activePane={{ type: WorkspacePaneTypes.hub, id: "hub" }}
        activeThreadRootID=""
        agentItems={[managerAgent]}
        agentsError=""
        channels={[]}
        collapsedWorkspaceGroups={{}}
        currentUserID="u-admin"
        directMessages={[]}
        hub={hub}
        locale="en"
        notificationAgentItems={[]}
        onCreateAgent={() => {}}
        onCreateNotificationParticipant={() => {}}
        onCreateRoom={() => {}}
        onOpenCreateTask={() => {}}
        onOpenCreateTeam={() => {}}
        onPreviewAgent={() => {}}
        onPreviewUser={() => {}}
        onSelectAgent={() => {}}
        onSelectComputer={() => {}}
        onSelectConversation={() => {}}
        onSelectHuman={() => {}}
        onSelectMCPServer={onSelectMCPServer}
        onSelectHubSkill={() => {}}
        onSelectHubTemplate={() => {}}
        onSelectTask={() => {}}
        onSelectTeam={() => {}}
        onSelectThread={() => {}}
        onToggleWorkspaceGroup={() => {}}
        onViewTaskDetails={() => {}}
        t={t}
        taskCount={0}
        taskItems={[]}
        teams={[]}
        threadGroups={[]}
        usersById={new Map()}
        workerAgentItems={[managerAgent]}
        workspaceTab={WorkspaceTabs.hub}
      />,
    );

    expect(screen.getByRole("button", { name: /Templates1/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /MCP2/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Skills1/ })).toBeInTheDocument();
    expect(screen.getByText("demo-template")).toBeInTheDocument();
    expect(screen.getByText("filesystem")).toBeInTheDocument();
    expect(screen.getByText("github")).toBeInTheDocument();
    expect(screen.getByText("demo-skill")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /github/ }));

    expect(onSelectMCPServer).toHaveBeenCalledWith(expect.objectContaining({ name: "github" }));
  });

  it("shows Tasks sidebar subcategories while preserving counts", () => {
    const onOpenCreateScheduledTask = vi.fn();
    const taskItem = {
      id: "task-1",
      assignment_type: "team",
      assignment_id: "team-1",
      team_id: "team-1",
      team_title: "te-team",
      execution_channel: "csgclaw",
      room_id: "room-1",
      room_title: "Room 1",
      parent_id: "",
      title: "Build blog",
      body: "Create the blog site",
      status: "pending",
      created_by: "manager",
      created_by_agent_name: "manager",
      assigned_to: "",
      assigned_to_agent_name: "",
      claimed_by: "",
      claimed_by_agent_name: "",
      priority: 0,
      depends_on: [],
      plan_summary: "",
      dispatched_at: "",
      result: "",
      error: "",
      created_at: "2026-06-01T00:00:00Z",
      updated_at: "2026-06-01T00:00:00Z",
    } satisfies WorkspaceTask;

    render(
      <WorkspaceTabPanels
        activePane={{ type: WorkspacePaneTypes.task, id: "" }}
        activeThreadRootID=""
        agentItems={[managerAgent]}
        agentsError=""
        channels={[]}
        collapsedWorkspaceGroups={{}}
        currentUserID="u-admin"
        directMessages={[]}
        hub={hub}
        locale="en"
        notificationAgentItems={[]}
        onCreateAgent={() => {}}
        onCreateNotificationParticipant={() => {}}
        onCreateRoom={() => {}}
        onOpenCreateScheduledTask={onOpenCreateScheduledTask}
        onOpenCreateTask={() => {}}
        onOpenCreateTeam={() => {}}
        onPreviewAgent={() => {}}
        onPreviewUser={() => {}}
        onSelectAgent={() => {}}
        onSelectComputer={() => {}}
        onSelectConversation={() => {}}
        onSelectHuman={() => {}}
        onSelectHubSkill={() => {}}
        onSelectHubTemplate={() => {}}
        onSelectTask={() => {}}
        onSelectTeam={() => {}}
        onSelectThread={() => {}}
        onToggleWorkspaceGroup={() => {}}
        onViewTaskDetails={() => {}}
        t={t}
        activeTaskBoardView="scheduled"
        taskCount={1}
        scheduledTaskCount={2}
        taskItems={[taskItem]}
        teams={[]}
        threadGroups={[]}
        usersById={new Map()}
        workerAgentItems={[managerAgent]}
        workspaceTab={WorkspaceTabs.tasks}
      />,
    );

    const panel = screen.getByRole("tabpanel", { name: "Tasks" });
    expect(screen.getByRole("button", { name: /Tasks3/ })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: /Normal tasks1/ })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: /Scheduled2/ })).toBeInTheDocument();
    expect(within(panel).queryByText("Build blog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "New scheduled task" }));
    expect(onOpenCreateScheduledTask).toHaveBeenCalledTimes(1);
  });

  it("shows remote skills in the upload dialog remote mode", () => {
    const installRemoteSkill = vi.fn(async () => ({ name: "agent-builder" }));
    const loadMoreRemoteSkills = vi.fn(async () => undefined);
    const setRemoteSkillsEnabled = vi.fn();
    const setRemoteSkillsSearch = vi.fn();
    const remoteHub = {
      ...hub,
      installRemoteSkill,
      loadMoreRemoteSkills,
      remoteInstallBusy: "",
      remoteInstallError: "",
      remoteSkillsHasMore: true,
      remoteSkillsLoadingMore: false,
      remoteSkillsSearch: "",
      remoteSkills: [
        {
          name: "agent-builder",
          description: "Build agents",
          remotePath: "AIWizards/agent-builder",
          remoteURL: "https://opencsg-stg.example.test/skills/AIWizards/agent-builder",
        },
      ],
      skills: [{ name: "agent-builder", description: "Installed skill" }],
      setRemoteSkillsEnabled,
      setRemoteSkillsSearch,
    } as unknown as WorkspaceSidebarProps["hub"];

    render(
      <WorkspaceTabPanels
        activePane={{ type: WorkspacePaneTypes.hub, id: "hub" }}
        activeThreadRootID=""
        agentItems={[managerAgent]}
        agentsError=""
        channels={[]}
        collapsedWorkspaceGroups={{}}
        currentUserID="u-admin"
        directMessages={[]}
        hub={remoteHub}
        locale="en"
        notificationAgentItems={[]}
        onCreateAgent={() => {}}
        onCreateNotificationParticipant={() => {}}
        onCreateRoom={() => {}}
        onOpenCreateTask={() => {}}
        onOpenCreateTeam={() => {}}
        onPreviewAgent={() => {}}
        onPreviewUser={() => {}}
        onSelectAgent={() => {}}
        onSelectComputer={() => {}}
        onSelectConversation={() => {}}
        onSelectHuman={() => {}}
        onSelectHubSkill={() => {}}
        onSelectHubTemplate={() => {}}
        onSelectTask={() => {}}
        onSelectTeam={() => {}}
        onSelectThread={() => {}}
        onToggleWorkspaceGroup={() => {}}
        onViewTaskDetails={() => {}}
        t={t}
        taskCount={0}
        taskItems={[]}
        teams={[]}
        threadGroups={[]}
        usersById={new Map()}
        workerAgentItems={[managerAgent]}
        workspaceTab={WorkspaceTabs.hub}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Upload skill" }));
    expect(screen.getByRole("tab", { name: /Upload zip/ })).toHaveAttribute("aria-selected", "true");
    expect(setRemoteSkillsEnabled).not.toHaveBeenCalledWith(true);

    fireEvent.click(screen.getByRole("tab", { name: /Community install/ }));

    expect(setRemoteSkillsEnabled).toHaveBeenCalledWith(true);
    expect(screen.getAllByText("agent-builder").length).toBeGreaterThan(0);
    expect(screen.getByText("Build agents")).toBeInTheDocument();
    expect(screen.queryByText("official")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /agent-builder/ })).toHaveAttribute(
      "href",
      "https://opencsg-stg.example.test/skills/AIWizards/agent-builder",
    );
    fireEvent.change(screen.getByRole("searchbox", { name: "Search community skills" }), {
      target: { value: "sa" },
    });
    expect(setRemoteSkillsSearch).toHaveBeenCalledWith("sa");

    const remoteList = Array.from(document.querySelectorAll<HTMLDivElement>("[class]")).find((element) => {
      const className = element.getAttribute("class") || "";
      return className.includes("remoteList") && !className.includes("remoteListState");
    });
    if (!remoteList) {
      throw new Error("Expected the remote skill list to render");
    }
    Object.defineProperty(remoteList, "scrollHeight", { configurable: true, value: 1000 });
    Object.defineProperty(remoteList, "clientHeight", { configurable: true, value: 300 });
    Object.defineProperty(remoteList, "scrollTop", { configurable: true, value: 650 });
    fireEvent.scroll(remoteList);
    expect(loadMoreRemoteSkills).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Replace" }));
    expect(installRemoteSkill).toHaveBeenCalledWith(
      expect.objectContaining({ remotePath: "AIWizards/agent-builder" }),
      { replace: true },
    );
  });
});
