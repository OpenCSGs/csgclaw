import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorkspacePathActions } from "./WorkspacePathActions";

const api = vi.hoisted(() => ({
  fetchWorkspaceLocation: vi.fn(),
  openWorkspaceLocation: vi.fn(),
}));

vi.mock("@/api/chatFiles", () => api);

function renderActions(location?: { absolute_path: string; directory: boolean; open_targets: string[] }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const props = {
    agentID: "agent-1",
    roomID: "room-1",
    path: "",
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
  api.fetchWorkspaceLocation.mockReset();
  api.openWorkspaceLocation.mockReset();
});

test("loads workspace path actions only when opened and reports a path-specific error", async () => {
  const user = userEvent.setup();
  api.fetchWorkspaceLocation.mockRejectedValue(new Error("request failed"));
  renderActions();

  expect(api.fetchWorkspaceLocation).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "chatOpenPath" }));
  expect(await screen.findByText("chatWorkspaceLocationFailed")).toBeInTheDocument();
  expect(screen.queryByText("attachmentPreviewFailed")).not.toBeInTheDocument();
});

test("does not keep a previous query error visible once workspace location is supplied", async () => {
  const user = userEvent.setup();
  api.fetchWorkspaceLocation.mockRejectedValue(new Error("request failed"));
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
