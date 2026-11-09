import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorkspacePathActions } from "./WorkspacePathActions";

const api = vi.hoisted(() => ({
  fetchFileLocation: vi.fn(),
  openFileLocation: vi.fn(),
}));

vi.mock("@/api/chatFiles", () => api);

function renderActions(location?: {
  absolute_path: string;
  directory: boolean;
  open_targets: { id: string; label?: string; default?: boolean }[];
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const props = {
    roomID: "room-1",
    reference: { source: "workspace" as const, agent_id: "agent-1", path: "", name: "", label: "" },
    location,
    t: (key: string) => key,
  };
  const view = render(
    <QueryClientProvider client={queryClient}>
      <WorkspacePathActions {...props} />
    </QueryClientProvider>,
  );
  return { ...view, queryClient, props };
}

beforeEach(() => {
  api.fetchFileLocation.mockReset();
  api.openFileLocation.mockReset();
});

test("loads workspace path actions only when opened and reports a path-specific error", async () => {
  const user = userEvent.setup();
  api.fetchFileLocation.mockRejectedValue(new Error("request failed"));
  renderActions();

  expect(api.fetchFileLocation).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "chatOpenPath" }));
  expect(await screen.findByText("chatWorkspaceLocationFailed")).toBeInTheDocument();
  expect(screen.queryByText("attachmentPreviewFailed")).not.toBeInTheDocument();
});

test("does not keep a previous query error visible once workspace location is supplied", async () => {
  const user = userEvent.setup();
  api.fetchFileLocation.mockRejectedValue(new Error("request failed"));
  const view = renderActions();

  await user.click(screen.getByRole("button", { name: "chatOpenPath" }));
  expect(await screen.findByText("chatWorkspaceLocationFailed")).toBeInTheDocument();

  view.rerender(
    <QueryClientProvider client={view.queryClient}>
      <WorkspacePathActions
        {...view.props}
        location={{ absolute_path: "/workspace", directory: true, open_targets: [] }}
      />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(screen.queryByText("chatWorkspaceLocationFailed")).not.toBeInTheDocument());
});

test("renders the file's associated apps and opens the selected app", async () => {
  const user = userEvent.setup();
  api.fetchFileLocation.mockResolvedValue({
    absolute_path: "/workspace/report.pdf",
    directory: false,
    open_targets: [{ id: "system" }, { id: "app:/Applications/Preview.app", label: "Preview", default: true }],
  });
  api.openFileLocation.mockResolvedValue(undefined);
  renderActions();
  await user.click(screen.getByRole("button", { name: "chatOpenPath" }));
  await user.click(await screen.findByRole("menuitem", { name: "Preview (chatOpenDefault)" }));
  expect(api.openFileLocation).toHaveBeenCalledWith(
    "room-1",
    expect.objectContaining({ source: "workspace" }),
    "app:/Applications/Preview.app",
    "",
  );
  expect(screen.queryByText("iTerm2")).not.toBeInTheDocument();
});

test("resolves an attachment capability and opens the attachment by ID", async () => {
  const user = userEvent.setup();
  api.fetchFileLocation.mockResolvedValue({
    absolute_path: "/assets/open/file/report.pdf",
    directory: false,
    open_targets: [{ id: "system" }],
  });
  api.openFileLocation.mockResolvedValue(undefined);
  const reference = {
    source: "attachment" as const,
    attachment_id: "generated-file",
    label: "report.pdf",
    name: "report.pdf",
  };
  render(
    <QueryClientProvider client={new QueryClient()}>
      <WorkspacePathActions roomID="room-1" reference={reference} token="capability" t={(key) => key} />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "chatOpenPath" }));
  await user.click(await screen.findByRole("menuitem", { name: "chatOpenSystem" }));
  expect(api.fetchFileLocation).toHaveBeenCalledWith("room-1", reference, "capability", expect.any(AbortSignal));
  expect(api.openFileLocation).toHaveBeenCalledWith("room-1", reference, "system", "capability");
});
