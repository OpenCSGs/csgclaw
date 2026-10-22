import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import type { UseQueryResult } from "@tanstack/react-query";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { useWorkspaceHubTemplatesQuery } from "@/hooks/workspace/workspaceQueries";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { AgentLike } from "@/models/agents";
import type { WorkspaceTeam } from "@/models/tasks";
import { AgentOverviewPage } from "@/pages/AgentOverviewPage/AgentOverviewPage";
import { createTranslator } from "@/shared/i18n";
import { paneFromLocation } from "@/models/routing";

vi.mock("@/hooks/workspace/workspaceQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/workspace/workspaceQueries")>()),
  useWorkspaceHubTemplatesQuery: vi.fn(),
}));

vi.mock("@/components/business/HubDetailPane", () => ({
  HubDetailPane: ({
    hub,
    inlineTemplateDetail,
  }: {
    hub: { detailPaneProps: { selectedTemplateId: string } };
    inlineTemplateDetail?: boolean;
  }) => (
    <section aria-label="Template detail" data-inline={inlineTemplateDetail}>
      {hub.detailPaneProps.selectedTemplateId}
    </section>
  ),
}));

const template: HubTemplate = {
  id: "builtin.codex-worker",
  name: "Codex worker",
  role: "worker",
  source: { name: "builtin", kind: "builtin" },
};

function queryState(patch: Partial<UseQueryResult<HubTemplate[]>> = {}): UseQueryResult<HubTemplate[]> {
  return {
    data: undefined,
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    ...patch,
  } as UseQueryResult<HubTemplate[]>;
}

function NavigationState() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <output aria-label="Current URL">
        {location.pathname}
        {location.search}
      </output>
      <button onClick={() => navigate(-1)}>Back</button>
      <button onClick={() => navigate(1)}>Forward</button>
    </>
  );
}

function Page({
  route = "/agents?tab=templates",
  username = "",
  agents = [],
  teams = [],
}: {
  route?: string;
  username?: string;
  agents?: AgentLike[];
  teams?: WorkspaceTeam[];
}) {
  const [selectedTemplateId, setSelectedTemplateId] = useState("");
  const controller = {
    ready: true,
    activePane: { type: "agents", id: "" },
    sidebarProps: {
      t: createTranslator("zh"),
      authStatus: {
        authenticated: Boolean(username),
        user_id: username,
        opencsg_base_url: "https://qa.opencsg.com/account",
      },
      workerAgentItems: agents,
      teams,
    },
    agentViewProps: { item: null },
    hubViewProps: {
      hub: { detailPaneProps: { selectedTemplateId, selectedResourceType: "template" } },
      onSelectTemplateInPlace: (template: HubTemplate) => setSelectedTemplateId(template.id || ""),
    },
  } as unknown as WorkspaceController;
  return (
    <MemoryRouter initialEntries={[route]}>
      <WorkspaceControllerProvider controller={controller}>
        <AgentOverviewPage />
        <NavigationState />
      </WorkspaceControllerProvider>
    </MemoryRouter>
  );
}

function TeamNavigationContents() {
  const location = useLocation();
  const navigate = useNavigate();
  const activePane = paneFromLocation(location.pathname);
  const t = createTranslator("zh");
  const agents = [{ id: "agent-worker", name: "测试智能体", role: "worker" }];
  const teams = ["1", "2"].map((id) => ({
    id: `team-${id}`,
    title: `回归团队 ${id}`,
    lead_agent_id: "agent-worker",
    member_agent_ids: ["agent-worker"],
    status: "active",
    created_at: "",
    updated_at: "",
  }));
  const controller = {
    ready: true,
    activePane,
    sidebarProps: { t, workerAgentItems: agents, teams },
    agentViewProps: { item: null },
    teamViewProps: { t, agents, team: teams.find((team) => team.id === activePane.id) },
  } as unknown as WorkspaceController;
  return (
    <WorkspaceControllerProvider controller={controller}>
      <AgentOverviewPage />
      <button onClick={() => navigate("/teams/team-1")}>Go to referenced team</button>
    </WorkspaceControllerProvider>
  );
}

