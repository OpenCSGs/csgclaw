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

it("keeps the route on a network error and rechecks after bootstrap refresh", async () => {
  const fetchMessages = vi
    .mocked(fetchMessagesRequest)
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockRejectedValueOnce(roomMissing);
  const onMissing = vi.fn();
  const onFound = vi.fn();
  const { rerender } = renderHook(
    ({ updatedAt }) => useMissingConversationRoute("deleted", updatedAt, onMissing, onFound),
    {
      initialProps: { updatedAt: 0 },
    },
  );

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
  const onFound = vi.fn();
  const { rerender } = renderHook<void, { roomID?: string }>(
    ({ roomID }) => useMissingConversationRoute(roomID, 1, onMissing, onFound),
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
  renderHook(() => useMissingConversationRoute("valid", 1, onMissing, onFound));

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
    ({ roomID }) => useMissingConversationRoute(roomID, 1, onMissing, onFound),
    { initialProps: { roomID: "valid" } },
  );
  await waitFor(() => expect(fetchBootstrap).toHaveBeenCalledOnce());
  rerender({ roomID: undefined });
  await act(async () => completeBootstrap(recoveredBootstrap));
  expect(onFound).not.toHaveBeenCalled();
  expect(onMissing).not.toHaveBeenCalled();
});
