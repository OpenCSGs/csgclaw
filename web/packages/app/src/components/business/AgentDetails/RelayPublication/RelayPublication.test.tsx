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
      renderAction={(action) =>
        action ? (
          <DropdownMenuRoot>
            <DropdownMenuTrigger asChild>
              <Button>More</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>{action}</DropdownMenuContent>
          </DropdownMenuRoot>
        ) : null
      }
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
  return { user: userEvent.setup(), client };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchRelayPublication).mockResolvedValue(unpublished);
  vi.mocked(publishRelayAgent).mockResolvedValue(online);
  vi.mocked(unpublishRelayAgent).mockResolvedValue(unpublished);
});
describe("Relay publication", () => {
  it("hides the menu for an unpublished agent", async () => {
    const { client } = setup();
    await waitFor(() => expect(client.getQueryData(["relay-publication", "agent-alpha"])).toEqual(unpublished));
    expect(screen.queryByRole("button", { name: "More" })).not.toBeInTheDocument();
    expect(publishRelayAgent).not.toHaveBeenCalled();
  });
  it("unpublishes an online agent from the management dialog", async () => {
    vi.mocked(fetchRelayPublication).mockResolvedValue(online);
    const { user } = setup();
    await user.click(await screen.findByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayUnpublish" })).toBeEnabled());
    expect(screen.queryByRole("menuitem", { name: "relayPublish" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "relayUnpublish" }));
    expect(screen.getByText("agt_test")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "relayUnpublish" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(unpublishRelayAgent).toHaveBeenCalledWith("agent-alpha");
  });
});

it.each([{ publishing: true }, { pending: true }, { removing: true }])(
  "allows cancelling or retrying cleanup of an incomplete publication: %o",
  async (state) => {
    vi.mocked(fetchRelayPublication).mockResolvedValue({ ...unpublished, ...state, can_unpublish: true });
    const { user } = setup();
    await user.click(await screen.findByRole("button", { name: "More" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "relayCancelPublication" })).toBeEnabled());
    await user.click(screen.getByRole("menuitem", { name: "relayCancelPublication" }));
    expect(screen.queryByRole("button", { name: "relayPublish" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "retry" })).not.toBeInTheDocument();
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
