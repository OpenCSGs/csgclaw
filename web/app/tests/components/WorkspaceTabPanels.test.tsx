import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi, type Mock } from "vitest";
import { WorkspaceTabPanels } from "@/pages/WorkspacePage/components/WorkspaceSidebar/WorkspaceTabPanels";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { WORKSPACE_SECTION_ORDER_STORAGE_KEY } from "@/shared/storage/keys";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceSidebarProps } from "@/pages/WorkspacePage/components/WorkspaceSidebar/types";

const labels: Record<string, string> = {
  agentsTab: "Agents",
  computerAgentsSection: "Agents",
  computersSection: "Computers",
  hubSkillsEmpty: "No skills",
  hubSkillsLabel: "Skills",
  hubTab: "Hub",
  hubTemplatesSection: "Templates",
  humanSection: "Human",
  localComputer: "Local computer",
  noAgents: "No workers yet.",
  noTeams: "No teams yet.",
  notificationsSection: "Notifications",
  profilePreview: "Profile preview",
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
  selectedHubSkillName: "demo-skill",
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
            ["u-admin", { id: "u-admin", name: "admin", handle: "admin", role: "admin", avatar: "A" }],
            ["manager", { id: "manager", name: "manager", handle: "manager", role: "agent" }],
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

    expect(
      within(panel)
        .getByText(/@admin/)
        .closest("button"),
    ).toHaveTextContent("@admin · admin");
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

    const humanRow = within(panel)
      .getByText(/@admin/)
      .closest("button");
    if (!humanRow) {
      throw new Error("Expected the human row to render");
    }

    fireEvent.click(humanRow);

    expect(onSelectHuman).toHaveBeenCalledWith(
      expect.objectContaining({ id: "u-admin", handle: "admin", role: "admin" }),
    );
  });

  it("marks the current human row active when the human pane is selected", () => {
    const { panel } = renderAgentsPanel({ activePane: { type: WorkspacePaneTypes.human, id: "u-admin" } });

    expect(
      within(panel)
        .getByText(/@admin/)
        .closest("button"),
    ).toHaveClass("active");
  });

  it("moves older default agent section order to the current Models and Human placement", () => {
    window.localStorage.setItem(
      WORKSPACE_SECTION_ORDER_STORAGE_KEY,
      JSON.stringify({ agents: ["agents", "teams", "computers", "notifications"] }),
    );

    const { container } = renderAgentsPanel();
    const groupLabels = Array.from(container.querySelectorAll<HTMLButtonElement>(".workspace-group-toggle")).map(
      (button) => button.textContent,
    );

    expect(groupLabels).toEqual(["Agents1", "modelsSection0", "Human1", "Computers1", "Notifications0", "Teams0"]);
  });

  it("renders hub templates and skills in separate workspace groups", () => {
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
    expect(screen.getByRole("button", { name: /Skills1/ })).toBeInTheDocument();
    expect(screen.getByText("demo-template")).toBeInTheDocument();
    expect(screen.getByText("demo-skill")).toBeInTheDocument();
  });
});
