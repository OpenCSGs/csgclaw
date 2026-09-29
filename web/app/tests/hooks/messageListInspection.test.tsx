import { useRef } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMessageListAutoScroll } from "@/hooks/workspace/useMessageListAutoScroll";
function Harness({ inspecting, revision }: { inspecting: boolean; revision: number }) {
  const ref = useRef<HTMLElement>(null);
  useMessageListAutoScroll({
    active: true,
    conversationId: "room",
    messageListRef: ref,
    visibleMessagesKey: String(revision),
  });
  return (
    <section ref={ref} data-testid="chat">
      <button type="button" data-preserve-scroll={inspecting ? "true" : undefined}>
        Statistics
      </button>
    </section>
  );
}
beforeEach(() =>
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe = vi.fn();
      disconnect = vi.fn();
    },
  ),
);
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function atBottom() {
  const chat = screen.getByTestId("chat");
  Object.defineProperty(chat, "clientHeight", { configurable: true, value: 400 });
  Object.defineProperty(chat, "scrollHeight", { configurable: true, writable: true, value: 1000 });
  chat.scrollTop = 600;
  return chat;
}
describe("transcript scrolling while inspecting statistics", () => {
  it("pauses streamed growth and resumes following when inspection ends", async () => {
    const view = render(<Harness inspecting revision={1} />);
    const chat = atBottom();
    Object.defineProperty(chat, "scrollHeight", { value: 1200 });
    view.rerender(<Harness inspecting revision={2} />);
    expect(chat.scrollTop).toBe(600);
    view.rerender(<Harness inspecting={false} revision={2} />);
    await waitFor(() => expect(chat.scrollTop).toBe(1200));
  });
  it("does not resume following after the reader scrolls upward", async () => {
    const view = render(<Harness inspecting revision={1} />);
    const chat = atBottom();
    Object.defineProperty(chat, "scrollHeight", { value: 1200 });
    fireEvent.wheel(chat, { deltaY: -100 });
    chat.scrollTop = 400;
    fireEvent.scroll(chat);
    view.rerender(<Harness inspecting={false} revision={2} />);
    await act(() => new Promise((resolve) => setTimeout(resolve, 40)));
    expect(chat.scrollTop).toBe(400);
  });
});