describe("agent overview team navigation", () => {
  beforeEach(() => vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState()));

  it.each(["/agents", "/teams/team-2"])(
    "switches the list to teams and clears the agent search when navigating from %s",
    async (route) => {
      render(
        <MemoryRouter initialEntries={[route]}>
          <TeamNavigationContents />
        </MemoryRouter>,
      );
      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "我的智能体 1" }));
      await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), "测试智能体");
      await user.click(screen.getByRole("button", { name: "Go to referenced team" }));
      expect(await screen.findByRole("heading", { name: "回归团队 1", level: 1 })).toBeVisible();
      expect(screen.getByRole("button", { name: "团队 2" })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("button", { name: "我的智能体 1" })).toHaveAttribute("aria-pressed", "false");
      expect(screen.getByRole("textbox", { name: "搜索智能体和团队" })).toHaveValue("");
      expect(screen.getByRole("button", { name: /回归团队 1/ })).toBeVisible();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    },
  );

  it("selects the team filter when opening a team directly", async () => {
    render(
      <MemoryRouter initialEntries={["/teams/team-1"]}>
        <TeamNavigationContents />
      </MemoryRouter>,
    );
    expect(await screen.findByRole("heading", { name: "回归团队 1", level: 1 })).toBeVisible();
    expect(screen.getByRole("button", { name: "团队 2" })).toHaveAttribute("aria-pressed", "true");
  });
});

describe("agent overview guidance and template loading", () => {
  it.each(["local", "remote"])("displays and searches %s template display names", async (kind) => {
    const user = userEvent.setup();
    const saved: HubTemplate = {
      id: kind === "local" ? "local.internal-worker" : "alice/internal-worker",
      name: "internal-worker",
      role: "worker",
      display_name: "测试中文名",
      namespace: kind === "remote" ? "alice" : undefined,
      source: { kind },
    };
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [saved] }));
    render(<Page route="/agents?tab=templates&filter=personal" username="alice" />);
    expect(screen.getByRole("button", { name: /测试中文名/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /internal-worker/ })).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox"), "测试中文");
    await user.click(screen.getByRole("button", { name: /测试中文名/ }));
    expect(await screen.findByRole("region", { name: "Template detail" })).toHaveTextContent(saved.id!);
  });

  it("opens My templates and selects the saved template after asynchronous loading", async () => {
    const local: HubTemplate = { id: "local.中文助手", name: "中文助手", role: "worker", source: { kind: "local" } };
    const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView");
    const route = "/agents?tab=templates&filter=personal&template=local.%E4%B8%AD%E6%96%87%E5%8A%A9%E6%89%8B";
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ isLoading: true }));
    const view = render(<Page route={route} />);
    expect(screen.getByRole("button", { name: "我的" })).toHaveAttribute("aria-pressed", "true");
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [template, local] }));
    view.rerender(<Page route={route} />);
    expect(await screen.findByRole("region", { name: "Template detail" })).toHaveTextContent(local.id!);
    expect(screen.getByRole("region", { name: "Template detail" })).toHaveAttribute("data-inline", "true");
    expect(screen.getByRole("button", { name: "中文助手" })).toHaveAttribute("aria-current", "true");
    expect(screen.queryByRole("button", { name: "Codex worker" })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(scroll).toHaveBeenCalledWith({ block: "nearest" });
    scroll.mockRestore();
  });

  it("selects and highlights a newly published community template alongside its local snapshot", async () => {
    const local: HubTemplate = { id: "local.中文助手", name: "中文助手", role: "worker", source: { kind: "local" } };
    const published: HubTemplate = { ...local, id: "alice/中文助手", source: { kind: "remote" } };
    const route = "/agents?tab=templates&filter=personal&template=alice%2F%E4%B8%AD%E6%96%87%E5%8A%A9%E6%89%8B";
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [template, local] }));
    const view = render(<Page route={route} username="alice" />);
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [template, local, published] }));
    view.rerender(<Page route={route} username="alice" />);
    expect(await screen.findByRole("region", { name: "Template detail" })).toHaveTextContent(published.id!);
    const rows = screen.getAllByRole("button", { name: /中文助手/ });
    expect(rows).toHaveLength(2);
    const selected = rows.find((row) => row.getAttribute("aria-current") === "true");
    expect(selected).toHaveTextContent("云端");
    expect(rows.filter((row) => row.getAttribute("aria-current") === "true")).toHaveLength(1);
  });

  it("restores the selected template with browser back and forward", async () => {
    const locals: HubTemplate[] = [
      { id: "local.first", name: "First", role: "worker", source: { kind: "local" } },
      { id: "local.second", name: "Second", role: "worker", source: { kind: "local" } },
    ];
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: locals }));
    render(<Page route="/agents?tab=templates&filter=personal&template=local.first" />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Second" }));
    expect(await screen.findByRole("region", { name: "Template detail" })).toHaveTextContent("local.second");
    await user.click(screen.getByRole("button", { name: "Back" }));
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Template detail" })).toHaveTextContent("local.first"),
    );
    expect(screen.getByRole("button", { name: "First" })).toHaveAttribute("aria-current", "true");
    await user.click(screen.getByRole("button", { name: "Forward" }));
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Template detail" })).toHaveTextContent("local.second"),
    );
    expect(screen.getByRole("button", { name: "Second" })).toHaveAttribute("aria-current", "true");
  });

  it("waits for templates before showing selection guidance", () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ isLoading: true, isFetching: true }));
    const view = render(<Page />);
    expect(screen.getByRole("heading", { name: "正在加载智能体模板…" })).toBeInTheDocument();
    expect(screen.queryByText("正在加载 IM 工作区...")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "选一个模板" })).not.toBeInTheDocument();

    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [template] }));
    view.rerender(<Page />);
    expect(screen.queryByRole("heading", { name: "正在加载智能体模板…" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Codex worker/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "选一个模板" })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
  });

  it("keeps existing templates visible during a background refresh", () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [template], isFetching: true }));
    render(<Page />);
    expect(screen.getByText("正在刷新模板…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Codex worker/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "选一个模板" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "正在加载智能体模板…" })).not.toBeInTheDocument();
  });

  it("explains a failed initial load and offers a retry", async () => {
    const state = queryState({ isError: true });
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(state);
    render(<Page />);
    expect(screen.getByRole("heading", { name: "模板加载失败" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "选一个模板" })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getAllByRole("button", { name: "重试" })[0]);
    expect(state.refetch).toHaveBeenCalledOnce();
  });

  it("distinguishes an empty category from an empty search", async () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [] }));
    render(<Page />);
    expect(screen.getByRole("heading", { name: "暂无官方模板" })).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "我的 0" }));
    expect(screen.getByRole("heading", { name: "还没有自己的模板" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "搜索模板名称、用途说明或来源" }), "missing");
    expect(
      screen.getByRole("heading", { name: createTranslator("zh")("workspaceSearchNoResults") }),
    ).toBeInTheDocument();
    expect(screen.getByText("试试其他关键词，或清空搜索。")).toBeInTheDocument();
  });

  it("shows the agent and team guide before an item is selected", () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState());
    render(<Page route="/agents" />);
    expect(screen.getByRole("heading", { name: "选一个智能体或团队" })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.getByRole("button", { name: "新建智能体" })).toBeInTheDocument();
    expect(screen.getByText("点左侧任意一行，右边就是它的管理页")).toBeInTheDocument();
  });
});

