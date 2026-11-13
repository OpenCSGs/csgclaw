import { act, render as renderView, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import type { UseQueryResult } from "@tanstack/react-query";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { useWorkspaceHubTemplateQuery, useWorkspaceHubTemplatesQuery } from "@/hooks/workspace/workspaceQueries";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { AgentLike } from "@/models/agents";
import type { WorkspaceTeam } from "@/models/tasks";
import { AgentOverviewPage } from "@/pages/AgentOverviewPage/AgentOverviewPage";
import { createTranslator } from "@/shared/i18n";
import { CLOUD_AGENTS_STORAGE_KEY, PINNED_ENTITIES_STORAGE_KEY } from "@/shared/storage/keys";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";
import { fetchCloudAgentTemplate, fetchRelayCatalog } from "@/api/relay";
import { cloudAgentFixture } from "../helpers/cloudAgentFixture";
import { paneFromLocation } from "@/models/routing";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/relay", () => ({ fetchRelayCatalog: vi.fn(), fetchCloudAgentTemplate: vi.fn() }));
beforeEach(() => {
  vi.mocked(fetchRelayCatalog).mockResolvedValue([]);
  vi.mocked(fetchCloudAgentTemplate).mockResolvedValue(null);
});

vi.mock("@/hooks/workspace/workspaceQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/workspace/workspaceQueries")>()),
  useWorkspaceHubTemplatesQuery: vi.fn(),
  useWorkspaceHubTemplateQuery: vi.fn(),
}));

vi.mock("@/components/business/HubDetailPane", () => ({
  HubDetailPane: ({
    hub,
    inlineTemplateDetail,
    inlineTemplateInstanceModel,
  }: {
    hub: { detailPaneProps: { selectedTemplateId: string } };
    inlineTemplateDetail?: boolean;
    inlineTemplateInstanceModel?: string;
  }) => (
    <section aria-label="Template detail" data-inline={inlineTemplateDetail}>
      {hub.detailPaneProps.selectedTemplateId}
      {inlineTemplateInstanceModel}
    </section>
  ),
}));

function render(element: Parameters<typeof renderView>[0]) {
  return renderView(element, { wrapper: createQueryWrapper().wrapper });
}

const template: HubTemplate = {
  id: "builtin.codex-worker",
  name: "Codex worker",
  role: "worker",
  source: { name: "builtin", kind: "builtin" },
};

