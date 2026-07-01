import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { useWorkspaceShellController } from "@/hooks/workspace/useWorkspaceShellController";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { WorkspaceTab } from "@/models/routing";
import { HUB_NEW_BADGE_SEEN_STORAGE_KEY } from "@/shared/storage/keys";

const rooms: IMConversation[] = [
  {
    id: "room-1",
    is_direct: false,
    members: [],
    messages: [],
    title: "Room 1",
  },
];

const t: TranslateFn = (key) => key;
const selectTasks = vi.fn();
const setIsSidebarCollapsed = vi.fn();

function ShellHarness() {
  const [workspaceTab, setWorkspaceTab] = useState<WorkspaceTab>(WorkspaceTabs.messages);
  const selectConversation = vi.fn();
  const shell = useWorkspaceShellController({
    activeConversationId: "room-1",
    activePane: { type: WorkspacePaneTypes.conversation, id: "room-1" },
    collapsedWorkspaceGroups: {},
    isSidebarCollapsed: false,
    locale: "en",
    navigatePane: vi.fn(),
    rooms,
    selectComputer: vi.fn(),
    selectConversation,
    selectHub: vi.fn(),
    selectTasks,
    setCollapsedWorkspaceGroups: vi.fn(),
    setIsSidebarCollapsed,
    setWorkspaceTab,
    t,
    theme: "dark",
    workspaceTab,
  });

  return (
    <>
      <div data-testid="workspace-tab">{shell.workspaceTab}</div>
      <div data-testid="hub-new-badge">{String(shell.showHubNewBadge)}</div>
      <button type="button" onClick={() => shell.selectWorkspaceTab(WorkspaceTabs.threads)}>
        Threads
      </button>
      <button type="button" onClick={() => shell.selectWorkspaceTab(WorkspaceTabs.messages)}>
        Messages
      </button>
      <button type="button" onClick={() => shell.selectWorkspaceTab(WorkspaceTabs.tasks)}>
        Tasks
      </button>
    </>
  );
}

describe("useWorkspaceShellController", () => {
  afterEach(() => {
    window.localStorage.clear();
    selectTasks.mockReset();
    setIsSidebarCollapsed.mockReset();
  });

  it("keeps the explicit Threads tab active on room routes", async () => {
    render(<ShellHarness />);

    fireEvent.click(screen.getByRole("button", { name: "Threads" }));

    await waitFor(() => {
      expect(screen.getByTestId("workspace-tab")).toHaveTextContent(WorkspaceTabs.threads);
    });

    fireEvent.click(screen.getByRole("button", { name: "Messages" }));

    await waitFor(() => {
      expect(screen.getByTestId("workspace-tab")).toHaveTextContent(WorkspaceTabs.messages);
    });
  });

  it("navigates to the tasks pane when requested", async () => {
    render(<ShellHarness />);

    fireEvent.click(screen.getByRole("button", { name: "Tasks" }));

    await waitFor(() => {
      expect(selectTasks).toHaveBeenCalledTimes(1);
    });
  });

  it("expands the sidebar before selecting a workspace tab", async () => {
    render(<ShellHarness />);

    fireEvent.click(screen.getByRole("button", { name: "Messages" }));

    await waitFor(() => {
      expect(setIsSidebarCollapsed).toHaveBeenCalledWith(false);
    });
  });

  it("marks the hub badge as seen after selecting the hub tab", async () => {
    function HubShellHarness() {
      const [workspaceTab, setWorkspaceTab] = useState<WorkspaceTab>(WorkspaceTabs.messages);
      const shell = useWorkspaceShellController({
        activeConversationId: "room-1",
        activePane: { type: WorkspacePaneTypes.conversation, id: "room-1" },
        collapsedWorkspaceGroups: {},
        isSidebarCollapsed: false,
        locale: "en",
        navigatePane: vi.fn(),
        rooms,
        selectComputer: vi.fn(),
        selectConversation: vi.fn(),
        selectHub: vi.fn(),
        selectTasks: vi.fn(),
        setCollapsedWorkspaceGroups: vi.fn(),
        setIsSidebarCollapsed: vi.fn(),
        setWorkspaceTab,
        t,
        theme: "dark",
        workspaceTab,
      });

      return (
        <>
          <div data-testid="hub-new-badge">{String(shell.showHubNewBadge)}</div>
          <button type="button" onClick={() => shell.selectWorkspaceTab(WorkspaceTabs.hub)}>
            Hub
          </button>
        </>
      );
    }

    render(<HubShellHarness />);

    expect(screen.getByTestId("hub-new-badge")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: "Hub" }));

    await waitFor(() => {
      expect(screen.getByTestId("hub-new-badge")).toHaveTextContent("false");
    });
    expect(window.localStorage.getItem(HUB_NEW_BADGE_SEEN_STORAGE_KEY)).toBe("seen");
  });
});
