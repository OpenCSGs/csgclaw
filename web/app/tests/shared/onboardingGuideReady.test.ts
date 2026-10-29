import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const html = readFileSync(resolve("public/onboarding-guide/guide.html"), "utf8");
const script = readFileSync(resolve("public/onboarding-guide/guide.js"), "utf8");

describe("onboarding guide readiness", () => {
  let frames: FrameRequestCallback[];
  const postMessage = vi.fn();

  beforeEach(() => {
    document.body.innerHTML = new DOMParser().parseFromString(html, "text/html").body.innerHTML;
    frames = [];
    postMessage.mockClear();
    vi.stubGlobal("parent", { postMessage });
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => frames.push(callback));
    vi.stubGlobal("matchMedia", () => ({ matches: false }));
    vi.stubGlobal("scrollTo", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("acknowledges the initialized page after allowing the first view to render", () => {
    window.eval(script);
    expect(document.querySelectorAll("#dots button")).toHaveLength(11);
    expect(postMessage).not.toHaveBeenCalled();
    frames.shift()!(0);
    expect(postMessage).not.toHaveBeenCalled();
    frames.shift()!(0);
    expect(postMessage).toHaveBeenCalledWith({ type: "csgclaw:guide-ready" }, window.location.origin);
  });

  it("does not acknowledge a page whose initialization failed", () => {
    vi.stubGlobal("matchMedia", () => {
      throw new Error("Initialization failed");
    });
    expect(() => window.eval(script)).toThrow("Initialization failed");
    expect(frames).toHaveLength(0);
    expect(postMessage).not.toHaveBeenCalled();
  });
});