it("moves the signed-in user's community templates to My templates with a cloud tag", async () => {
  const remote = (namespace: string): HubTemplate => ({
    id: `${namespace}/worker`,
    name: `${namespace} worker`,
    namespace,
    source: { name: "official", kind: "remote" },
  });
  vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
    queryState({
      data: [
        template,
        remote("agentichub"),
        remote("alice"),
        remote("bob"),
        { id: "local.worker", name: "Local worker", role: "worker", source: { kind: "local" } },
      ],
    }),
  );
  const view = render(<Page username="alice" />);
  expect(screen.getByRole("button", { name: /agentichub worker/ })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /alice worker/ })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "我的 2" }));
  expect(screen.getByRole("button", { name: /alice worker 云端/ })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Local worker" })).toBeInTheDocument();
  expect(screen.getAllByText("云端")).toHaveLength(1);
  expect(screen.getByRole("link", { name: "在 AgenticHub 上管理我的模板" })).toHaveAttribute(
    "href",
    "https://qa.opencsg.com/agentichub?view=resources&subview=template",
  );
  expect(screen.queryByRole("button", { name: /bob worker|agentichub worker|Codex worker/ })).not.toBeInTheDocument();
  view.rerender(<Page />);
  expect(screen.queryByRole("button", { name: /alice worker/ })).not.toBeInTheDocument();
  expect(screen.queryByText("云端")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Local worker" })).toBeInTheDocument();
});

