import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchMessagesRequest } from "@/api/im";
import { useMissingConversationRoute } from "@/hooks/workspace/useMissingConversationRoute";

vi.mock("@/api/im", () => ({ fetchMessagesRequest: vi.fn() }));

const roomMissing = { status: 404, code: "room_not_found" };

beforeEach(() => vi.mocked(fetchMessagesRequest).mockReset());

it("keeps the route on a network error and rechecks after bootstrap refresh", async () => {
  const fetchMessages = vi
    .mocked(fetchMessagesRequest)
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockRejectedValueOnce(roomMissing);
  const onMissing = vi.fn();
  const { rerender } = renderHook(({ updatedAt }) => useMissingConversationRoute("deleted", updatedAt, onMissing), {
    initialProps: { updatedAt: 0 },
  });

  expect(fetchMessages).not.toHaveBeenCalled();
  await act(async () => rerender({ updatedAt: 1 }));
  expect(fetchMessages).toHaveBeenCalledWith("deleted");
  expect(onMissing).not.toHaveBeenCalled();

  rerender({ updatedAt: 2 });
  await waitFor(() => expect(onMissing).toHaveBeenCalledExactlyOnceWith("deleted"));
});

it("ignores a late missing-room result after switching routes", async () => {
  let rejectProbe!: (error: unknown) => void;
  vi.mocked(fetchMessagesRequest).mockImplementationOnce(
    () =>
      new Promise((_, reject) => {
        rejectProbe = reject;
      }),
  );
  const onMissing = vi.fn();
  const { rerender } = renderHook<void, { roomID?: string }>(
    ({ roomID }) => useMissingConversationRoute(roomID, 1, onMissing),
    { initialProps: { roomID: "deleted" } },
  );
  rerender({ roomID: undefined });
  await act(async () => rejectProbe(roomMissing));
  expect(onMissing).not.toHaveBeenCalled();
});

it("keeps a valid route when the conversation is absent only from the cache", async () => {
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([]);
  const onMissing = vi.fn();
  await act(async () => {
    renderHook(() => useMissingConversationRoute("valid", 1, onMissing));
  });
  expect(onMissing).not.toHaveBeenCalled();
});
