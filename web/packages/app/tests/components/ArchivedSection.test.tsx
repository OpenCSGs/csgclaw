import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchTaskConversations } from "@/api/taskConversations";
import { fetchArchivedAgents } from "@/api/agents";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { ArchivedSection } from "@/pages/SettingsPage/sections/ArchivedSection";
import { createTranslator } from "@/shared/i18n";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/taskConversations", () => ({ fetchTaskConversations: vi.fn() }));
vi.mock("@/api/agents", () => ({ fetchArchivedAgents: vi.fn(), restoreAgent: vi.fn(), deleteArchivedAgent: vi.fn() }));
const t = createTranslator("zh");

beforeEach(() => {
  window.localStorage.clear();
  vi.clearAllMocks();
  vi.mocked(fetchArchivedAgents).mockResolvedValue([{ id: "agent-old", name: "旧助手", avatar: "avatar/3D-1.png" }]);
});

function setup(initialTotal: number) {
  let total = initialTotal;
  const { client, wrapper } = createQueryWrapper();
  vi.mocked(fetchTaskConversations).mockImplementation(async (search) => ({
    total: search ? Math.min(total, 1) : total,
    has_more: !search && total > 1,
    rooms: total ? [{ id: "archived-room", title: "Review", members: [], messages: [], is_direct: false }] : [],
  }));
  const controller = {
    ready: true,
    t,
    sidebarProps: { t, currentUserID: "user", usersById: new Map(), onSelectConversation: vi.fn() },
    taskEntry: {
      busyID: "",
      isWorking: () => false,
      patch: vi.fn(async () => {
        total -= 1;
        await client.invalidateQueries({ queryKey: ["task-conversations"] });
      }),
    },
  } as unknown as WorkspaceController;
  render(
    <WorkspaceControllerProvider controller={controller}>
      <ArchivedSection />
    </WorkspaceControllerProvider>,
    { wrapper },
  );
  return userEvent.setup();
}

it("uses the full archive total across pagination and search, and refreshes it after restoration", async () => {
  const user = setup(51);
  expect(await screen.findByText("51 条")).toBeVisible();
  await user.type(screen.getByRole("textbox", { name: "搜索任务标题和历史消息" }), "Review");
  await waitFor(() => expect(fetchTaskConversations).toHaveBeenCalledWith("Review", true, 0));
  expect(screen.getByText("51 条")).toBeVisible();
  await user.click(await screen.findByRole("button", { name: /Review.*更多|更多.*Review/ }));
  await user.click(screen.getByRole("menuitem", { name: "取消归档" }));
  expect(await screen.findByText("50 条")).toBeVisible();
});

it("matches archive copy and lets the user restore confirmation prompts", async () => {
  const user = setup(0);
  expect(await screen.findByText("0 条")).toBeVisible();
  expect(screen.getByText(t("archivedDescription"))).toBeVisible();
  expect(screen.getByText("还没有归档的会话。")).toBeVisible();
  const toggle = screen.getByRole("switch", { name: "归档前先确认" });
  expect(toggle).toBeChecked();
  await user.click(toggle);
  expect(toggle).not.toBeChecked();
  await user.click(toggle);
  expect(toggle).toBeChecked();
});

it("switches between archived conversations and agents with the matching count and avatar", async () => {
  const user = setup(2);
  expect(await screen.findByText("2 条")).toBeVisible();
  await user.click(screen.getByText("智能体", { selector: "button" }));
  expect(await screen.findByText("旧助手")).toBeVisible();
  expect(await screen.findByText("1 条")).toBeVisible();
  expect(screen.getByText(t("archivedAgentsHint"))).toBeVisible();
  expect(document.querySelector("img")).toHaveAttribute("src", "avatar/3D-1.png");
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  await user.click(screen.getByText("会话与空间", { selector: "button" }));
  expect(await screen.findByText("2 条")).toBeVisible();
  expect(screen.getByRole("switch", { name: "归档前先确认" })).toBeVisible();
});
