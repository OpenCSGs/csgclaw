import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createQueryWrapper } from "../helpers/queryClient";
import { fetchConversationResourceBatch } from "@/api/agentResources";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { sendMessageRequest } from "@/api/im";
import { StartTaskPage } from "@/pages/StartTaskPage/StartTaskPage";
import type { IMConversation } from "@/models/conversations";

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
  } = {},
) {
  const create = vi.fn().mockResolvedValue(room);
  const open = vi.fn();
  const onCreateAgent = vi.fn();
  const requireAuth = vi.fn(() => options.authenticated ?? true);
  const controller = {
    ready: true,
    t: (key: string) => key,
    openAgentSettings: vi.fn(),
    agentViewProps: { authStatuses: { codex: { authenticated: options.authenticated ?? true } } },
    taskEntry: { create, open },
    sidebarProps: {
      t: (key: string) => key,
      locale: "zh",
      currentUserID: "user-admin",
      onRequireOpenCSGAuthentication: requireAuth,
      usersById: new Map(),
      workerAgentItems: [
        { id: "u-manager", name: "Manager", role: "manager", user_id: "manager", model_config: { model_id: model } },
        { id: "u-worker", name: "Worker", user_id: "worker", model_id: model, model_provider_id: options.provider },
      ],
      teams: [{ id: "team-1", title: "Team", lead_agent_id: "u-manager", member_agent_ids: ["u-worker"] }],
      onCreateAgent,
      onOpenCreateTeam: vi.fn(),
      onSelectAgent: vi.fn(),
    },
  } as unknown as WorkspaceController;
  render(
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
  return { create, open, onCreateAgent, requireAuth };
}

describe("StartTaskPage task preparation", () => {
  beforeEach(() => vi.mocked(sendMessageRequest).mockReset());
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
  await user.click(await screen.findByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: "addAttachmentResource" }));
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
