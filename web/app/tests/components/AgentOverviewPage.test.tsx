import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import type { UseQueryResult } from "@tanstack/react-query";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { useWorkspaceHubTemplatesQuery } from "@/hooks/workspace/workspaceQueries";
import type { HubTemplate } from "@/models/hubWorkspace";
import { AgentOverviewPage } from "@/pages/AgentOverviewPage/AgentOverviewPage";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/hooks/workspace/workspaceQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/workspace/workspaceQueries")>()),
  useWorkspaceHubTemplatesQuery: vi.fn(),
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

function Page({ route = "/agents?tab=templates", username = "" }: { route?: string; username?: string }) {
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
      workerAgentItems: [],
      teams: [],
    },
    agentViewProps: { item: null },
  } as unknown as WorkspaceController;
  return (
    <MemoryRouter initialEntries={[route]}>
      <WorkspaceControllerProvider controller={controller}>
        <AgentOverviewPage />
      </WorkspaceControllerProvider>
    </MemoryRouter>
  );
}

describe("agent overview guidance and template loading", () => {
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
    await user.click(screen.getByRole("button", { name: "我的模板" }));
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
  await userEvent.setup().click(screen.getByRole("button", { name: "我的模板" }));
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
