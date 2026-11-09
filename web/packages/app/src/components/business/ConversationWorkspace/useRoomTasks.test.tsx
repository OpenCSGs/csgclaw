import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchRoomTasks } from "@/api/tasks";
import { normalizeTaskList } from "@/models/tasks";
import type { IMConversation } from "@/models/conversations";
import { useRoomTasks } from "./useRoomTasks";

vi.mock("@/api/tasks", () => ({ fetchRoomTasks: vi.fn() }));

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Provider({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

const room: IMConversation = { id: "r1", members: [], messages: [], type: "on_demand" };

describe("useRoomTasks", () => {
  beforeEach(() => vi.resetAllMocks());

  it("loads persisted room tasks and never carries them into a different room", async () => {
    vi.mocked(fetchRoomTasks)
      .mockResolvedValueOnce(normalizeTaskList([{ id: "root", assignment_type: "room", assignment_id: "r1" }]))
      .mockResolvedValueOnce([]);
    const { result, rerender } = renderHook((conversation) => useRoomTasks(conversation), {
      initialProps: room,
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.data?.[0]?.id).toBe("root"));
    rerender({ ...room, id: "r2" });
    expect(result.current.data).toBeUndefined();
    await waitFor(() => expect(result.current.data).toEqual([]));
    expect(fetchRoomTasks).toHaveBeenLastCalledWith("r2", expect.any(AbortSignal));
  });

  it("refreshes persisted status immediately on task feedback without polling chat", async () => {
    const pending = normalizeTaskList([
      { id: "root", assignment_type: "room", assignment_id: "r1", status: "in_progress" },
    ]);
    const finished = normalizeTaskList([
      { id: "root", assignment_type: "room", assignment_id: "r1", status: "completed" },
    ]);
    vi.mocked(fetchRoomTasks).mockResolvedValueOnce(pending).mockResolvedValue(finished);
    const { result, rerender } = renderHook((conversation) => useRoomTasks(conversation), {
      initialProps: room,
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("in_progress"));
    rerender({
      ...room,
      messages: [{ id: "feedback", sender_id: "manager", content: "Delivered", metadata: { task_id: "root" } }],
    });
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("completed"));
    expect(fetchRoomTasks).toHaveBeenCalledTimes(2);
    rerender({
      ...room,
      messages: [
        { id: "feedback", sender_id: "manager", content: "Delivered", metadata: { task_id: "root" } },
        { id: "chat", content: "Thanks" },
      ],
    });
    expect(fetchRoomTasks).toHaveBeenCalledTimes(2);
  });

  it("does not fetch tasks for ordinary rooms or direct conversations", () => {
    const ordinaryRoom: IMConversation = { ...room, type: "free" };
    const { rerender } = renderHook((conversation: IMConversation) => useRoomTasks(conversation), {
      initialProps: ordinaryRoom,
      wrapper: wrapper(),
    });
    rerender({ ...room, type: "on_demand", is_direct: true });
    expect(fetchRoomTasks).not.toHaveBeenCalled();
  });
});