function queryState<T = HubTemplate[]>(patch: Partial<UseQueryResult<T>> = {}): UseQueryResult<T> {
  return {
    data: undefined,
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    ...patch,
  } as UseQueryResult<T>;
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
  authExpired = false,
  locale = "zh",
  onRequireOpenCSGAuthentication = vi.fn(),
  onLogin = vi.fn(),
  authBusy = false,
  authPending = false,
  agents = [],
  teams = [],
}: {
  route?: string;
  username?: string;
  authExpired?: boolean;
  locale?: "zh" | "en";
  onRequireOpenCSGAuthentication?: () => boolean;
  onLogin?: () => void;
  authBusy?: boolean;
  authPending?: boolean;
  agents?: AgentLike[];
  teams?: WorkspaceTeam[];
}) {
  const [selectedTemplate, setSelectedTemplate] = useState<HubTemplate>();
  const selectedTemplateId = selectedTemplate?.id || "";
  const controller = {
    ready: true,
    activePane: { type: "agents", id: "" },
    sidebarProps: {
      t: createTranslator(locale),
      authExpired,
      onRequireOpenCSGAuthentication,
      onLogin,
      authBusy,
      authPending,
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
      hub: { detailPaneProps: { selectedTemplate, selectedTemplateId, selectedResourceType: "template" } },
      t: createTranslator("zh"),
      onSelectTemplateInPlace: setSelectedTemplate,
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

function TeamNavigationContents({ remountOnNavigation = false }: { remountOnNavigation?: boolean }) {
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
    sidebarProps: {
      t,
      workerAgentItems: agents,
      teams,
      onSelectAgent: (agent: AgentLike) => navigate(`/agents/${agent.id}`),
      onSelectTeam: (team: WorkspaceTeam) => navigate(`/teams/${team.id}`),
    },
    agentViewProps: { item: null },
    teamViewProps: { t, agents, team: teams.find((team) => team.id === activePane.id) },
  } as unknown as WorkspaceController;
  return (
    <WorkspaceControllerProvider controller={controller}>
      <AgentOverviewPage key={remountOnNavigation ? `${location.pathname}${location.search}` : undefined} />
      <button onClick={() => navigate("/teams/team-1")}>Go to referenced team</button>
      <NavigationState />
    </WorkspaceControllerProvider>
  );
}

describe("agent overview team navigation", () => {
  beforeEach(() => vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState()));
  afterEach(() => {
    localStorage.removeItem(CLOUD_AGENTS_STORAGE_KEY);
    localStorage.removeItem(`${PINNED_ENTITIES_STORAGE_KEY}:${location.pathname}:`);
  });

  it("keeps pin actions separate from selection and reveals collapsed matches when searching", async () => {
    render(
      <MemoryRouter initialEntries={["/teams/team-1"]}>
        <TeamNavigationContents />
      </MemoryRouter>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "全部 3" }));
    const selected = screen.getByRole("button", { name: /回归团队 1/ });
    expect(selected).toHaveAttribute("aria-current", "true");
    const agent = () => screen.getByRole("button", { name: /测试智能体/ });
    await user.click(within(agent().parentElement!).getByRole("button", { name: "置顶" }));

    const heading = screen.getByRole("button", { name: "置顶", expanded: true });
    await user.click(heading);
    expect(heading).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("button", { name: /测试智能体/ })).not.toBeInTheDocument();
    expect(selected).toBeVisible();

    await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), "测试");
    expect(heading).toHaveAttribute("aria-expanded", "true");
    expect(agent()).toBeVisible();
    await user.click(within(agent().parentElement!).getByRole("button", { name: "取消置顶" }));
    expect(heading).not.toBeInTheDocument();
    expect(agent()).toBeVisible();
    expect(screen.getByLabelText("Current URL")).toHaveTextContent("/teams/team-1");
  });

  it("keeps All selected while switching between agent and team details, including history navigation", async () => {
    render(
      <MemoryRouter initialEntries={["/agents"]}>
        <TeamNavigationContents />
      </MemoryRouter>,
    );
    const user = userEvent.setup();
    for (const [name, path] of [
      [/测试智能体/, "/agents/agent-worker"],
      [/回归团队 1/, "/teams/team-1"],
      [/回归团队 2/, "/teams/team-2"],
      [/测试智能体/, "/agents/agent-worker"],
      [/^Back$/, "/teams/team-2"],
      [/^Forward$/, "/agents/agent-worker"],
    ] as const) {
      const list = within(document.getElementById("agent-overview-list")!);
      const controls = name.source === "^Back$" || name.source === "^Forward$" ? screen : list;
      await user.click(controls.getByRole("button", { name }));
      await waitFor(() => expect(screen.getByLabelText("Current URL")).toHaveTextContent(path));
      expect(list.getByRole("button", { name: "全部 3" })).toHaveAttribute("aria-pressed", "true");
      expect(list.getByRole("button", { name: /测试智能体/ })).toBeVisible();
      expect(list.getByRole("button", { name: /回归团队 1/ })).toBeVisible();
      expect(list.getByRole("button", { name: /回归团队 2/ })).toBeVisible();
    }
    await waitFor(() => expect(screen.getByLabelText("Current URL")).toHaveTextContent("/agents/agent-worker"));
  });

  it("keeps the team filter and search when selecting another result", async () => {
    render(
      <MemoryRouter initialEntries={["/teams/team-1"]}>
        <TeamNavigationContents />
      </MemoryRouter>,
    );
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), "回归团队");
    await user.click(screen.getByRole("button", { name: /回归团队 2/ }));
    expect(await screen.findByRole("heading", { name: "回归团队 2", level: 1 })).toBeVisible();
    expect(screen.getByRole("button", { name: "团队 2" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("textbox", { name: "搜索智能体和团队" })).toHaveValue("回归团队");
  });

  it.each(["全部 4", "云端 1"])("preserves %s and the search when opening a cloud result", async (filterLabel) => {
    vi.mocked(fetchRelayCatalog).mockResolvedValue([...cloudAgentFixture]);
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify(cloudAgentFixture));
    render(
      <MemoryRouter initialEntries={["/agents"]}>
        <TeamNavigationContents />
      </MemoryRouter>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: filterLabel }));
    await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), "合同");
    await user.click(screen.getByRole("button", { name: /合同条款审阅/ }));
    expect(await screen.findByRole("heading", { name: "合同条款审阅", level: 1 })).toBeVisible();
    expect(screen.getByRole("button", { name: filterLabel })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("textbox", { name: "搜索智能体和团队" })).toHaveValue("合同");
  });

  it.each(["/agents", "/teams/team-2"])(
    "switches the list to teams and clears the agent search when navigating from %s",
    async (route) => {
      render(
        <MemoryRouter initialEntries={[route]}>
          <TeamNavigationContents />
        </MemoryRouter>,
      );
      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "智能体 1" }));
      await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), "测试智能体");
      await user.click(screen.getByRole("button", { name: "Go to referenced team" }));
      expect(await screen.findByRole("heading", { name: "回归团队 1", level: 1 })).toBeVisible();
      expect(screen.getByRole("button", { name: "团队 2" })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("button", { name: "智能体 1" })).toHaveAttribute("aria-pressed", "false");
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

  it.each([
    { route: "/agents/agent-worker", filter: "智能体 1", query: "测试智能体" },
    { route: "/agents?cloud=agt_demo_contract_reviewer", filter: "云端 1", query: "合同" },
    { route: "/agents/agent-worker", filter: "全部 4", query: "测试智能体" },
    { route: "/agents/agent-worker", filter: "智能体 1", query: "测试智能体", remount: true },
  ])(
    "restores the $filter filter and search per history entry on Back and Forward",
    async ({ route, filter, query, remount }) => {
      vi.mocked(fetchRelayCatalog).mockResolvedValue([...cloudAgentFixture]);
      localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify(cloudAgentFixture));
      render(
        <MemoryRouter initialEntries={[route]}>
          <TeamNavigationContents remountOnNavigation={remount} />
        </MemoryRouter>,
      );
      const user = userEvent.setup();
      await user.click(await screen.findByRole("button", { name: filter }));
      await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), query);
      await user.click(screen.getByRole("button", { name: "Go to referenced team" }));
      await user.click(screen.getByRole("button", { name: "团队 2" }));
      await user.clear(screen.getByRole("textbox", { name: "搜索智能体和团队" }));
      await user.type(screen.getByRole("textbox", { name: "搜索智能体和团队" }), "回归团队 1");

      await user.click(screen.getByRole("button", { name: "Back" }));
      expect(screen.getByLabelText("Current URL")).toHaveTextContent(route);
      expect(screen.getByRole("button", { name: filter })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("textbox", { name: "搜索智能体和团队" })).toHaveValue(query);

      await user.click(screen.getByRole("button", { name: "Forward" }));
      expect(screen.getByLabelText("Current URL")).toHaveTextContent("/teams/team-1");
      expect(screen.getByRole("button", { name: "团队 2" })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("textbox", { name: "搜索智能体和团队" })).toHaveValue("回归团队 1");
    },
  );
});

