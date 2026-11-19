import { act, renderHook } from "@testing-library/react";
import { fetchTaskConversations } from "@/api/taskConversations";
import { useTaskConversationPages } from "@/hooks/workspace/useTaskConversationPages";
import { useTaskEntry } from "@/hooks/workspace/useTaskEntry";
import { taskConversationPreview } from "@/models/taskConversations";
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

it("updates the live preview without reloading ordinary pages and reconciles searches and terminal turns", async () => {
  vi.useFakeTimers();
  const room: IMConversation = { id: "room", members: ["user"], messages: [] };
  const turn = (revision: number, content: string, status = "running") => ({
    id: "projection",
    content,
    created_at: "2026-11-19T09:00:00Z",
    metadata: {
      csgclaw: { turn_progress: { id: "turn", status, revision, started_at: "2026-11-19T09:00:00Z", items: [] } },
    },
  });
  const first = { ...room, messages: [turn(1, "draft")] };
  const fetch = vi
    .mocked(fetchTaskConversations)
    .mockReset()
    .mockResolvedValue({ rooms: [first], total: 1, has_more: false });
  const queries = createQueryWrapper();
  const { result, rerender, unmount } = renderHook(
    ({ value }: { value: IMConversation }) => {
      const entry = useTaskEntry({
        currentUserID: "user",
        rooms: [value],
        onUpdate: vi.fn(),
        onMessage: vi.fn(),
        onDelete: vi.fn(),
        onOpen: vi.fn(),
        isWorking: () => true,
      });
      useTaskConversationPages(false);
      useTaskConversationPages(false, "draft");
      return {
        preview: taskConversationPreview(first, entry.liveRooms.get("room")),
        onHistoryEvent: entry.onHistoryEvent,
      };
    },
    { initialProps: { value: first }, wrapper: queries.wrapper },
  );
  const tick = () => act(() => vi.advanceTimersByTimeAsync(501));
  try {
    await tick();
    fetch.mockClear();
    for (let rev = 2; rev <= 5; rev++) {
      rerender({ value: { ...room, messages: [turn(rev, "draft")] } });
      await tick();
    }
    expect(fetch).not.toHaveBeenCalled();
    rerender({ value: { ...room, messages: [turn(6, "revised draft")] } });
    expect(result.current.preview?.content).toBe("revised draft");
    await tick();
    expect(fetch.mock.calls.map(([q]) => q)).toEqual(["draft"]);
    fetch.mockClear();
    rerender({ value: { ...room, messages: [turn(7, "final answer", "succeeded")] } });
    await tick();
    expect(fetch.mock.calls.map(([q]) => q).sort()).toEqual(["", "draft"]);

    fetch.mockClear();
    const reply = { ...turn(10, ""), relates_to: { rel_type: "m.thread", event_id: "root" } };
    act(() => result.current.onHistoryEvent({ type: "message.created", message: reply }));
    await tick();
    expect(fetch).not.toHaveBeenCalled();
    act(() =>
      result.current.onHistoryEvent({
        type: "message.created",
        message: { ...reply, ...turn(11, "thread answer", "succeeded") },
      }),
    );
    await tick();
    expect(fetch.mock.calls.map(([q]) => q).sort()).toEqual(["", "draft"]);
    fetch.mockClear();
    act(() => result.current.onHistoryEvent({ type: "connection.open" }));
    await tick();
    expect(fetch.mock.calls.map(([q]) => q).sort()).toEqual(["", "draft"]);
  } finally {
    unmount();
    queries.client.clear();
    vi.useRealTimers();
  }
});
