// @vitest-environment jsdom

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LongMessageCollapse } from "./LongMessageCollapse";

type ResizeObserverCallback = ConstructorParameters<typeof ResizeObserver>[0];

const lineHeight = 24;
const naturalNineLineHeight = lineHeight * 9;
const collapsedPadding = 44;

let resizeObserverCallback: ResizeObserverCallback | null = null;
let naturalContentHeight = naturalNineLineHeight;

class TestResizeObserver implements ResizeObserver {
  constructor(callback: ResizeObserverCallback) {
    resizeObserverCallback = callback;
  }

  disconnect(): void {}

  observe(): void {}

  unobserve(): void {}
}

function t(key: string): string {
  return key === "messageLongCollapse" ? "Collapse" : "Expand";
}

describe("LongMessageCollapse", () => {
  const originalResizeObserver = globalThis.ResizeObserver;
  const originalGetComputedStyle = window.getComputedStyle;
  const originalScrollHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollHeight");

  beforeEach(() => {
    resizeObserverCallback = null;
    globalThis.ResizeObserver = TestResizeObserver;
    window.getComputedStyle = vi.fn((element: Element) => {
      const style = originalGetComputedStyle.call(window, element);
      return new Proxy(style, {
        get(target, property, receiver) {
          if (property === "lineHeight") {
            return `${lineHeight}px`;
          }
          if (property === "fontSize") {
            return "16px";
          }
          const value = Reflect.get(target, property, receiver);
          return typeof value === "function" ? value.bind(target) : value;
        },
      });
    });
    Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
      configurable: true,
      get() {
        if (
          this instanceof HTMLElement &&
          this.classList.contains("long-message-content") &&
          this.closest(".long-message-collapse.is-collapsed") &&
          this.style.paddingBottom !== "0px"
        ) {
          return naturalNineLineHeight + collapsedPadding;
        }
        return naturalContentHeight;
      },
    });
  });

  afterEach(() => {
    resizeObserverCallback = null;
    globalThis.ResizeObserver = originalResizeObserver;
    window.getComputedStyle = originalGetComputedStyle;
    if (originalScrollHeight) {
      Object.defineProperty(HTMLElement.prototype, "scrollHeight", originalScrollHeight);
    }
    vi.restoreAllMocks();
  });

  it("keeps nine-line messages stable across collapsed measurement and expansion", async () => {
    naturalContentHeight = naturalNineLineHeight;
    const user = userEvent.setup();
    render(
      <LongMessageCollapse
        html={"<p>one<br>two<br>three<br>four<br>five<br>six<br>seven<br>eight<br>nine</p>"}
        t={t}
      />,
    );

    const content = document.querySelector<HTMLElement>(".long-message-content");
    expect(content).not.toBeNull();
    expect(screen.getByRole("button", { name: "Expand" })).toBeTruthy();
    expect(content?.style.maxHeight).toBe(`${lineHeight * 8}px`);

    await act(async () => {
      resizeObserverCallback?.([], {} as ResizeObserver);
    });

    expect(screen.getByRole("button", { name: "Expand" })).toBeTruthy();
    expect(content?.style.maxHeight).toBe(`${lineHeight * 8}px`);

    await user.click(screen.getByRole("button", { name: "Expand" }));

    expect(screen.getByRole("button", { name: "Collapse" })).toBeTruthy();
    expect(content?.style.maxHeight).toBe(`${naturalNineLineHeight}px`);
  });

  it("does not show an expand button when long text fits within the collapsed height", () => {
    naturalContentHeight = lineHeight * 7;
    render(<LongMessageCollapse html={`<p>${"Long but visible content. ".repeat(30)}</p>`} t={t} />);

    expect(screen.queryByRole("button", { name: "Expand" })).toBeNull();
    expect(document.querySelector(".long-message-collapse")).toBeNull();
  });
});
