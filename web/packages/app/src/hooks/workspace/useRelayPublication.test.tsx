import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import { fetchRelayPublication, publishRelayAgent, type RelayPublicationStatus } from "@/api/relay";
import { useRelayPublication } from "./useRelayPublication";

vi.mock("@/api/relay", () => ({
  fetchRelayPublication: vi.fn(),
  publishRelayAgent: vi.fn(),
  unpublishRelayAgent: vi.fn(),
}));

it("keeps a pending publication attached to its original agent after navigation", async () => {
  const offline: RelayPublicationStatus = { configured: true, endpoint: "http://relay.test", published: false };
  const online: RelayPublicationStatus = {
    ...offline,
    published: true,
    agent: { id: "agt_alpha", status: "online", busy: false },
  };
  vi.mocked(fetchRelayPublication).mockResolvedValue(offline);
  let complete!: (data: RelayPublicationStatus) => void;
  vi.mocked(publishRelayAgent).mockReturnValue(
    new Promise((resolve) => {
      complete = resolve;
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result, rerender } = renderHook(({ id }) => useRelayPublication(id), {
    initialProps: { id: "alpha" },
    wrapper,
  });
  await waitFor(() => expect(result.current.status.isSuccess).toBe(true));
  act(() => result.current.publish.mutate());
  await waitFor(() => expect(publishRelayAgent).toHaveBeenCalledWith("alpha"));
  rerender({ id: "beta" });
  await waitFor(() => expect(result.current.status.isSuccess).toBe(true));
  await act(async () => {
    complete(online);
  });
  await waitFor(() => expect(client.getQueryData(["relay-publication", "alpha"])).toEqual(online));
  expect(client.getQueryData(["relay-publication", "beta"])).toEqual(offline);
  expect(result.current.status.data?.published).toBe(false);
});
