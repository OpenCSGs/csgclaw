import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchArchivedAgents, restoreAgent, deleteArchivedAgent } from "@/api/agents";
import { createTranslator } from "@/shared/i18n";
import { ArchivedAgents } from "./ArchivedAgents";

vi.mock("@/api/agents", () => ({ fetchArchivedAgents: vi.fn(), restoreAgent: vi.fn(), deleteArchivedAgent: vi.fn() }));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchArchivedAgents).mockResolvedValue([
    { id: "agent-review", name: "Review", archived_at: "2026-10-28T03:00:00Z", archived_by: "Jared" },
  ]);
  vi.mocked(restoreAgent).mockResolvedValue({ id: "agent-review", name: "Review" });
  vi.mocked(deleteArchivedAgent).mockResolvedValue(undefined);
});
function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ArchivedAgents
        t={createTranslator("en")}
        usersById={new Map([["user-review", { id: "user-review", avatar: "avatar/3D-1.png" }]])}
      />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}
it("requires the exact name before permanently deleting an archived agent", async () => {
  const user = setup();
  await screen.findByText("Review");
  expect(screen.getByText(/By Jared/)).toBeVisible();
  expect(document.querySelector("img")).toHaveAttribute("src", "avatar/3D-1.png");
  await user.click(await screen.findByRole("button", { name: "Delete permanently Review" }));
  const dialog = screen.getByRole("dialog");
  expect(dialog).toHaveTextContent("Keep collaboration space messages and attachments");
  expect(dialog).toHaveTextContent("Keep schedules stopped");
  expect(dialog).toHaveTextContent("private skill files, variables and secrets");
  expect(dialog).toHaveTextContent("skill, connector, channel and knowledge base bindings");
  expect(dialog).toHaveTextContent("keep shared resources and other agents unchanged");
  expect(dialog).toHaveTextContent("Remove team and collaboration space memberships");
  expect(dialog).toHaveTextContent("Keep files in the working directory");
  expect(dialog).toHaveTextContent("Keep a minimal snapshot");
  expect(dialog).toHaveTextContent('Type "Review" to confirm');
  const confirm = within(dialog).getByRole("button", { name: "Delete permanently" });
  expect(confirm).toBeDisabled();
  await user.type(within(dialog).getByRole("textbox", { name: "Agent name" }), "review");
  expect(confirm).toBeDisabled();
  await user.clear(within(dialog).getByRole("textbox", { name: "Agent name" }));
  await user.type(within(dialog).getByRole("textbox", { name: "Agent name" }), "Review");
  await user.click(confirm);
  await waitFor(() => expect(deleteArchivedAgent).toHaveBeenCalledWith("agent-review", "Review"));
  expect(restoreAgent).not.toHaveBeenCalled();
});
it("restores an agent only after confirmation and retains errors for retry", async () => {
  const user = setup();
  vi.mocked(restoreAgent).mockRejectedValueOnce({ message: "Runtime unavailable" });
  await user.click(await screen.findByRole("button", { name: "Restore agent" }));
  expect(restoreAgent).not.toHaveBeenCalled();
  const dialog = screen.getByRole("dialog");
  expect(dialog).toHaveTextContent("schedules stay paused");
  expect(dialog).toHaveTextContent("surviving teams and collaboration spaces");
  expect(dialog).toHaveTextContent("original direct conversation archive states");
  await user.click(within(dialog).getByRole("button", { name: "Restore agent" }));
  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Runtime unavailable");
  await user.click(within(dialog).getByRole("button", { name: "Restore agent" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(restoreAgent).toHaveBeenCalledTimes(2);
});

it("does not attribute legacy archives to the current user", async () => {
  vi.mocked(fetchArchivedAgents).mockResolvedValue([{ id: "agent-review", name: "Review" }]);
  setup();
  expect(await screen.findByText(/Operator not recorded/)).toBeVisible();
  expect(screen.queryByText(/By Jared/)).not.toBeInTheDocument();
});
