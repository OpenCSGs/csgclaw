import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TaskConversationList } from "@/components/business/TaskConversationList";
import { fetchTaskConversations } from "@/api/taskConversations";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import type { TaskEntryController } from "@/hooks/workspace/useTaskEntry";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/taskConversations", () => ({ fetchTaskConversations: vi.fn() }));
const room = { id: "r1", title: "Review", members: ["user-admin", "manager"], is_direct: true, messages: [] };
function Preference() {
  const [skip] = useArchiveConfirmation();
  return <output data-testid="preference">{String(skip)}</output>;
}
function setup(working = false, archived = false, compact = false) {
  vi.mocked(fetchTaskConversations).mockResolvedValue({ rooms: [room], total: 1, has_more: false });
  const patch = vi.fn().mockResolvedValue(undefined);
  const remove = vi.fn().mockResolvedValue(undefined);
  const entry = { patch, remove, isWorking: () => working, busyID: "" } as unknown as TaskEntryController;
  const usersById = new Map([{ id: "manager", name: "Manager", role: "worker" }].map((user) => [user.id, user]));
  render(
    <>
      <TaskConversationList
        archived={archived}
        compact={compact}
        entry={entry}
        currentUserID="user-admin"
        usersById={usersById}
        t={(key) => key}
        onSelect={vi.fn()}
      />
      <Preference />
    </>,
    { wrapper: createQueryWrapper().wrapper },
  );
  return { patch, remove };
}
describe("TaskConversationList archive management", () => {
  beforeEach(() => window.localStorage.clear());
  it("filters by the session directory instead of another task using the same Agent", async () => {
    const user = userEvent.setup();
    vi.mocked(fetchTaskConversations).mockResolvedValue({
      rooms: [
        { ...room, id: "first", title: "First project", configuration: { workspace_dir: "/projects/first" } },
        { ...room, id: "second", title: "Second project", configuration: { workspace_dir: "/projects/second" } },
      ],
      total: 2,
      has_more: false,
    });
    render(
      <TaskConversationList
        compact
        directories={[{ path: "/projects/first", agentIDs: ["manager"] }]}
        entry={{
          create: vi.fn().mockResolvedValue(room),
          patch: vi.fn().mockResolvedValue(undefined),
          remove: vi.fn().mockResolvedValue(undefined),
          open: vi.fn(),
          isWorking: () => false,
          busyID: "",
        }}
        currentUserID="user-admin"
        usersById={new Map()}
        t={(key) => key}
        onSelect={vi.fn()}
      />,
      { wrapper: createQueryWrapper().wrapper },
    );
    await screen.findByText("Second project");
    await user.click(screen.getByRole("combobox", { name: "iaSessionDirectoryFilter" }));
    await user.click(screen.getByRole("option", { name: /first/ }));
    expect(screen.getByText("First project")).toBeInTheDocument();
    expect(screen.queryByText("Second project")).not.toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "iaSessionDirectoryFilter" }));
    await user.click(screen.getByRole("option", { name: /second/ }));
    expect(screen.getByText("Second project")).toBeInTheDocument();
    expect(screen.queryByText("First project")).not.toBeInTheDocument();
  });
  it("offers pin and archive shortcuts without navigating the conversation", async () => {
    const user = userEvent.setup();
    const { patch } = setup(false, false, true);
    await user.click(await screen.findByRole("button", { name: "taskEntryPin · Review" }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith("r1", { pinned: true }));
    await user.click(screen.getByRole("button", { name: "taskEntryArchive · Review" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "cancel" }));
    expect(patch).toHaveBeenCalledTimes(1);
  });
  it("disables the archive shortcut while the task is running", async () => {
    setup(true, false, true);
    expect(await screen.findByRole("button", { name: "taskEntryArchive · Review" })).toBeDisabled();
  });
  it("updates the shared confirmation preference only after a successful archive", async () => {
    const user = userEvent.setup();
    const { patch } = setup();
    await user.click(await screen.findByRole("button", { name: "taskEntryMore · Review" }));
    await user.click(screen.getByRole("menuitem", { name: "taskEntryArchive" }));
    await user.click(screen.getByRole("checkbox", { name: "taskEntryDontAsk" }));
    await user.click(screen.getByRole("button", { name: "cancel" }));
    expect(screen.getByTestId("preference")).toHaveTextContent("false");
    expect(patch).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "taskEntryMore · Review" }));
    await user.click(screen.getByRole("menuitem", { name: "taskEntryArchive" }));
    await user.click(screen.getByRole("checkbox", { name: "taskEntryDontAsk" }));
    await user.click(screen.getByRole("button", { name: "confirm" }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith("r1", { archived: true }));
    await waitFor(() => expect(screen.getByTestId("preference")).toHaveTextContent("true"));
  });
  it("keeps archive errors visible in the dialog and does not save the preference on failure", async () => {
    const user = userEvent.setup();
    const { patch } = setup();
    patch.mockRejectedValueOnce(new Error("Save failed"));
    await user.click(await screen.findByRole("button", { name: "taskEntryMore · Review" }));
    await user.click(screen.getByRole("menuitem", { name: "taskEntryArchive" }));
    await user.click(screen.getByRole("checkbox", { name: "taskEntryDontAsk" }));
    await user.click(screen.getByRole("button", { name: "confirm" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Save failed");
    expect(screen.getByRole("dialog")).toContainElement(screen.getByRole("alert"));
    expect(screen.getByTestId("preference")).toHaveTextContent("false");
  });

  it("disables running conversation deletion and archive", async () => {
    const user = userEvent.setup();
    const { remove } = setup(true, true);
    await user.click(await screen.findByRole("button", { name: "taskEntryMore · Review" }));
    const item = screen.getByRole("menuitem", { name: "taskEntryDelete" });
    expect(item).toHaveAttribute("data-disabled");
    fireEvent.click(item);
    expect(remove).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
