// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { reportDiagnosticTiming } from "@/api/diagnostics";
import { observeDiagnosticRender, trackDiagnosticSubmission } from "./renderTiming";
vi.mock("@/api/diagnostics", () => ({ reportDiagnosticTiming: vi.fn().mockResolvedValue(undefined) }));
afterEach(() => {
  window.dispatchEvent(new Event("pagehide"));
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});
describe("sender rendering measurements", () => {
  it("records a final presentation once and isolates Agent turns", () => {
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(performance.now());
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
    trackDiagnosticSubmission("room", "source", performance.now() - 100);
    const cleanups = [
      observeDiagnosticRender("room", "source", "turn-a", true),
      observeDiagnosticRender("room", "source", "turn-a", false),
      observeDiagnosticRender("room", "source", "turn-b", true),
    ];
    expect(reportDiagnosticTiming).toHaveBeenCalledTimes(2);
    expect(vi.mocked(reportDiagnosticTiming).mock.calls.map((call) => call[2])).toEqual(["turn-a", "turn-b"]);
    cleanups.forEach((cleanup) => cleanup());
  });
  it("does not count background-tab suspension as response latency", () => {
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    trackDiagnosticSubmission("room", "hidden-source", performance.now());
    visibility.mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    visibility.mockReturnValue("visible");
    const cleanup = observeDiagnosticRender("room", "hidden-source", "turn", true);
    expect(reportDiagnosticTiming).not.toHaveBeenCalled();
    cleanup();
  });
});

it("records first text only after it intersects and a paint opportunity, not on placeholder render", () => {
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  let intersect: IntersectionObserverCallback = () => {};
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  const frames: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.push(callback);
    return frames.length;
  });
  vi.stubGlobal("cancelAnimationFrame", () => {});
  const paint = () => {
    for (let i = 0; i < 2; i++) {
      const current = frames.splice(0);
      current.forEach((fn) => fn(performance.now()));
    }
  };
  const root = document.createElement("div");
  const text = document.createElement("p");
  text.dataset.diagnosticResponseText = "";
  text.textContent = "hello";
  root.append(text);
  document.body.append(root);
  trackDiagnosticSubmission("room", "text-source", performance.now() - 100);
  const cleanup = observeDiagnosticRender("room", "text-source", "turn", false, root);
  paint();
  expect(vi.mocked(reportDiagnosticTiming).mock.calls.at(-1)?.[5]).toBeUndefined();
  intersect(
    [
      {
        target: text,
        isIntersecting: true,
        intersectionRatio: 1,
        boundingClientRect: new DOMRect(0, 0, 10, 10),
        intersectionRect: new DOMRect(0, 0, 10, 10),
        rootBounds: null,
        time: performance.now(),
      },
    ],
    {} as IntersectionObserver,
  );
  paint();
  expect(vi.mocked(reportDiagnosticTiming).mock.calls.at(-1)?.[5]).toEqual({
    ms: expect.any(Number),
    at: expect.any(String),
  });
  const count = vi.mocked(reportDiagnosticTiming).mock.calls.length;
  intersect(
    [
      {
        target: text,
        isIntersecting: true,
        intersectionRatio: 1,
        boundingClientRect: new DOMRect(0, 0, 10, 10),
        intersectionRect: new DOMRect(0, 0, 10, 10),
        rootBounds: null,
        time: performance.now(),
      },
    ],
    {} as IntersectionObserver,
  );
  paint();
  expect(reportDiagnosticTiming).toHaveBeenCalledTimes(count);
  cleanup();
  root.remove();
});
