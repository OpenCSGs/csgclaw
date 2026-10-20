import { act, renderHook, waitFor } from "@testing-library/react";
import { useSearchMessageNavigation } from "./useSearchMessageNavigation";
import { MESSAGE_LIST_NAVIGATE_EVENT } from "@/shared/constants/workspace";

it("waits for the selected conversation and positions the matching message", async () => {
  const list = document.createElement("div");
  const message = document.createElement("div");
  const scrollIntoView = vi.fn();
  message.dataset.messageId = "match";
  message.scrollIntoView = scrollIntoView;
  list.append(message);
  document.body.append(list);
  const pauseFollowing = vi.fn();
  list.addEventListener(MESSAGE_LIST_NAVIGATE_EVENT, pauseFollowing);
  const ref = { current: list };
  const openThread = vi.fn().mockResolvedValue(undefined);
  const { result, rerender } = renderHook(
    ({ roomID }) => useSearchMessageNavigation({ active: true, roomID, messageListRef: ref, openThread }),
    { initialProps: { roomID: "other" } },
  );

  act(() => result.current("room", { id: "match", content: "matched" }));
  expect(scrollIntoView).not.toHaveBeenCalled();
  rerender({ roomID: "room" });
  await waitFor(() => expect(scrollIntoView).toHaveBeenCalledWith({ block: "center", behavior: "auto" }));
  expect(pauseFollowing).toHaveBeenCalledOnce();
  expect(openThread).not.toHaveBeenCalled();
  list.remove();
});

it("opens a thread before locating a matching reply", async () => {
  const thread = document.createElement("div");
  thread.className = "thread-panel-body";
  const reply = document.createElement("div");
  reply.dataset.messageId = "reply";
  reply.scrollIntoView = vi.fn();
  thread.append(reply);
  const openThread = vi.fn(async () => {
    document.body.append(thread);
  });
  const { result } = renderHook(() =>
    useSearchMessageNavigation({ active: true, roomID: "room", messageListRef: { current: null }, openThread }),
  );

  act(() =>
    result.current("room", {
      id: "reply",
      content: "matched reply",
      relates_to: { rel_type: "m.thread", event_id: "root" },
    }),
  );
  await waitFor(() => expect(reply.scrollIntoView).toHaveBeenCalled());
  expect(openThread).toHaveBeenCalledWith("root");
  thread.remove();
});
