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

function setup() {
  const onArchive = vi.fn();
  render(
    <MemoryRouter>
      <AgentView
        item={worker}
        t={createTranslator("en")}
        onDelete={onArchive}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onRecreate={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
      />
    </MemoryRouter>,
  );
  return { user: userEvent.setup(), onArchive };
}

beforeEach(() => {
  vi.mocked(fetchAgentArchiveImpact).mockReset().mockResolvedValue(impact);
});

it.each(["escape", "cancel"])("shows each archive effect and returns focus to More after %s", async (action) => {
  const { user, onArchive } = setup();
  const more = screen.getByRole("button", { name: "More" });
  more.focus();
  await user.keyboard("{Enter}{End}{Enter}");
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
