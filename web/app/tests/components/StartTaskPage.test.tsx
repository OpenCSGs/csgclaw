import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createQueryWrapper } from "../helpers/queryClient";
import { fetchConversationResourceBatch } from "@/api/agentResources";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { sendMessageRequest } from "@/api/im";
import { StartTaskPage } from "@/pages/StartTaskPage/StartTaskPage";
import type { IMConversation, LocaleCode } from "@/models/conversations";
import { createTranslator } from "@/shared/i18n";
import { TASK_EXAMPLES_HIDDEN_STORAGE_KEY } from "@/shared/storage/keys";

vi.mock("@/api/agentResources", () => ({ fetchConversationResourceBatch: vi.fn() }));
vi.mock("@/api/im", () => ({ sendMessageRequest: vi.fn() }));
const room: IMConversation = {
  id: "task-1",
  title: "Task",
  is_direct: true,
  members: ["user-admin", "manager"],
  messages: [],
};
function DialogButtons() {
  const navigate = useNavigate();
  return (
    <>
      <button onClick={() => navigate({ search: "?dialog=settings" }, { replace: true })}>Open settings</button>
      <button onClick={() => navigate({ search: "" }, { replace: true })}>Close settings</button>
    </>
  );
}
function setup(
  path = "/new",
  model = "test-model",
  options: {
    provider?: string;
    authenticated?: boolean;
    configuration?: { model_provider_id: string; model_id: string };
    locale?: LocaleCode;
  } = {},
) {
  const create = vi.fn().mockResolvedValue(room);
  const open = vi.fn();
  const onMessage = vi.fn();
  const onCreateAgent = vi.fn();
  const requireAuth = vi.fn(() => options.authenticated ?? true);
  const t = options.locale ? createTranslator(options.locale) : (key: string) => key;
  const controller = {
    ready: true,
    t,
    openAgentSettings: vi.fn(),
    agentViewProps: { authStatuses: { codex: { authenticated: options.authenticated ?? true } } },
    taskEntry: { create, open, onMessage },
    sidebarProps: {
      t,
      locale: options.locale || "zh",
      currentUserID: "user-admin",
      onRequireOpenCSGAuthentication: requireAuth,
      usersById: new Map(),
      workerAgentItems: [
        { id: "u-manager", name: "Manager", role: "manager", user_id: "manager", model_config: { model_id: model } },
        { id: "u-worker", name: "Worker", user_id: "worker", model_id: model, model_provider_id: options.provider },
      ],
      teams: [{ id: "team-1", title: "Team", lead_agent_id: "u-manager", member_agent_ids: ["u-worker"] }],
      onCreateAgent,
      onCreateCustomAgent: onCreateAgent,
      onOpenCreateTeam: vi.fn(),
      onSelectAgent: vi.fn(),
    },
  } as unknown as WorkspaceController;
  const view = render(
    <MemoryRouter
      initialEntries={[
        {
          pathname: path.split("?")[0],
          search: path.includes("?") ? `?${path.split("?")[1]}` : "",
          state: { configuration: options.configuration },
        },
      ]}
    >
      <WorkspaceControllerProvider controller={controller}>
        <StartTaskPage />
        <DialogButtons />
      </WorkspaceControllerProvider>
    </MemoryRouter>,
    { wrapper: createQueryWrapper().wrapper },
  );
  return { create, open, onMessage, onCreateAgent, requireAuth, unmount: view.unmount };
}

describe("task example visibility", () => {
  beforeEach(() => {
    localStorage.removeItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY);
    sessionStorage.removeItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY);
  });
  afterEach(() => {
    localStorage.removeItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY);
    sessionStorage.removeItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY);
  });

  it("keeps dismissal in the current tab and restores examples in a new session", () => {
    const first = setup();
    fireEvent.click(screen.getByRole("button", { name: "iaStartExamplesHide" }));
    expect(screen.queryByRole("button", { name: "iaStartExamplesHide" })).not.toBeInTheDocument();
    first.unmount();

    const sameTab = setup();
    expect(screen.queryByRole("button", { name: "iaStartExamplesHide" })).not.toBeInTheDocument();
    sameTab.unmount();

    sessionStorage.removeItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY);
    setup();
    expect(screen.getByRole("button", { name: "iaStartExamplesHide" })).toBeVisible();
  });

  it("ignores a dismissal saved permanently by older versions", () => {
    localStorage.setItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY, "true");
    setup();
    expect(screen.getByRole("button", { name: "iaStartExamplesHide" })).toBeVisible();
  });
});

