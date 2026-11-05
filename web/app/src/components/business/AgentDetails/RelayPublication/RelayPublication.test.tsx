import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchRelayPublication, publishRelayAgent, unpublishRelayAgent } from "@/api/relay";
import { useRelayPublication } from "@/hooks/workspace/useRelayPublication";
import type { TranslateFn } from "@/models/conversations";
import { Button, DropdownMenuRoot, DropdownMenuTrigger, DropdownMenuContent } from "@/components/ui";
import { RelayPublication } from "./RelayPublication";

vi.mock("@/api/relay", () => ({
  fetchRelayPublication: vi.fn(),
  publishRelayAgent: vi.fn(),
  unpublishRelayAgent: vi.fn(),
}));
const endpoint = "http://127.0.0.1:28080";
const unpublished = { configured: true, published: false, endpoint };
const online = { ...unpublished, published: true, agent: { id: "agt_test", status: "online", busy: false } };
function Harness() {
  return (
    <RelayPublication
      controller={useRelayPublication("agent-alpha")}
      t={((key: string) => key) as TranslateFn}
      renderAction={(action) => (
        <DropdownMenuRoot>
          <DropdownMenuTrigger asChild>
            <Button>More</Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>{action}</DropdownMenuContent>
        </DropdownMenuRoot>
      )}
    />
  );
}
function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchRelayPublication).mockResolvedValue(unpublished);
  vi.mocked(publishRelayAgent).mockResolvedValue(online);
  vi.mocked(unpublishRelayAgent).mockResolvedValue(unpublished);
});
describe("Relay publication", () => {
  it("publishes only after confirmation and shows online state", async () => {
    const user = setup();
    expect(screen.queryByRole("button", { name: "relayPublish" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayPublish" })).toBeEnabled());
    await user.click(screen.getByRole("menuitem", { name: "relayPublish" }));
    await screen.findByRole("dialog");
    expect(publishRelayAgent).not.toHaveBeenCalled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "relayPublish" }));
    await screen.findByText("relayOnline");
    expect(publishRelayAgent).toHaveBeenCalledWith("agent-alpha");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("uses the current login without requesting connection settings", async () => {
    vi.mocked(fetchRelayPublication).mockResolvedValue({ ...unpublished, configured: false });
    const user = setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayPublish" })).toBeEnabled());
    await user.click(screen.getByRole("menuitem", { name: "relayPublish" }));
    await screen.findByRole("dialog");
    expect(publishRelayAgent).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "relayPublish" }));
    await waitFor(() => expect(publishRelayAgent).toHaveBeenCalledWith("agent-alpha"));
    expect(screen.queryByLabelText("relayToken")).not.toBeInTheDocument();
  });
  it("unpublishes an online agent from the management dialog", async () => {
    vi.mocked(fetchRelayPublication).mockResolvedValue(online);
    const user = setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayUnpublish" })).toBeEnabled());
    expect(screen.queryByRole("menuitem", { name: "relayPublish" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "relayUnpublish" }));
    expect(screen.getByText("agt_test")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "relayUnpublish" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(unpublishRelayAgent).toHaveBeenCalledWith("agent-alpha");
  });
  it("shows a retry dialog after publication fails", async () => {
    vi.mocked(publishRelayAgent).mockRejectedValue(new Error("Connection failed"));
    const user = setup();
    expect(screen.queryByRole("button", { name: "relayPublish" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayPublish" })).toBeEnabled());
    await user.click(screen.getByRole("menuitem", { name: "relayPublish" }));
    await screen.findByRole("dialog");
    expect(publishRelayAgent).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "relayPublish" }));
    await waitFor(() => expect(screen.getAllByRole("alert").length).toBeGreaterThan(0));
    expect(screen.queryByLabelText("relayToken")).not.toBeInTheDocument();
    await user.click(screen.getByText("cancel", { selector: "button span" }));
    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(screen.getByRole("menuitem", { name: "relayPublish" }));
    await screen.findByRole("dialog");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(publishRelayAgent).toHaveBeenCalledTimes(1);
  });
});

it.each([{ publishing: true }, { pending: true }, { removing: true }])(
  "allows cancelling or retrying cleanup of an incomplete publication: %o",
  async (state) => {
    vi.mocked(fetchRelayPublication).mockResolvedValue({ ...unpublished, ...state, can_unpublish: true });
    const user = setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayCancelPublication" })).toBeEnabled());
    await user.click(screen.getByRole("menuitem", { name: "relayCancelPublication" }));
    expect(screen.queryByRole("button", { name: "relayPublish" })).not.toBeInTheDocument();
    vi.mocked(unpublishRelayAgent).mockRejectedValueOnce(new Error("Cleanup failed"));
    await user.click(screen.getByRole("button", { name: "relayCancelPublication" }));
    await waitFor(() => expect(screen.getAllByRole("alert").length).toBeGreaterThan(0));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "relayCancelPublication" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(unpublishRelayAgent).toHaveBeenCalledTimes(2);
    expect(publishRelayAgent).not.toHaveBeenCalled();
  },
);
