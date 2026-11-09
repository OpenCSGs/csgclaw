import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import { ConversationView } from "@/components/business/ConversationWorkspace";
import { RoomTypes, type IMConversation, type IMData } from "@/models/conversations";
import { paneFromLocation, pathForPane, WorkspacePaneTypes } from "@/models/routing";
import { createTranslator } from "@/shared/i18n";
import { createQueryWrapper } from "../helpers/queryClient";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/shared/realtime/imEvents", () => ({ subscribeIMEvents: vi.fn(() => () => {}) }));
vi.mock("@/api/tasks", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/tasks")>()),
  fetchRoomTasks: vi.fn(async () => []),
  fetchRoomTaskEvents: vi.fn(async () => []),
}));

const t = createTranslator("en");
const room: IMConversation = {
  id: "room-a",
  title: "Gomoku",
  type: RoomTypes.onDemand,
  is_direct: false,
  members: ["u-admin", "u-manager"],
  manager_id: "u-manager",
  messages: [],
};
const data: IMData = {
  current_user_id: "u-admin",
  rooms: [room, { ...room, id: "room-b", title: "Other room" }],
  users: [
    { id: "u-admin", name: "Admin", role: "admin" },
    { id: "u-manager", name: "Manager", role: "manager" },
  ],
};
const agents = [{ id: "u-manager", name: "Manager", role: "manager", status: "running" }];
const noop = () => {};
const guard = openCSGAuthGuardStub();

function Harness() {
  const navigate = useNavigate();
  const activePane = paneFromLocation(useLocation().pathname);
  const activeConversationId =
    activePane.type === WorkspacePaneTypes.conversation ? (activePane.id ?? room.id) : room.id;
  const controller = useConversationController({
    activeConversationId,
    activePane,
    agents,
    authBusyProvider: "",
    authStatuses: {},
    data,
    locale: "en",
    managerProfile: null,
    managerProfileIncomplete: false,
    hasObservedWorkLease: () => false,
    messageActionBusy: "",
    messageActionFeedback: { key: "", message: "" },
    messageListActive: activePane.type === WorkspacePaneTypes.conversation,
    navigatePane: (pane) => navigate(pathForPane(pane, data.rooms)),
    onMessageAction: noop,
    onProviderLogin: async () => {},
    openCSGAuthGuard: guard,
    rooms: data.rooms,
    selectComputer: noop,
    selectConversation: noop,
    setActiveConversationId: noop,
    setBootstrapData: noop,
    setShowToolCalls: noop,
    showToolCalls: false,
    t,
    theme: "light",
    workingParticipantsForRoom: () => [],
  });
  return (
    <>
      <nav>
        <button onClick={() => navigate("/agents/u-manager")}>Agent page</button>
        <button onClick={() => navigate("/rooms/room-b")}>Other room</button>
        <button onClick={() => navigate("/rooms/room-a")}>Return to room</button>
      </nav>
      {activePane.type === WorkspacePaneTypes.conversation ? (
        <main className="chat-panel">
          <ConversationView {...controller.conversationViewProps} agents={agents} onPreviewUser={noop} />
        </main>
      ) : (
        <p>Agent details</p>
      )}
    </>
  );
}

function renderWorkspace() {
  return render(
    <MemoryRouter initialEntries={["/rooms/room-a"]}>
      <Harness />
    </MemoryRouter>,
    {
      wrapper: createQueryWrapper().wrapper,
    },
  );
}

describe("conversation panel navigation", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 1200, 900));
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it.each(["Agent page", "Other room"])(
    "restores members and allows switching back to tasks after visiting %s",
    async (destination) => {
      const user = userEvent.setup();
      renderWorkspace();
      await user.click(screen.getByRole("button", { name: t("roomTasksOpen") }));
      expect(screen.getByRole("tabpanel", { name: t("roomTasksOpen") })).toBeVisible();
      await user.click(screen.getByRole("button", { name: t("memberManagement") }));
      expect(screen.getByRole("complementary", { name: t("memberManagementTitle") })).toBeVisible();
      expect(screen.queryByRole("complementary", { name: t("chatSidePanel") })).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: destination }));
      await user.click(screen.getByRole("button", { name: "Return to room" }));
      expect(screen.getByRole("complementary", { name: t("memberManagementTitle") })).toBeVisible();
      expect(screen.queryByRole("complementary", { name: t("chatSidePanel") })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: t("memberManagement") })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("button", { name: t("roomTasksOpen") })).toHaveAttribute("aria-pressed", "false");
      await user.click(screen.getByRole("button", { name: t("roomTasksOpen") }));
      expect(screen.queryByRole("complementary", { name: t("memberManagementTitle") })).not.toBeInTheDocument();
      expect(screen.getByRole("tabpanel", { name: t("roomTasksOpen") })).toBeVisible();
      expect(screen.getAllByRole("tab", { name: t("roomTasksOpen") })).toHaveLength(1);
      await user.click(screen.getByRole("button", { name: destination }));
      await user.click(screen.getByRole("button", { name: "Return to room" }));
      expect(screen.getByRole("tabpanel", { name: t("roomTasksOpen") })).toBeVisible();
      expect(screen.getByRole("button", { name: t("memberManagement") })).toHaveAttribute("aria-pressed", "false");
    },
  );

  it.each([false, true])(
    "restores members after remount and remembers explicit close with prior tasks: %s",
    async (priorTasks) => {
      const user = userEvent.setup();
      const view = renderWorkspace();
      if (priorTasks) await user.click(screen.getByRole("button", { name: t("roomTasksOpen") }));
      await user.click(screen.getByRole("button", { name: t("memberManagement") }));
      view.unmount();
      const restored = renderWorkspace();
      expect(screen.getByRole("complementary", { name: t("memberManagementTitle") })).toBeVisible();
      expect(screen.queryByRole("complementary", { name: t("chatSidePanel") })).not.toBeInTheDocument();
      await user.click(
        within(screen.getByRole("complementary", { name: t("memberManagementTitle") })).getByRole("button", {
          name: t("close"),
        }),
      );
      restored.unmount();
      renderWorkspace();
      expect(screen.queryByRole("complementary", { name: t("memberManagementTitle") })).not.toBeInTheDocument();
      if (priorTasks) expect(screen.getByRole("tabpanel", { name: t("roomTasksOpen") })).toBeVisible();
      else expect(screen.queryByRole("complementary", { name: t("chatSidePanel") })).not.toBeInTheDocument();
    },
  );
});
