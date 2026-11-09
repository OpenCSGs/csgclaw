import { act, renderHook } from "@testing-library/react";
import { useCallback, useState } from "react";
import { fetchMessagesRequest } from "@/api/im";
import type { IMData, IMMessage } from "@/models/conversations";
import { useConversationMessageSync } from "./useConversationMessageSync";

vi.mock("@/api/im", () => ({ fetchMessagesRequest: vi.fn() }));
const message = (id: string): IMMessage => ({ id, sender_id: "agent", content: id });
function setup() {
  return renderHook(
    ({ roomID }) => {
      const [data, setData] = useState<IMData | null>({
        users: [],
        rooms: ["one", "two"].map((id) => ({ id, members: [], messages: [] })),
      });
      const update = useCallback((value: Parameters<typeof setData>[0]) => setData(value), []);
      const invalidate = useConversationMessageSync(
        roomID,
        update,
        data?.rooms.find((room) => room.id === roomID),
      );
      return { data, setData, invalidate };
    },
    { initialProps: { roomID: "one" } },
  );
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(fetchMessagesRequest).mockReset();
});
afterEach(() => vi.useRealTimers());

it("recovers the first prompt and later replies without SSE, preserving messages received during a fetch", async () => {
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([message("prompt")]);
  const { result, unmount } = setup();
  await act(async () => {});
  expect(result.current.data?.rooms[0].messages.map((item) => item.id)).toEqual(["prompt"]);
  let finish!: (messages: IMMessage[]) => void;
  vi.mocked(fetchMessagesRequest).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  act(() =>
    result.current.setData(
      (data) =>
        data && {
          ...data,
          rooms: data.rooms.map((room) =>
            room.id === "one" ? { ...room, messages: [...room.messages, message("live")] } : room,
          ),
        },
    ),
  );
  await act(async () => {
    finish([message("prompt"), message("reply")]);
  });
  expect(result.current.data?.rooms[0].messages.map((item) => item.id)).toEqual(["prompt", "reply", "live"]);
  unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4000);
  });
  expect(fetchMessagesRequest).toHaveBeenCalledTimes(2);
});

it("ignores a late response after switching rooms and retries after a transient error", async () => {
  let resolve!: (messages: IMMessage[]) => void;
  vi.mocked(fetchMessagesRequest).mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { result, rerender } = setup();
  vi.mocked(fetchMessagesRequest).mockRejectedValueOnce(new Error("offline"));
  rerender({ roomID: "two" });
  await act(async () => {
    resolve([message("old")]);
  });
  expect(result.current.data?.rooms[0].messages).toEqual([]);
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([message("recovered")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[1].messages[0].id).toBe("recovered");
});

it("advances a single reply from running to completed without letting stale snapshots rewind it", async () => {
  const progress = (revision: number, status: string): IMMessage => ({
    ...message("turn"),
    content: status === "completed" ? "Full reply" : "",
    metadata: {
      csgclaw: {
        turn_progress: {
          id: "turn",
          revision,
          status,
          started_at: "2026-10-21T00:00:00Z",
          items: [],
        },
      },
    },
  });
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([progress(1, "running")]);
  const { result } = setup();
  await act(async () => {});
  expect(result.current.data?.rooms[0].messages[0].content).toBe("");
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([progress(3, "completed")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages).toHaveLength(1);
  expect(result.current.data?.rooms[0].messages[0].content).toBe("Full reply");
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([progress(2, "running")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages[0].content).toBe("Full reply");
});

it("discards a snapshot from before a clear and continues syncing new replies", async () => {
  let finish!: (messages: IMMessage[]) => void;
  vi.mocked(fetchMessagesRequest).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result } = setup();
  act(() => {
    result.current.invalidate("one");
    result.current.setData((data) => data && { ...data, rooms: data.rooms.map((room) => ({ ...room, messages: [] })) });
  });
  await act(async () => {
    finish([message("deleted")]);
  });
  expect(result.current.data?.rooms[0].messages).toEqual([]);
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([message("new")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages.map((item) => item.id)).toEqual(["new"]);
});

it("restores server order when an older message was missed by SSE", async () => {
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([]);
  const { result } = setup();
  await act(async () => {});
  act(() =>
    result.current.setData(
      (data) =>
        data && {
          ...data,
          rooms: data.rooms.map((room) => (room.id === "one" ? { ...room, messages: [message("newer")] } : room)),
        },
    ),
  );
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([message("older"), message("newer")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages.map((item) => item.id)).toEqual(["older", "newer"]);
});

it("retains a live thread update but accepts a lower count on the following poll", async () => {
  const root = { ...message("root"), thread: { root_id: "root", reply_count: 1 } };
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([root]);
  const { result } = setup();
  await act(async () => {});
  let finish!: (messages: IMMessage[]) => void;
  vi.mocked(fetchMessagesRequest).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  act(() =>
    result.current.setData(
      (current) =>
        current && {
          ...current,
          rooms: current.rooms.map((room) =>
            room.id === "one"
              ? { ...room, messages: [{ ...root, thread: { root_id: "root", reply_count: 2 } }] }
              : room,
          ),
        },
    ),
  );
  await act(async () => {
    finish([root]);
  });
  expect(result.current.data?.rooms[0].messages[0].thread?.reply_count).toBe(2);
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([root]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages[0].thread?.reply_count).toBe(1);
});

it("globally invalidates old snapshots on participant deletion and resumes with authoritative state", async () => {
  let finish!: (messages: IMMessage[]) => void;
  vi.mocked(fetchMessagesRequest).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result } = setup();
  act(() => result.current.invalidate());
  await act(async () => {
    finish([message("deleted-user-message")]);
  });
  expect(result.current.data?.rooms[0].messages).toEqual([]);
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([message("remaining-user-message")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages.map((item) => item.id)).toEqual(["remaining-user-message"]);
});

it("converges to an empty transcript after missing a room clear event", async () => {
  vi.mocked(fetchMessagesRequest).mockResolvedValueOnce([message("old")]);
  const { result } = setup();
  await act(async () => {});
  expect(result.current.data?.rooms[0].messages).toHaveLength(1);
  vi.mocked(fetchMessagesRequest).mockResolvedValue([]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages).toEqual([]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current.data?.rooms[0].messages).toEqual([]);
});