describe("agent overview guidance and template loading", () => {
  it.each([
    { kind: "builtin", id: "builtin.codex-worker", filter: "official" },
    { kind: "remote", id: "agentichub/worker", filter: "official" },
    { kind: "local", id: "local.worker", filter: "personal" },
    { kind: "remote", id: "alice/worker", filter: "personal" },
  ])("shows the template avatar for $id in $filter templates", ({ kind, id, filter }) => {
    const saved: HubTemplate = {
      id,
      name: "Template with avatar",
      role: "worker",
      avatar: "avatar/3D-4.png",
      source: { kind },
    };
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [saved] }));
    render(<Page route={`/agents?tab=templates&filter=${filter}`} username="alice" />);
    const row = screen.getByRole("button", { name: /Template with avatar/ });
    expect(within(row).getByAltText("")).toHaveAttribute(
      "src",
      kind === "local" ? "avatar/3D-4.png" : "avatar/template.png",
    );
  });

  it.each([
    { kind: "builtin", id: "builtin.codex-worker" },
    { kind: "remote", id: "agentichub/worker" },
  ])("shows the shared avatar for official $id when no avatar is provided", ({ kind, id }) => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
      queryState({ data: [{ ...template, id, source: { kind } }] }),
    );
    render(<Page />);
    const row = screen.getByRole("button", { name: "Codex worker" });
    expect(within(row).getByAltText("")).toHaveAttribute("src", "avatar/template.png");
  });

  it("keeps the bundled default avatar for personal templates with no avatar", () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
      queryState({ data: [{ ...template, id: "local.worker", source: { kind: "local" } }] }),
    );
    render(<Page route="/agents?tab=templates&filter=personal" />);
    const row = screen.getByRole("button", { name: "Codex worker" });
    expect(within(row).getByAltText("")).toHaveAttribute("src", "avatar/3D-1.png");
  });

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

  it("reports a community failure without hiding loaded templates and allows retry", async () => {
    const user = userEvent.setup();
    const refetch = vi.fn();
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [template], isError: true, refetch }));
    render(<Page />);
    expect(screen.getByText("社区模板加载失败，已加载的模板仍可使用，请重试。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Codex worker/ })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(refetch).toHaveBeenCalledOnce();
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

it("keeps local templates first and remote templates in catalog order after pinning", async () => {
  vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
    queryState({
      data: [
        { id: "alice/z", name: "Zulu", namespace: "alice", source: { kind: "remote" } },
        { id: "alice/a", name: "Alpha", namespace: "alice", source: { kind: "remote" } },
        { id: "local.worker", name: "Local worker", role: "worker", source: { kind: "local" } },
      ],
    }),
  );
  const user = userEvent.setup();
  render(<Page username="alice" />);
  await user.click(screen.getByRole("button", { name: "我的 3" }));
  const local = screen.getByRole("button", { name: "Local worker" });
  const zulu = screen.getByRole("button", { name: /Zulu 云端/ });
  const alpha = screen.getByRole("button", { name: /Alpha 云端/ });
  await user.click(within(alpha.parentElement!).getByRole("button", { name: "置顶" }));
  expect(within(alpha.parentElement!).getByRole("button", { name: "取消置顶" })).toBeVisible();
  expect(local.compareDocumentPosition(zulu)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  expect(zulu.compareDocumentPosition(alpha)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
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
  expect(screen.queryByRole("heading", { name: /仅云端/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /alice worker/ })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "我的 2" }));
  expect(screen.getByRole("button", { name: /alice worker 云端/ })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Local worker" })).toBeInTheDocument();
  const cloudHeading = screen.getByRole("heading", { name: "仅云端（1）" });
  expect(screen.getByRole("button", { name: "Local worker" }).compareDocumentPosition(cloudHeading)).toBe(
    Node.DOCUMENT_POSITION_FOLLOWING,
  );
  expect(cloudHeading.compareDocumentPosition(screen.getByRole("button", { name: /alice worker 云端/ }))).toBe(
    Node.DOCUMENT_POSITION_FOLLOWING,
  );
  expect(within(screen.getByRole("button", { name: /alice worker 云端/ })).getByText("云端")).toBeVisible();
  expect(screen.getByRole("link", { name: "管理云端模板" })).toHaveAttribute(
    "href",
    "https://qa.opencsg.com/agentichub?view=resources&subview=template",
  );
  expect(screen.queryByRole("button", { name: /bob worker|agentichub worker|Codex worker/ })).not.toBeInTheDocument();
  view.rerender(<Page />);
  expect(screen.queryByRole("button", { name: /alice worker/ })).not.toBeInTheDocument();
  expect(within(screen.getByRole("complementary")).queryByText("云端")).not.toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: /仅云端/ })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Local worker" })).toBeInTheDocument();
});

