import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { fetchAgentArchiveImpact } from "@/api/agents";
import { createTranslator } from "@/shared/i18n";
import { AgentView } from "./AgentView";

vi.mock("@/api/agents", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/agents")>()),
  fetchAgentArchiveImpact: vi.fn(),
}));

const worker = { id: "agent-review", name: "Review", role: "worker", runtime_name: "codex" };
const impact = { teams: 2, spaces: 3, direct_chats: 4, rooms: 7, scheduled_tasks: 5 };

function setup(item = worker) {
  const onArchive = vi.fn();
  const onPermanentDelete = vi.fn().mockResolvedValue(undefined);
  render(
    <MemoryRouter>
      <AgentView
        item={item}
        t={createTranslator("en")}
        onDelete={onArchive}
        onPermanentDelete={onPermanentDelete}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onRecreate={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
      />
    </MemoryRouter>,
  );
  return { user: userEvent.setup(), onArchive, onPermanentDelete };
}

beforeEach(() => {
  vi.mocked(fetchAgentArchiveImpact).mockReset().mockResolvedValue(impact);
});

it.each(["escape", "cancel"])("shows each archive effect and returns focus to More after %s", async (action) => {
  const { user, onArchive } = setup();
  const more = screen.getByRole("button", { name: "More" });
  more.focus();
  await user.keyboard("{Enter}{End}{ArrowUp}{Enter}");
  const dialog = await screen.findByRole("dialog", { name: 'Archive "Review"?' });
  expect(await within(dialog).findByText(/Pause 5 schedules/)).toBeVisible();
  expect(dialog).toHaveTextContent("2 team rosters");
  expect(dialog).toHaveTextContent("3 collaboration space rosters");
  expect(dialog).toHaveTextContent("4 direct conversations");
  expect(dialog).toHaveTextContent("Interrupt active tasks and conversations");
  expect(dialog).toHaveTextContent("Settings → Data → Archived → Agents");
  expect(dialog).toHaveTextContent("keep space messages and data");
  if (action === "escape") await user.keyboard("{Escape}");
  else await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(more).toHaveFocus());
  expect(onArchive).not.toHaveBeenCalled();
});

it("requires a successful impact check and allows retry before confirming", async () => {
  vi.mocked(fetchAgentArchiveImpact).mockRejectedValueOnce(new Error("offline"));
  const { user, onArchive } = setup();
  await user.click(screen.getByRole("button", { name: "More" }));
  await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
  const dialog = screen.getByRole("dialog");
  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Unable to load the archive impact");
  const confirm = within(dialog).getByRole("button", { name: "Confirm archive" });
  expect(confirm).toBeDisabled();
  expect(onArchive).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "Retry" }));
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(confirm);
  expect(onArchive).toHaveBeenCalledExactlyOnceWith(worker);
});

it("requires the exact name, keeps the dialog open on failure, and retries permanent deletion", async () => {
  const { user, onArchive, onPermanentDelete } = setup();
  let rejectFirstAttempt: (reason: unknown) => void = () => {};
  onPermanentDelete.mockImplementationOnce(
    () =>
      new Promise<void>((_, reject) => {
        rejectFirstAttempt = reject;
      }),
  );
  expect(screen.queryByRole("button", { name: "Delete agent" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "More" }));
  await user.click(screen.getByRole("menuitem", { name: "Delete agent" }));
  const dialog = screen.getByRole("dialog", { name: 'Permanently delete "Review"?' });
  const confirm = within(dialog).getByRole("button", { name: "Delete permanently" });
  const input = within(dialog).getByRole("textbox", { name: "Agent name" });
  expect(dialog).toHaveTextContent("Keep files in the working directory");
  expect(confirm).toBeDisabled();
  await user.type(input, "review");
  expect(confirm).toBeDisabled();
  await user.clear(input);
  await user.type(input, "Review");
  await user.click(confirm);
  expect(onPermanentDelete).toHaveBeenCalledExactlyOnceWith(worker, "Review");
  expect(confirm).toBeDisabled();
  await user.keyboard("{Escape}");
  expect(dialog).toBeVisible();
  rejectFirstAttempt({ code: "relay_unpublish_required", message: "published" });
  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Unpublish or cancel publication");
  expect(input).toHaveValue("Review");
  await user.click(confirm);
  await waitFor(() => expect(dialog).not.toBeInTheDocument());
  expect(onPermanentDelete).toHaveBeenCalledTimes(2);
  expect(onArchive).not.toHaveBeenCalled();
});

it("opens permanent deletion with the keyboard and returns focus to More on cancel", async () => {
  const { user, onPermanentDelete } = setup();
  const trigger = screen.getByRole("button", { name: "More" });
  trigger.focus();
  await user.keyboard("{Enter}{End}{Enter}");
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(onPermanentDelete).not.toHaveBeenCalled();
});

it("does not offer permanent deletion for the Manager", async () => {
  const { user } = setup({ ...worker, id: "agent-manager", name: "Manager", role: "manager" });
  const more = screen.queryByRole("button", { name: "More" });
  if (more) await user.click(more);
  expect(screen.queryByRole("menuitem", { name: "Delete agent" })).not.toBeInTheDocument();
});