describe("task presets", () => {
  beforeEach(() => {
    sessionStorage.removeItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY);
    vi.mocked(sendMessageRequest)
      .mockReset()
      .mockResolvedValue({} as never);
  });

  it.each([
    ["/new", "knowledgeCard", "把资料变成易懂图文知识卡", "咖啡冲煮入门", ["manager"]],
    ["/new?agent=u-worker", "meetingNotes", "编写项目会议纪要模板", "版本上线复盘会", ["worker"]],
    ["/new", "aiTrends", "生成“近一年 AI 趋势”情报简报式PPT", "近 12 个月", ["manager"]],
    ["/new?team=team-1", "members", "查看协作空间成员并列出职责", "不要推测或编造", ["manager", "worker"]],
    ["/new?team=team-1", "research", "调研 Agent 落地现状", "\n1.产品与厂商", ["manager", "worker"]],
    ["/new?team=team-1", "review", "公告改动评审", "\n“我们很高兴地宣布", ["manager", "worker"]],
  ] as const)("starts %s with the full %s preset", async (path, key, title, content, members) => {
    const { create, open } = setup(path, "test-model", { locale: "zh" });
    const prompt = createTranslator("zh")(`iaStartExample_${key}_prompt`);
    const user = userEvent.setup();

    expect(create).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: title }));

    await waitFor(() => expect(open).toHaveBeenCalledWith(room));
    expect(prompt).toContain(content);
    expect(sendMessageRequest).toHaveBeenCalledWith(expect.objectContaining({ content: prompt }));
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ member_ids: members, is_direct: !path.includes("team=") }),
    );
  });

  it.each([
    [
      "/new",
      [
        "Turn materials into a visual knowledge card",
        "Write a project meeting notes template",
        "Create an AI trends briefing presentation",
      ],
      "List workspace members and responsibilities",
    ],
    [
      "/new?team=team-1",
      [
        "List workspace members and responsibilities",
        "Research real-world Agent adoption",
        "Review a launch announcement",
      ],
      "Turn materials into a visual knowledge card",
    ],
  ] as const)("shows localized presets for %s", (path, titles, otherTargetTitle) => {
    setup(path, "test-model", { locale: "en" });
    for (const title of titles) expect(screen.getByRole("button", { name: title })).toBeVisible();
    expect(screen.queryByRole("button", { name: otherTargetTitle })).not.toBeInTheDocument();
  });
});

