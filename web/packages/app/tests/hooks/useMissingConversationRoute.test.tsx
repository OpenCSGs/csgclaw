import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchMessagesRequest } from "@/api/im";
import { fetchWorkspaceBootstrapData } from "@/hooks/workspace/workspaceQueries";
import type { IMData } from "@/models/conversations";
import { useMissingConversationRoute } from "@/hooks/workspace/useMissingConversationRoute";

vi.mock("@/api/im", () => ({ fetchMessagesRequest: vi.fn() }));

vi.mock("@/hooks/workspace/workspaceQueries", () => ({ fetchWorkspaceBootstrapData: vi.fn() }));

const roomMissing = { status: 404, code: "room_not_found" };

beforeEach(() => {
  vi.mocked(fetchMessagesRequest).mockReset();
  vi.mocked(fetchWorkspaceBootstrapData).mockReset();
});

it("keeps the route on a network error and rechecks on an explicit retry", async () => {
  const fetchMessages = vi
    .mocked(fetchMessagesRequest)
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockRejectedValueOnce(roomMissing);
  const onMissing = vi.fn();
  const onFound = vi.fn();
  const { result } = renderHook(() => useMissingConversationRoute("deleted", onMissing, onFound));

  await act(async () => {});
  expect(fetchMessages).toHaveBeenCalledExactlyOnceWith("deleted");
  expect(onMissing).not.toHaveBeenCalled();

  act(() => result.current());
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
  const onFound = vi.fn();
  const { rerender } = renderHook<void, { roomID?: string }>(
    ({ roomID }) => useMissingConversationRoute(roomID, onMissing, onFound),
    { initialProps: { roomID: "deleted" } },
  );
  rerender({ roomID: undefined });
  await act(async () => rejectProbe(roomMissing));
  expect(onMissing).not.toHaveBeenCalled();
});

const recoveredRoom = { id: "valid", title: "New chat", members: ["user"], messages: [] };
const recoveredBootstrap: IMData = { rooms: [recoveredRoom], users: [], current_user_id: "user" };

it("recovers the room metadata after a successful transcript probe", async () => {
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([]);
  vi.mocked(fetchWorkspaceBootstrapData).mockResolvedValueOnce(recoveredBootstrap);
  const onMissing = vi.fn();
  const onFound = vi.fn();
  renderHook(() => useMissingConversationRoute("valid", onMissing, onFound));

  await waitFor(() => expect(onFound).toHaveBeenCalledExactlyOnceWith(recoveredRoom));
  expect(onMissing).not.toHaveBeenCalled();
});

it("discards recovered room data when the route changes during bootstrap refresh", async () => {
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([]);
  let completeBootstrap!: (value: IMData) => void;
  const fetchBootstrap = vi.mocked(fetchWorkspaceBootstrapData).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        completeBootstrap = resolve;
      }),
  );
  const onMissing = vi.fn();
  const onFound = vi.fn();
  const { rerender } = renderHook<void, { roomID?: string }>(
    ({ roomID }) => useMissingConversationRoute(roomID, onMissing, onFound),
    { initialProps: { roomID: "valid" } },
  );
  await waitFor(() => expect(fetchBootstrap).toHaveBeenCalledOnce());
  rerender({ roomID: undefined });
  await act(async () => completeBootstrap(recoveredBootstrap));
  expect(onFound).not.toHaveBeenCalled();
  expect(onMissing).not.toHaveBeenCalled();
});

