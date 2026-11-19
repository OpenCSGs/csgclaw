import { act, renderHook } from "@testing-library/react";
import { fetchTaskConversations } from "@/api/taskConversations";
import { useTaskConversationPages } from "@/hooks/workspace/useTaskConversationPages";
import { useTaskEntry } from "@/hooks/workspace/useTaskEntry";
import type { IMConversation } from "@/models/conversations";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/taskConversations", () => ({ fetchTaskConversations: vi.fn() }));

it("finishes a slow history refresh while collaboration progress keeps arriving", async () => {
  vi.useFakeTimers();
  const room: IMConversation = {
    id: "roundtable",
    title: "Research",
    type: "autonomous",
    members: ["user-admin"],
    messages: [],
  };
  const page = { rooms: [room], total: 1, has_more: false };
  let finishRefresh!: (value: typeof page) => void;
  const pending = new Promise<typeof page>((resolve) => {
    finishRefresh = resolve;
  });
  vi.mocked(fetchTaskConversations)
    .mockResolvedValueOnce({ rooms: [], total: 0, has_more: false })
    .mockReturnValueOnce(pending)
    .mockResolvedValue(page);
  const queries = createQueryWrapper();
  const { result, rerender, unmount } = renderHook(
    ({ rooms }: { rooms: IMConversation[] }) => {
      useTaskEntry({
        currentUserID: "user-admin",
        rooms,
        onUpdate: vi.fn(),
        onMessage: vi.fn(),
        onDelete: vi.fn(),
        onOpen: vi.fn(),
        isWorking: () => true,
      });
      return useTaskConversationPages(false);
    },
    { initialProps: { rooms: [room] }, wrapper: queries.wrapper },
  );
  try {
    await act(() => vi.advanceTimersByTimeAsync(501));
    expect(fetchTaskConversations).toHaveBeenCalledTimes(2);
    for (let i = 0; i < 4; i++) {
      rerender({ rooms: [{ ...room, messages: [{ id: `progress-${i}`, content: "Working" }] }] });
      await act(() => vi.advanceTimersByTimeAsync(501));
    }
    expect(fetchTaskConversations).toHaveBeenCalledTimes(2);
    await act(async () => {
      finishRefresh({ rooms: [], total: 0, has_more: false });
      await vi.advanceTimersByTimeAsync(501);
    });
    expect(fetchTaskConversations).toHaveBeenCalledTimes(3);
    expect(result.current.data?.pages[0].rooms).toEqual([room]);
  } finally {
    unmount();
    queries.client.clear();
    vi.useRealTimers();
  }
});