describe("StartTaskPage task preparation", () => {
  beforeEach(() => vi.mocked(sendMessageRequest).mockReset());
  it.each([
    ["/", "u-manager"],
    ["/new?agent=u-worker", "u-worker"],
  ])("opens resource management for the selected agent from %s", async (path, agentID) => {
    vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
      items: [],
      total: 0,
      list_revision: "r",
      has_more: false,
    });
    setup(path);
    const user = userEvent.setup();
    for (const [label, tab] of [
      ["composerResourceSkill", "skills"],
      ["composerResourceConnector", "connectors"],
      ["composerResourceKnowledge", "connectors"],
    ]) {
      await user.click(screen.getByRole("button", { name: "composerAddContent" }));
      await user.click(screen.getByRole("button", { name: label }));
      expect(screen.getByRole("link", { name: "composerResourceManage" })).toHaveAttribute(
        "href",
        `#/agents/${agentID}?tab=${tab}`,
      );
      await user.keyboard("{Escape}");
      await waitFor(() => expect(screen.getByRole("textbox", { name: "taskEntryPrompt" })).toHaveFocus());
    }
  });

  it("focuses the task prompt when starting a task with the selected agent", () => {
    setup("/new?agent=u-worker");
    expect(screen.getByRole("textbox", { name: "taskEntryPrompt" })).toHaveFocus();
  });

  it.each(["opencsg", "codex"])(
    "checks %s authentication for the selected room model before the first prompt",
    async (provider) => {
      const { create, requireAuth } = setup("/new?agent=u-worker", "default-model", {
        authenticated: false,
        provider: "local",
        configuration: { model_provider_id: provider, model_id: "override-model" },
      });
      fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), { target: { value: "continue" } });
      fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
      if (provider === "opencsg") expect(requireAuth).toHaveBeenCalledOnce();
      else expect(await screen.findByRole("alert")).toHaveTextContent("authRequired");
      expect(create).not.toHaveBeenCalled();
      expect(sendMessageRequest).not.toHaveBeenCalled();
    },
  );

  it("does not create a session until sending, and preserves the created session and message id on retry", async () => {
    vi.mocked(sendMessageRequest)
      .mockRejectedValueOnce(new Error("network failure"))
      .mockResolvedValueOnce({} as never);
    const { create, open } = setup();
    expect(create).not.toHaveBeenCalled();
    fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), { target: { value: "  检查代码  " } });
    fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
    await screen.findByRole("alert");
    expect(create).toHaveBeenCalledTimes(1);
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ title: "检查代码", is_direct: true, member_ids: ["manager"] }),
    );
    expect(open).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox", { name: "taskEntryPrompt" })).toHaveValue("  检查代码  ");
    fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
    await waitFor(() => expect(open).toHaveBeenCalledWith(room));
    expect(create).toHaveBeenCalledTimes(1);
    const sends = vi.mocked(sendMessageRequest).mock.calls;
    expect(sends[0][0].client_message_id).toBe(sends[1][0].client_message_id);
  });
  it("stores the accepted first message before opening the conversation", async () => {
    const message = { id: "first", sender_id: "user-admin", content: "hello" };
    vi.mocked(sendMessageRequest).mockResolvedValue(message);
    const { open, onMessage } = setup();
    fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), { target: { value: "hello" } });
    fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
    await waitFor(() => expect(open).toHaveBeenCalledWith(room));
    expect(onMessage).toHaveBeenCalledWith(room.id, message);
    expect(onMessage.mock.invocationCallOrder[0]).toBeLessThan(open.mock.invocationCallOrder[0]);
  });
  it("creates and opens a task for the selected worker instead of Manager", async () => {
    vi.mocked(sendMessageRequest).mockResolvedValue({} as never);
    const { create, open } = setup("/new?agent=u-worker");
    fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), { target: { value: "hi" } });
    fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
    await waitFor(() => expect(open).toHaveBeenCalledWith(room));
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ member_ids: ["worker"], is_direct: true, type: "free" }),
    );
    expect(sendMessageRequest).toHaveBeenCalledWith(expect.objectContaining({ room_id: room.id, content: "hi" }));
  });
  it("creates collaboration using a team member snapshot and on-demand delivery", async () => {
    vi.mocked(sendMessageRequest).mockResolvedValue({} as never);
    const { create, open } = setup("/new?team=team-1");
    expect(screen.queryByLabelText("iaWorkdirTitle")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("iaModelTitle")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("iaReasoningTitle")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "taskEntryEmptyRoom" }));
    await waitFor(() => expect(open).toHaveBeenCalled());
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        is_direct: false,
        type: "on_demand",
        manager_id: "manager",
        member_ids: ["manager", "worker"],
      }),
    );
    expect(sendMessageRequest).not.toHaveBeenCalled();
  });
  it("keeps configuration failure in preparation without creating an unusable conversation", async () => {
    const { create } = setup("/new", "");
    fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), { target: { value: "Task" } });
    fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
    expect(screen.getByRole("status")).toHaveTextContent("taskEntryModelMissing");
    expect(screen.getByRole("button", { name: "taskEntryStart" })).toBeDisabled();
    expect(create).not.toHaveBeenCalled();
  });
  it("preserves the unsent task while Settings opens and closes on the same page", () => {
    setup();
    fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), {
      target: { value: "unsubmitted task" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open settings" }));
    expect(screen.getByRole("textbox", { name: "taskEntryPrompt" })).toHaveValue("unsubmitted task");
    fireEvent.click(screen.getByRole("button", { name: "Close settings" }));
    expect(screen.getByRole("textbox", { name: "taskEntryPrompt" })).toHaveValue("unsubmitted task");
  });
  it("reports a stale target instead of silently doing nothing", async () => {
    const { create } = setup("/new?agent=deleted");
    fireEvent.change(screen.getByRole("textbox", { name: "taskEntryPrompt" }), { target: { value: "Task" } });
    fireEvent.click(screen.getByRole("button", { name: "taskEntryStart" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("taskEntryMissingTarget");
    expect(create).not.toHaveBeenCalled();
  });
});

it("StartTask 使用规范化输入检查资源组合，校验失败时不创建会话", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    items: [{ source: "mcp", id: "search", type: "connector", name: "Search", enabled: true, available: true }],
    total: 1,
    list_revision: "r",
    has_more: false,
  });
  vi.mocked(sendMessageRequest)
    .mockReset()
    .mockResolvedValue({} as never);
  const { create, open } = setup("/new?agent=u-worker");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "composerAddContent" }));
  await user.click(screen.getByRole("button", { name: "composerResourceConnector" }));
  await user.click(await screen.findByRole("option", { name: "Search" }));
  const input = screen.getByRole("textbox", { name: "taskEntryPrompt" });
  await user.type(input, "/review");
  expect(screen.getByRole("button", { name: "taskEntryStart" })).toBeDisabled();
  await user.keyboard("{Enter}");
  expect(create).not.toHaveBeenCalled();
  expect(sendMessageRequest).not.toHaveBeenCalled();
  await user.type(input, " 检查项目");
  await user.click(screen.getByRole("button", { name: "taskEntryStart" }));
  await waitFor(() => expect(open).toHaveBeenCalledWith(room));
  expect(create).toHaveBeenCalledTimes(1);
  expect(sendMessageRequest).toHaveBeenCalledWith(
    expect.objectContaining({
      content: "检查项目",
      resource_refs: [
        { source: "mcp", id: "search" },
        { source: "skill", id: "review" },
      ],
    }),
  );
});