it("does not restart recovery on unrelated renders or overlapping retries", async () => {
  const fetchMessages = vi.mocked(fetchMessagesRequest).mockResolvedValue([]);
  let completeBootstrap!: (value: IMData) => void;
  const bootstrap = new Promise<IMData>((resolve) => {
    completeBootstrap = resolve;
  });
  const fetchBootstrap = vi.mocked(fetchWorkspaceBootstrapData).mockReturnValue(bootstrap);
  const onMissing = vi.fn();
  const onFound = vi.fn();
  const { result, rerender } = renderHook(({ roomID }) => useMissingConversationRoute(roomID, onMissing, onFound), {
    initialProps: { roomID: "valid", cacheRevision: 0 },
  });

  await waitFor(() => expect(fetchBootstrap).toHaveBeenCalledOnce());
  for (let cacheRevision = 1; cacheRevision <= 5; cacheRevision++) {
    rerender({ roomID: "valid", cacheRevision });
    act(() => result.current());
  }
  await act(async () => completeBootstrap(recoveredBootstrap));
  expect(fetchMessages).toHaveBeenCalledExactlyOnceWith("valid");
  expect(fetchBootstrap).toHaveBeenCalledOnce();
  expect(onFound).toHaveBeenCalledExactlyOnceWith(recoveredRoom);
  expect(onMissing).not.toHaveBeenCalled();
});

it("retries once if the connection reopens during an unsuccessful probe", async () => {
  let rejectProbe!: (error: unknown) => void;
  const fetchMessages = vi
    .mocked(fetchMessagesRequest)
    .mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectProbe = reject;
        }),
    )
    .mockRejectedValueOnce(roomMissing);
  const onMissing = vi.fn();
  const onFound = vi.fn();
  const { result } = renderHook(() => useMissingConversationRoute("deleted", onMissing, onFound));

  act(() => {
    result.current();
    result.current();
  });
  expect(fetchMessages).toHaveBeenCalledOnce();
  await act(async () => rejectProbe(new TypeError("offline")));
  await waitFor(() => expect(onMissing).toHaveBeenCalledExactlyOnceWith("deleted"));
  expect(fetchMessages).toHaveBeenCalledTimes(2);
});

describe("automatic recovery retry", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("recovers from a temporary bootstrap failure without a reconnect", async () => {
    const fetchMessages = vi.mocked(fetchMessagesRequest).mockResolvedValue([]);
    const fetchBootstrap = vi
      .mocked(fetchWorkspaceBootstrapData)
      .mockRejectedValueOnce({ status: 503 })
      .mockResolvedValueOnce(recoveredBootstrap);
    const onMissing = vi.fn();
    const onFound = vi.fn();
    renderHook(() => useMissingConversationRoute("valid", onMissing, onFound));

    await act(async () => {});
    expect(fetchBootstrap).toHaveBeenCalledOnce();
    await act(() => vi.advanceTimersByTimeAsync(500));

    expect(fetchMessages).toHaveBeenCalledTimes(2);
    expect(fetchBootstrap).toHaveBeenCalledTimes(2);
    expect(onFound).toHaveBeenCalledExactlyOnceWith(recoveredRoom);
    expect(onMissing).not.toHaveBeenCalled();
  });

  it("stops automatic retries after a second temporary failure", async () => {
    const fetchMessages = vi.mocked(fetchMessagesRequest).mockRejectedValue({ status: 504 });
    const onMissing = vi.fn();
    const onFound = vi.fn();
    renderHook(() => useMissingConversationRoute("valid", onMissing, onFound));

    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(fetchMessages).toHaveBeenCalledTimes(2);
    expect(onMissing).not.toHaveBeenCalled();
    expect(onFound).not.toHaveBeenCalled();
  });

  it("cancels the retry when the current route changes", async () => {
    const fetchMessages = vi.mocked(fetchMessagesRequest).mockRejectedValue(new TypeError("offline"));
    const onMissing = vi.fn();
    const onFound = vi.fn();
    const { rerender } = renderHook<void, { roomID?: string }>(
      ({ roomID }) => useMissingConversationRoute(roomID, onMissing, onFound),
      { initialProps: { roomID: "valid" } },
    );

    await act(async () => {});
    rerender({ roomID: undefined });
    await act(() => vi.advanceTimersByTimeAsync(5000));

    expect(fetchMessages).toHaveBeenCalledExactlyOnceWith("valid");
    expect(onMissing).not.toHaveBeenCalled();
    expect(onFound).not.toHaveBeenCalled();
  });
});