describe("agent overview list summaries", () => {
  beforeEach(() => vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState()));

  const agents: AgentLike[] = [
    { id: "online", name: "在线助手", status: "running" },
    { id: "offline", name: "离线助手", status: "stopped" },
    {
      id: "degraded",
      name: "网关异常助手",
      runtime: { state: "running", availability: { state: "degraded" } },
    },
  ];
  const teams: WorkspaceTeam[] = ["active", "paused", "archived"].map((status) => ({
    id: status,
    title: `${status} team`,
    lead_agent_id: "online",
    member_agent_ids: ["u-online", "offline", "offline"],
    status,
    created_at: "",
    updated_at: "",
  }));

  it("counts agents and teams without changing category totals when searching", async () => {
    render(<Page route="/agents" agents={agents} teams={teams} />);
    expect(screen.getByRole("button", { name: "全部 6" })).toHaveAttribute("aria-pressed", "true");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "我的智能体 3" }));
    expect(screen.getByRole("button", { name: /在线助手/ })).toBeVisible();
    expect(screen.queryByRole("button", { name: /active team/ })).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox"), "active");
    await user.click(screen.getByRole("button", { name: "团队 3" }));
    expect(screen.getByRole("button", { name: /active team/ })).toBeVisible();
    expect(screen.queryByRole("button", { name: /paused team/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "全部 6" })).toBeVisible();
  });

  it("shows availability rather than treating a running but degraded runtime as online", () => {
    const view = render(<Page route="/agents" agents={agents} />);
    expect(within(screen.getByRole("button", { name: /在线助手/ })).getByRole("img", { name: "在线" })).toHaveAttribute(
      "data-available",
      "true",
    );
    for (const name of ["离线助手", "网关异常助手"]) {
      expect(
        within(screen.getByRole("button", { name: new RegExp(name) })).getByRole("img", { name: "离线" }),
      ).toHaveAttribute("data-available", "false");
    }
    view.rerender(<Page route="/agents" agents={[{ ...agents[1], status: "running" }]} />);
    expect(within(screen.getByRole("button", { name: /离线助手/ })).getByRole("img", { name: "在线" })).toHaveAttribute(
      "data-available",
      "true",
    );
  });

  it("labels teams and summarizes their deduplicated membership and persisted status", () => {
    render(<Page route="/agents" agents={agents} teams={teams} />);
    for (const [index, status] of ["活跃", "已暂停", "已归档"].entries()) {
      const row = within(screen.getByRole("button", { name: new RegExp(teams[index].title) }));
      expect(row.getByText("团队")).toBeVisible();
      expect(row.getByText(`2 个成员 · ${status}`)).toBeVisible();
    }
  });

  it("counts visible templates by official and owned groups, excluding hidden builtins", async () => {
    const remote = (namespace: string): HubTemplate => ({
      id: `${namespace}/worker`,
      name: `${namespace} worker`,
      namespace,
      source: { kind: "remote" },
    });
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
      queryState({
        data: [
          template,
          { id: "builtin.manager", name: "Manager template", role: "manager", source: { kind: "builtin" } },
          remote("agentichub"),
          remote("alice"),
          remote("bob"),
          { id: "local.worker", name: "Local worker", role: "worker", source: { kind: "local" } },
        ],
      }),
    );
    render(<Page username="alice" />);
    expect(screen.getByRole("tab", { name: "智能体模板 5" })).toBeVisible();
    expect(screen.getByRole("button", { name: "官方 2" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.setup().click(screen.getByRole("button", { name: "我的 2" }));
    expect(screen.getByRole("button", { name: /alice worker/ })).toBeVisible();
    expect(screen.getByRole("button", { name: "Local worker" })).toBeVisible();
    expect(screen.queryByRole("button", { name: /Manager template/ })).not.toBeInTheDocument();
  });

  it("does not claim zero templates while the initial request is pending", () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ isLoading: true }));
    const view = render(<Page />);
    expect(screen.getByRole("tab", { name: "智能体模板" })).toBeVisible();
    expect(screen.getByRole("button", { name: "官方" })).toBeVisible();
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [] }));
    view.rerender(<Page />);
    expect(screen.getByRole("tab", { name: "智能体模板 0" })).toBeVisible();
    expect(screen.getByRole("button", { name: "官方 0" })).toBeVisible();
    expect(screen.getByRole("button", { name: "我的 0" })).toBeVisible();
  });
});