describe("personal template cloud sign-in", () => {
  const route = "/agents?tab=templates&filter=personal";
  beforeEach(() => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
      queryState({
        data: [
          template,
          { id: "local.worker", name: "Local worker", role: "worker", source: { kind: "local" } },
          { id: "alice/worker", name: "Cloud worker", source: { kind: "remote" } },
        ],
      }),
    );
  });

  it("shows the sign-in prompt below usable local templates", () => {
    render(<Page route={route} />);
    const local = screen.getByRole("button", { name: "Local worker" });
    const prompt = screen.getByRole("region", { name: "登录后查看云端模板" });
    expect(local.compareDocumentPosition(prompt) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(prompt).getByText("你在 AgenticHub 上的模板，登录后才能列出来。")).toBeVisible();
    expect(within(prompt).getByRole("button", { name: "去登录" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: /Cloud worker/ })).not.toBeInTheDocument();
  });

  it.each([false, true])("uses the existing authentication guard (authorized=%s)", async (authorized) => {
    const onRequireOpenCSGAuthentication = vi.fn(() => authorized);
    const onLogin = vi.fn();
    render(<Page route={route} onRequireOpenCSGAuthentication={onRequireOpenCSGAuthentication} onLogin={onLogin} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "去登录" }));
    expect(onRequireOpenCSGAuthentication).toHaveBeenCalledOnce();
    expect(onLogin).toHaveBeenCalledTimes(authorized ? 1 : 0);
  });

  it.each(["authBusy", "authPending"] as const)("disables sign-in while %s", (flag) => {
    render(<Page route={route} {...{ [flag]: true }} />);
    expect(screen.getByRole("button", { name: "去登录" })).toBeDisabled();
  });

  it("localizes the prompt and supports keyboard sign-in", async () => {
    const onLogin = vi.fn();
    render(<Page route={route} locale="en" onLogin={onLogin} />);
    const prompt = screen.getByRole("region", { name: "Sign in to see cloud templates" });
    within(prompt).getByRole("button", { name: "Sign in" }).focus();
    await userEvent.setup().keyboard("{Enter}");
    expect(onLogin).toHaveBeenCalledOnce();
  });

  it("hides the prompt after login and restores it after logout or session expiry", () => {
    const view = render(<Page route={route} />);
    view.rerender(<Page route={route} username="alice" />);
    expect(screen.queryByRole("button", { name: "去登录" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Cloud worker 云端/ })).toBeVisible();
    view.rerender(<Page route={route} username="alice" authExpired />);
    expect(screen.getByRole("button", { name: "去登录" })).toBeVisible();
    expect(screen.queryByRole("button", { name: /Cloud worker/ })).not.toBeInTheDocument();
    view.rerender(<Page route={route} />);
    expect(screen.getByRole("button", { name: "去登录" })).toBeVisible();
  });

  it.each([false, true])("keeps the cloud section after login with no remote templates (local=%s)", (hasLocal) => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(
      queryState({
        data: hasLocal ? [{ id: "local.worker", name: "Local worker", role: "worker", source: { kind: "local" } }] : [],
      }),
    );
    const view = render(<Page route={route} />);
    expect(screen.getByRole("button", { name: "去登录" })).toBeVisible();
    view.rerender(<Page route={route} username="alice" />);
    const list = within(screen.getByRole("complementary"));
    const heading = list.getByRole("heading", { name: "仅云端（0）" });
    const empty = list.getByText("暂无云端智能体");
    expect(heading.compareDocumentPosition(empty) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(list.queryByRole("button", { name: "去登录" })).not.toBeInTheDocument();
    if (hasLocal) expect(list.getByRole("button", { name: "Local worker" })).toBeVisible();
    expect(list.getByRole("link", { name: "管理云端模板" })).toBeVisible();
  });

  it.each([{ isLoading: true }, { isFetching: true }, { isError: true }])(
    "keeps the cloud heading without showing an empty result before a successful load: %j",
    (state) => {
      vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState(state));
      render(<Page route={route} username="alice" />);
      expect(screen.getByRole("heading", { name: "仅云端（0）" })).toBeVisible();
      expect(screen.queryByText("暂无云端智能体")).not.toBeInTheDocument();
    },
  );

  it("localizes the empty cloud section", () => {
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState({ data: [] }));
    render(<Page route={route} username="alice" locale="en" />);
    expect(screen.getByRole("heading", { name: "Cloud only (0)" })).toBeVisible();
    expect(screen.getByText("No cloud agents yet")).toBeVisible();
  });

  it("distinguishes a filtered cloud list from an account with no cloud templates", async () => {
    render(<Page route={route} username="alice" />);
    await userEvent.setup().type(screen.getByRole("textbox", { name: "搜索模板名称、用途说明或来源" }), "missing");
    const list = within(screen.getByRole("complementary"));
    expect(list.getByRole("heading", { name: "仅云端（0）" })).toBeVisible();
    expect(list.getByText(createTranslator("zh")("workspaceSearchNoResults"))).toBeVisible();
    expect(list.queryByText("暂无云端智能体")).not.toBeInTheDocument();
  });

  it.each(["/agents", "/agents?tab=templates", "/agents?tab=plaza"])(
    "keeps this prompt scoped to personal templates: %s",
    (route) => {
      render(<Page route={route} />);
      expect(screen.queryByRole("region", { name: "登录后查看云端模板" })).not.toBeInTheDocument();
    },
  );

  it.each([{ isLoading: true }, { isError: true }])(
    "keeps sign-in available while templates are unavailable: %j",
    (state) => {
      vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState(state));
      render(<Page route={route} />);
      expect(screen.getByRole("button", { name: "去登录" })).toBeEnabled();
    },
  );
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
    await user.click(screen.getByRole("button", { name: "智能体 3" }));
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

