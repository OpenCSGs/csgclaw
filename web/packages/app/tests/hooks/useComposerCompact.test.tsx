import { act, render } from "@testing-library/react";
import { useRef } from "react";
import { useComposerCompact } from "@/shared/layout/useComposerCompact";

let resize: () => void;
function Harness({ width, label = "model", ready = true }: { width: number; label?: string; ready?: boolean }) {
  const box = useRef<HTMLDivElement>(null);
  const row = useRef<HTMLDivElement>(null);
  useComposerCompact(box, row, ready);
  return ready ? (
    <div
      ref={(node) => {
        box.current = node;
        if (node) Object.defineProperty(node, "clientWidth", { configurable: true, value: width });
      }}
      data-testid="box"
    >
      <div
        ref={(node) => {
          row.current = node;
          if (!node) return;
          Object.defineProperty(node, "clientWidth", { configurable: true, value: width });
          Object.defineProperty(node, "scrollWidth", {
            configurable: true,
            get: () => {
              const stage = box.current?.getAttribute("data-compact");
              return stage === "all" ? 250 : stage === "config" ? 400 : node.textContent === "long model" ? 900 : 600;
            },
          });
        }}
      >
        {label}
      </div>
    </div>
  ) : null;
}

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        resize = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
});
afterEach(() => vi.unstubAllGlobals());

it("uses actual overflow with a one pixel tolerance and restores labels when widened", () => {
  const { getByTestId, rerender } = render(<Harness width={300} />);
  const box = getByTestId("box");
  expect(box).toHaveAttribute("data-compact", "all");
  rerender(<Harness width={500} />);
  act(() => resize());
  expect(box).toHaveAttribute("data-compact", "config");
  rerender(<Harness width={599} />);
  act(() => resize());
  expect(box).not.toHaveAttribute("data-compact");
});

it("keeps the current stage at zero width and measures again when visible", () => {
  const { getByTestId, rerender } = render(<Harness width={500} />);
  rerender(<Harness width={0} />);
  act(() => resize());
  expect(getByTestId("box")).toHaveAttribute("data-compact", "config");
  rerender(<Harness width={800} />);
  act(() => resize());
  expect(getByTestId("box")).not.toHaveAttribute("data-compact");
});

it("remeasures changed content without waiting for animation frames", async () => {
  vi.stubGlobal("requestAnimationFrame", vi.fn());
  const { getByTestId, rerender } = render(<Harness width={800} />);
  rerender(<Harness width={800} label="long model" />);
  await act(async () => {});
  expect(getByTestId("box")).toHaveAttribute("data-compact", "config");
  expect(requestAnimationFrame).not.toHaveBeenCalled();
});

it("initializes when an asynchronously loaded composer mounts", () => {
  const { getByTestId, rerender } = render(<Harness width={300} ready={false} />);
  rerender(<Harness width={300} />);
  expect(getByTestId("box")).toHaveAttribute("data-compact", "all");
});