it("StartTask 支持单个 skill 的空正文输入", async () => {
  vi.mocked(sendMessageRequest)
    .mockReset()
    .mockResolvedValue({} as never);
  const { create, open } = setup("/new?agent=u-worker");
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "taskEntryPrompt" }), "/review");
  await user.click(screen.getByRole("button", { name: "taskEntryStart" }));
  await waitFor(() => expect(open).toHaveBeenCalledWith(room));
  expect(create).toHaveBeenCalledTimes(1);
  expect(sendMessageRequest).toHaveBeenCalledWith(
    expect.objectContaining({ content: "", resource_refs: [{ source: "skill", id: "review" }] }),
  );
});

it("removes a selected resource while preserving the task text", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    items: [{ source: "mcp", id: "search", type: "connector", name: "Search", enabled: true, available: true }],
    total: 1,
    list_revision: "r",
    has_more: false,
  });
  vi.mocked(sendMessageRequest)
    .mockReset()
    .mockResolvedValue({} as never);
  const { open } = setup("/new?agent=u-worker");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "composerAddContent" }));
  await user.click(screen.getByRole("button", { name: "composerResourceConnector" }));
  await user.click(await screen.findByRole("option", { name: "Search" }));
  await user.type(screen.getByRole("textbox", { name: "taskEntryPrompt" }), "Check project");
  const remove = screen.getByRole("button", { name: "remove Search" });
  expect(remove.closest(".resource-tag")).toHaveAttribute("data-resource-type", "connector");
  await user.click(remove);
  expect(screen.getByRole("textbox", { name: "taskEntryPrompt" })).toHaveValue("Check project");
  await user.click(screen.getByRole("button", { name: "taskEntryStart" }));
  await waitFor(() => expect(open).toHaveBeenCalledWith(room));
  expect(sendMessageRequest).toHaveBeenCalledWith(
    expect.objectContaining({ content: "Check project", resource_refs: [] }),
  );
});