describe("cloud agent plaza", () => {
  beforeEach(() => {
    vi.mocked(fetchRelayCatalog).mockResolvedValue([...cloudAgentFixture]);
    localStorage.removeItem(CLOUD_AGENTS_STORAGE_KEY);
    vi.mocked(useWorkspaceHubTemplatesQuery).mockReturnValue(queryState());
  });
  afterEach(() => localStorage.removeItem(CLOUD_AGENTS_STORAGE_KEY));

  it("refreshes the displayed agent's availability while the plaza stays open", async () => {
    vi.useFakeTimers();
    const agent = cloudAgentFixture[0];
    let status: "online" | "offline" = "online";
    vi.mocked(fetchRelayCatalog).mockImplementation(async (_signal, _target, ids) =>
      ids?.length ? [{ ...agent, status }] : [{ ...agent, status: "offline" }],
    );
    const view = render(<Page route="/agents?tab=plaza" username="alice" />);
    try {
      await act(() => vi.advanceTimersByTimeAsync(100));
      await act(() => vi.advanceTimersByTimeAsync(100));
      expect(screen.getByText("在线")).toBeVisible();

      status = "offline";
      await act(() => vi.advanceTimersByTimeAsync(30_000));
      expect(screen.getByText("离线")).toBeVisible();

      status = "online";
      await act(() => vi.advanceTimersByTimeAsync(30_000));
      expect(screen.getByText("在线")).toBeVisible();
    } finally {
      view.unmount();
      vi.useRealTimers();
    }
  });

  it("keeps the community order and labels only recommended agents", async () => {
    const base = cloudAgentFixture[0];
    vi.mocked(fetchRelayCatalog).mockResolvedValue([
      { ...base, id: "z-first", name: "Zulu", recommended: false },
      { ...base, id: "a-second", name: "Alpha", recommended: true },
    ]);
    render(<Page route="/agents?tab=plaza" username="alice" />);
    const first = await screen.findByRole("heading", { name: "Zulu" });
    const second = screen.getByRole("heading", { name: /Alpha.*推荐/ });
    expect(first.compareDocumentPosition(second)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(screen.getAllByText("推荐")).toHaveLength(1);
  });

  it("includes the complete cloud description in the profile body", async () => {
    const description = "第一段：" + "完整的云端智能体说明。".repeat(15) + "\n第二段：保留换行和结尾。";
    const agent = { ...cloudAgentFixture[0], description };
    vi.mocked(fetchRelayCatalog).mockResolvedValue([agent]);
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify([agent]));
    render(<Page route={`/agents?cloud=${agent.id}`} />);
    const body = await screen.findByRole("region", { name: "说明" });
    expect(body.querySelector("p")?.textContent).toBe(description);
    expect(within(body).getByRole("heading", { name: "说明" })).toBeVisible();
    expect(body.closest(".agent-profile-scroll-region")).toBeInTheDocument();
  });

  it("switches from empty My scope to Public and adds an agent without leaving the plaza", async () => {
    const user = userEvent.setup();
    render(<Page route="/agents?tab=plaza" username="alice" />);
    await user.click(await screen.findByRole("button", { name: "我的" }));
    await user.click(screen.getByRole("button", { name: "浏览公共智能体" }));
    expect(screen.getByRole("button", { name: "公共" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText("Current URL")).toHaveTextContent("/agents?tab=plaza");
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "添加到我的智能体" }));
    expect(screen.getByRole("heading", { name: cloudAgentFixture[0].name })).toBeVisible();
    expect(screen.getByText("已添加")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "我的" }));
    expect(screen.queryByRole("heading", { name: cloudAgentFixture[0].name })).not.toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY)!)).toEqual(cloudAgentFixture);
  });

  it.each(["loading", "failed", "ready"])(
    "offers authorization instead of the %s catalog when signed out",
    async (state) => {
      if (state === "loading") vi.mocked(fetchRelayCatalog).mockReturnValue(new Promise(() => {}));
      if (state === "failed") vi.mocked(fetchRelayCatalog).mockRejectedValue(new Error("Sign in required"));
      const onRequireOpenCSGAuthentication = vi.fn(() => false);
      render(<Page route="/agents?tab=plaza" onRequireOpenCSGAuthentication={onRequireOpenCSGAuthentication} />);
      await act(async () => {});

      expect(screen.getByRole("heading", { name: "智能体广场", level: 1 })).toBeVisible();
      expect(screen.getByText("看中的，招呼一声就来")).toBeVisible();
      const description = screen.getByText(createTranslator("zh")("agentPlazaDescription"));
      expect(description).toBeVisible();
      expect(description.compareDocumentPosition(screen.getByRole("button", { name: "授权登录" }))).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING,
      );
      expect(screen.getByText("智能体广场是云端资源，登录账号后才能浏览和添加。")).toBeVisible();
      expect(screen.queryByText("正在加载已发布智能体…")).not.toBeInTheDocument();
      expect(screen.queryByText("无法加载智能体广场")).not.toBeInTheDocument();
      expect(screen.queryByRole("textbox", { name: "搜索公共智能体" })).not.toBeInTheDocument();
      expect(screen.queryByRole("heading", { name: "合同条款审阅" })).not.toBeInTheDocument();
      await userEvent.setup().click(screen.getByRole("button", { name: "授权登录" }));
      expect(onRequireOpenCSGAuthentication).toHaveBeenCalledOnce();
    },
  );

  it("localizes the sign-in prompt and supports keyboard authorization", async () => {
    const onRequireOpenCSGAuthentication = vi.fn(() => false);
    render(
      <Page route="/agents?tab=plaza" locale="en" onRequireOpenCSGAuthentication={onRequireOpenCSGAuthentication} />,
    );
    expect(screen.getByText("The agent plaza lives in the cloud. Sign in to browse and add agents.")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Agent Plaza", level: 1 })).toBeVisible();
    expect(screen.getByText(createTranslator("en")("agentPlazaDescription"))).toBeVisible();
    screen.getByRole("button", { name: "Authorize & sign in" }).focus();
    await userEvent.setup().keyboard("{Enter}");
    expect(onRequireOpenCSGAuthentication).toHaveBeenCalledOnce();
  });

  it("requires authorization again when the session has expired", () => {
    render(<Page route="/agents?tab=plaza" username="alice" authExpired />);
    expect(screen.getByRole("button", { name: "授权登录" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "添加到我的智能体" })).not.toBeInTheDocument();
  });

  it("preserves the introduction and allows reauthorization when the catalog rejects the current login", async () => {
    vi.mocked(fetchRelayCatalog).mockRejectedValue({ code: "relay_login_required" });
    const onLogin = vi.fn();
    const onRequireOpenCSGAuthentication = vi.fn(() => true);
    render(
      <Page
        route="/agents?tab=plaza"
        username="alice"
        onLogin={onLogin}
        onRequireOpenCSGAuthentication={onRequireOpenCSGAuthentication}
      />,
    );
    const button = await screen.findByRole("button", { name: "授权登录" });
    expect(screen.getByRole("heading", { name: "智能体广场", level: 1 })).toBeVisible();
    expect(screen.queryByText("无法加载智能体广场")).not.toBeInTheDocument();
    await userEvent.setup().click(button);
    expect(onRequireOpenCSGAuthentication).toHaveBeenCalledOnce();
    expect(onLogin).toHaveBeenCalledOnce();
  });

  it.each(["authBusy", "authPending"] as const)("disables authorization while %s", (flag) => {
    render(<Page route="/agents?tab=plaza" {...{ [flag]: true }} />);
    expect(screen.getByRole("button", { name: "授权登录" })).toBeDisabled();
  });

  it("replaces the sign-in prompt with the catalog after login and restores it after logout", async () => {
    const view = render(<Page route="/agents?tab=plaza" />);
    expect(screen.getByRole("button", { name: "授权登录" })).toBeVisible();
    const description = createTranslator("zh")("agentPlazaDescription");
    expect(screen.getByText(description)).toBeVisible();
    view.rerender(<Page route="/agents?tab=plaza" username="alice" />);
    expect(await screen.findByRole("button", { name: "添加到我的智能体" })).toBeVisible();
    expect(screen.getByText(description)).toBeVisible();
    expect(screen.queryByRole("button", { name: "授权登录" })).not.toBeInTheDocument();
    view.rerender(<Page route="/agents?tab=plaza" />);
    expect(screen.getByText(description)).toBeVisible();
    expect(screen.getByRole("button", { name: "授权登录" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "添加到我的智能体" })).not.toBeInTheDocument();
  });

  it("adds a cloud agent, opens its separate detail and removes it from My agents", async () => {
    const user = userEvent.setup();
    render(<Page route="/agents?tab=plaza" username="alice" />);
    expect(await screen.findByRole("heading", { name: "智能体广场" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "合同条款审阅" })).toBeVisible();
    expect(screen.getByText("在线")).toBeVisible();
    expect(screen.queryByText("忙碌中")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "添加到我的智能体" }));
    expect(await screen.findByText("已添加")).toBeVisible();
    expect(screen.queryByRole("button", { name: "添加到我的智能体" })).not.toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY)!)).toEqual(cloudAgentFixture);
    await user.click(screen.getByRole("button", { name: "查看智能体" }));
    expect(screen.getByLabelText("Current URL")).toHaveTextContent("/agents?cloud=agt_demo_contract_reviewer");
    expect(screen.getByRole("heading", { name: "合同条款审阅" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "档案" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "开始聊天" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "全部 1" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /合同条款审阅\s*在线\s*云端/ })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("tab", { name: "我的智能体 1" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: "更多" }));
    await user.click(screen.getByRole("menuitem", { name: "从我的智能体移除" }));
    expect(screen.getByRole("button", { name: "云端 0" })).toBeVisible();
    expect(JSON.parse(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY)!)).toEqual([]);
  });

  it("opens the existing template detail for an AgenticHub instance without leaving its profile", async () => {
    const agent = { ...cloudAgentFixture[0], publisher: "Agentichub", modelID: "instance-model" };
    const source: HubTemplate = { id: "alice/worker", name: "Worker", source: { kind: "remote" } };
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify([agent]));
    vi.mocked(fetchRelayCatalog).mockResolvedValue([agent]);
    vi.mocked(fetchCloudAgentTemplate).mockResolvedValue({ repo_path: "alice/worker" });
    vi.mocked(useWorkspaceHubTemplateQuery).mockReturnValue({ data: source } as UseQueryResult<HubTemplate>);
    render(<Page route={`/agents?cloud=${agent.id}`} />);
    expect(await screen.findByLabelText("Template detail")).toHaveTextContent("alice/worker");
    expect(screen.getByLabelText("Template detail")).toHaveTextContent("instance-model");
    await userEvent.setup().click(screen.getByRole("button", { name: "更多" }));
    expect(await screen.findByRole("menuitem", { name: "从我的智能体移除" })).toBeVisible();
    expect(screen.getByLabelText("Current URL")).toHaveTextContent(`/agents?cloud=${agent.id}`);
  });

  it("keeps cloud agent metadata visible when its source template fails and allows retry", async () => {
    const agent = {
      ...cloudAgentFixture[0],
      publisher: "Agentichub",
      creatorName: "Template creator",
      modelID: "instance-model",
      templateRepoPath: "alice/worker",
    };
    const refetch = vi.fn();
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify([agent]));
    vi.mocked(fetchRelayCatalog).mockResolvedValue([agent]);
    vi.mocked(useWorkspaceHubTemplateQuery).mockReturnValue(queryState<HubTemplate>({ isError: true, refetch }));
    const user = userEvent.setup();

    render(<Page route={`/agents?cloud=${agent.id}`} />);

    expect(await screen.findByText("暂时无法加载来源模板。")).toBeVisible();
    expect(screen.getByText("Template creator")).toBeVisible();
    expect(screen.getByText("Agentichub")).toBeVisible();
    expect(screen.getByText("instance-model")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("opens the shared new-task page for the selected cloud agent", async () => {
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify(cloudAgentFixture));
    const user = userEvent.setup();
    render(<Page route="/agents?cloud=agt_demo_contract_reviewer" />);

    await user.click(await screen.findByRole("button", { name: "新建任务" }));

    expect(screen.getByLabelText("Current URL")).toHaveTextContent("/new?cloud=agt_demo_contract_reviewer");
  });

  it("restores added agents after remount and supports searching the plaza", async () => {
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify(cloudAgentFixture));
    const user = userEvent.setup();
    const view = render(<Page route="/agents?cloud=agt_demo_contract_reviewer" />);
    expect(await screen.findByRole("button", { name: /合同条款审阅\s*在线\s*云端/ })).toBeVisible();
    view.unmount();
    render(<Page route="/agents?tab=plaza" username="alice" />);
    expect(await screen.findByText("已添加")).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "搜索公共智能体" }), "not found");
    expect(screen.queryByRole("heading", { name: "合同条款审阅" })).not.toBeInTheDocument();
    await user.clear(screen.getByRole("textbox", { name: "搜索公共智能体" }));
    expect(screen.getByRole("heading", { name: "合同条款审阅" })).toBeVisible();
    expect(screen.getByRole("button", { name: "查看智能体" })).toBeVisible();
  });

  it("does not resurrect an unpublished agent from saved metadata", async () => {
    localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify(cloudAgentFixture));
    vi.mocked(fetchRelayCatalog).mockResolvedValue([]);
    render(<Page route="/agents?cloud=agt_demo_contract_reviewer" />);
    expect(await screen.findByText("这个云端智能体尚未添加")).toBeVisible();
    expect(screen.queryByRole("button", { name: /合同条款审阅/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "更多" })).not.toBeInTheDocument();
  });

  it("refreshes after publication and exposes recoverable directory errors", async () => {
    vi.mocked(fetchRelayCatalog).mockRejectedValue(new Error("Unavailable"));
    const user = userEvent.setup();
    render(<Page route="/agents?tab=plaza" username="alice" />);
    expect(await screen.findByText("无法加载智能体广场")).toBeVisible();
    vi.mocked(fetchRelayCatalog).mockResolvedValue([]);
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByRole("heading", { name: "智能体广场" })).toBeVisible();
    vi.mocked(fetchRelayCatalog).mockResolvedValue([...cloudAgentFixture]);
    await act(async () => {
      window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT));
    });
    expect(await screen.findByRole("heading", { name: "合同条款审阅" })).toBeVisible();
  });

  it("shows an empty My scope before adding and keeps local agents separate", async () => {
    const user = userEvent.setup();
    render(<Page route="/agents?tab=plaza" username="alice" agents={[{ id: "agent-local", name: "Local worker" }]} />);
    await user.click(await screen.findByRole("button", { name: "我的" }));
    expect(screen.getByText("当前账号还没有自己的云端智能体。")).toBeVisible();
    await user.click(screen.getByRole("tab", { name: "我的智能体 1" }));
    expect(screen.getByRole("button", { name: /Local worker/ })).toBeVisible();
    expect(screen.getByRole("button", { name: "云端 0" })).toBeVisible();
  });
});
