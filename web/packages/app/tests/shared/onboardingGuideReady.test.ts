import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";

const { JSDOM } = createRequire(import.meta.url)("jsdom") as {
  JSDOM: new (html: string, options: Record<string, unknown>) => { window: Window & typeof globalThis };
};
const asset = (name: string) => readFileSync(resolve("public/onboarding-guide", name), "utf8");

function createGuide() {
  const { window: w } = new JSDOM(asset("guide.html"), {
    url: "http://localhost/onboarding-guide/guide.html?locale=en&embedded=1",
    runScripts: "outside-only",
    pretendToBeVisual: true,
  });
  const postMessage = vi.fn();
  Object.defineProperty(w, "parent", { value: { postMessage } });
  const frames: FrameRequestCallback[] = [];
  w.requestAnimationFrame = (callback) => frames.push(callback);
  w.matchMedia = vi.fn(() => ({ matches: false }) as MediaQueryList);
  w.scrollTo = vi.fn();
  w.eval(asset("i18n.js"));
  return { w, frames, postMessage };
}

describe("embedded onboarding guide readiness", () => {
  it("reports readiness after successful initialization and the first view has time to render", () => {
    const { w, frames, postMessage } = createGuide();
    try {
      w.eval(asset("guide.js"));
      expect(w.document.querySelectorAll("#dots button")).toHaveLength(11);
      expect(postMessage).not.toHaveBeenCalled();
      frames.shift()!(0);
      expect(postMessage).not.toHaveBeenCalled();
      frames.shift()!(0);
      expect(postMessage).toHaveBeenCalledWith({ type: "csgclaw:guide-ready" }, w.location.origin);
      w.document.querySelector<HTMLButtonElement>("[data-exit]")!.click();
      expect(postMessage).toHaveBeenLastCalledWith({ type: "csgclaw:guide-exit" }, w.location.origin);
    } finally {
      w.close();
    }
  });

  it("does not report readiness after failed initialization, including on host focus requests", () => {
    const { w, frames, postMessage } = createGuide();
    try {
      w.scrollTo = () => {
        throw new Error("Initialization failed");
      };
      expect(() => w.eval(asset("guide.js"))).toThrow("Initialization failed");
      w.dispatchEvent(
        new w.MessageEvent("message", {
          source: w.parent,
          origin: w.location.origin,
          data: { type: "csgclaw:guide-focus" },
        }),
      );
      expect(frames).toHaveLength(0);
      expect(postMessage).not.toHaveBeenCalled();
    } finally {
      w.close();
    }
  });
});
