import { useRef, useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMessageListAutoScroll } from "@/hooks/workspace/useMessageListAutoScroll";
function Harness({ inspecting, revision }: { inspecting: boolean; revision: number }) {
  const ref = useRef<HTMLElement>(null);
  const [expanded, setExpanded] = useState(false);
  const { follow } = useMessageListAutoScroll({
    active: true,
    conversationId: "room",
    messageListRef: ref,
    visibleMessagesKey: String(revision),
  });
  return (
    <section ref={ref} data-testid="chat">
      <div data-turn-id="turn">
        <button data-scroll-anchor-toggle aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
          Toggle progress
        </button>
        {expanded ? <div>Expanded progress</div> : null}
      </div>
      <button onClick={() => follow()}>Follow</button>
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
  it("preserves disclosure position but allows follow and subsequent streaming while still expanded", async () => {
    const view = render(<Harness inspecting={false} revision={1} />);
    const chat = atBottom();
    fireEvent.click(screen.getByRole("button", { name: "Toggle progress" }));
    Object.defineProperty(chat, "scrollHeight", { value: 1600 });
    await act(() => new Promise((resolve) => setTimeout(resolve, 60)));
    expect(chat.scrollTop).toBe(600);
    expect(screen.getByRole("button", { name: "Toggle progress" })).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(screen.getByRole("button", { name: "Follow" }));
    expect(chat.scrollTop).toBe(1600);
    Object.defineProperty(chat, "scrollHeight", { value: 1800 });
    view.rerender(<Harness inspecting={false} revision={2} />);
    await waitFor(() => expect(chat.scrollTop).toBe(1800));
  });
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
