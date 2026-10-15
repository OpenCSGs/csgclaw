import { useRef } from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MessageTimeline } from "./MessageTimeline";
import { createTranslator } from "@/shared/i18n";
import { MESSAGE_LIST_NAVIGATE_EVENT } from "@/shared/constants/workspace";

const items = ["Start", "Second question", "Last question"].map((label, index) => ({
  id: `message-'${index}`,
  label,
  author: "Jared",
  preview: `Reply to ${label}`,
}));
const scrollTo = vi.fn();
let resizeContent = () => {};
const positions = [100, 400, 1200];

function Harness() {
  const scrollRef = useRef<HTMLElement>(null);
  return (
    <>
      <section
        ref={(node) => {
          scrollRef.current = node;
          if (!node) return;
          Object.defineProperties(node, {
            scrollHeight: { value: 1800, configurable: true },
            clientHeight: { value: 400 },
          });
          node.getBoundingClientRect = () => new DOMRect(0, 40, 800, 400);
          node.scrollTo = scrollTo;
        }}
        data-testid="messages"
      >
        {items.map((item, index) => (
          <div
            key={item.id}
            data-message-id={item.id}
            ref={(node) => {
              if (node)
                node.getBoundingClientRect = () =>
                  new DOMRect(0, 40 + positions[index] - (scrollRef.current?.scrollTop || 0), 600, 90);
            }}
          >
            {item.label}
          </div>
        ))}
      </section>
      <MessageTimeline items={items} scrollRef={scrollRef} t={createTranslator("en")} />
    </>
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  positions[1] = 400;
});

describe("message directory interactions", () => {
  it("previews one turn on hover without moving the conversation and closes on leave", async () => {
    render(<Harness />);
    const rail = screen.getByRole("slider");
    rail.getBoundingClientRect = () => new DOMRect(780, 40, 32, 400);
    fireEvent.pointerEnter(rail, { clientY: 240 });
    const dialog = await screen.findByRole("dialog", { name: "Message preview" });
    expect(within(dialog).getByText("Second question")).toBeInTheDocument();
    expect(within(dialog).getByText("Reply to Second question")).toBeInTheDocument();
    expect(within(dialog).queryByText("Last question")).not.toBeInTheDocument();
    expect(scrollTo).not.toHaveBeenCalled();
    fireEvent.pointerLeave(rail);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
  it("opens by keyboard, jumps to a concrete message, and recalibrates after content grows", async () => {
    vi.stubGlobal("matchMedia", () => ({ matches: true }));
    vi.stubGlobal(
      "ResizeObserver",
      class {
        private callback: () => void;
        constructor(callback: () => void) {
          this.callback = callback;
        }
        observe(target: HTMLElement) {
          if (target.dataset.testid === "messages") resizeContent = this.callback;
        }
        disconnect() {}
      },
    );
    const user = userEvent.setup();
    render(<Harness />);
    await act(() => new Promise((resolve) => setTimeout(resolve, 30)));
    const messages = screen.getByTestId("messages");
    const pause = vi.fn();
    messages.addEventListener(MESSAGE_LIST_NAVIGATE_EVENT, pause);
    await user.tab();
    const rail = screen.getByRole("slider");
    expect(rail).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    const dialog = screen.getByRole("dialog", { name: "Message preview" });
    expect(within(dialog).getByText("Reply to Second question")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Jump to message: Second question" }));
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 368, behavior: "auto" });
    expect(pause).toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(rail).toHaveFocus();
    positions[1] = 700;
    act(() => resizeContent());
    await act(() => new Promise((resolve) => setTimeout(resolve, 30)));
    await user.keyboard("{Enter}");
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 668, behavior: "auto" });
    await user.keyboard("{End}");
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1800, behavior: "auto" });
  });

  it("supports wheel, drag, keyboard navigation and Escape without scrolling preview ancestors", async () => {
    vi.stubGlobal("matchMedia", () => ({ matches: true }));
    render(<Harness />);
    await act(() => new Promise((resolve) => setTimeout(resolve, 30)));
    const rail = screen.getByRole("slider");
    rail.getBoundingClientRect = () => new DOMRect(780, 40, 24, 400);
    fireEvent.wheel(rail, { deltaY: 100 });
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 368, behavior: "auto" });
    fireEvent.keyDown(rail, { key: "End" });
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1800, behavior: "auto" });
    fireEvent.keyDown(rail, { key: "Home" });
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 68, behavior: "auto" });
    fireEvent.pointerDown(rail, { pointerId: 1, button: 0, clientY: 240 });
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 368, behavior: "auto" });
    fireEvent.pointerMove(rail, { pointerId: 1, clientY: 360 });
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1168, behavior: "auto" });
    fireEvent.pointerUp(rail, { pointerId: 1 });
    fireEvent.keyDown(rail, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
