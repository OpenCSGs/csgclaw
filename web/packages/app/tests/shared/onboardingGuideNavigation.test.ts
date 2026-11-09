import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";

const { JSDOM } = createRequire(import.meta.url)("jsdom") as {
  JSDOM: new (html: string, options: Record<string, unknown>) => { window: Window & typeof globalThis };
};
const asset = (name: string) => readFileSync(resolve("public/onboarding-guide", name), "utf8");

function renderGuide(locale = "en", reduce = false) {
  const { window: w } = new JSDOM(asset("guide.html"), {
    url: `http://localhost/onboarding-guide/guide.html?locale=${locale}`,
    runScripts: "outside-only",
  });
  let scrollY = 0;
  Object.defineProperty(w, "innerHeight", { value: 800 });
  Object.defineProperty(w, "scrollY", { get: () => scrollY });
  w.document.querySelectorAll<HTMLElement>(".slide").forEach((slide, index) => {
    slide.getBoundingClientRect = () => new w.DOMRect(0, index * 800 - scrollY, 1000, 800);
  });
  const frames: FrameRequestCallback[] = [];
  const timers = new Map<number, () => void>();
  let timerID = 0;
  w.requestAnimationFrame = (callback) => frames.push(callback);
  const timerWindow: Window = w;
  timerWindow.setTimeout = (handler) => {
    timers.set(++timerID, handler as () => void);
    return timerID;
  };
  timerWindow.clearTimeout = (id) => {
    if (id !== undefined) timers.delete(id);
  };
  w.matchMedia = vi.fn(() => ({ matches: reduce }) as MediaQueryList);
  w.scrollTo = vi.fn();
  w.HTMLElement.prototype.scrollIntoView = vi.fn();
  w.eval(asset("i18n.js"));
  w.eval(asset("guide.js"));

  const flushFrames = () => frames.splice(0).forEach((callback) => callback(0));
  const scroll = (step: number) => {
    scrollY = step * 800;
    w.dispatchEvent(new w.Event("scroll"));
    flushFrames();
  };
  return {
    w,
    scroll,
    flushFrames,
    counter: () => w.document.getElementById("counterNow")!.textContent,
    jump: (step: number) => w.document.querySelectorAll<HTMLButtonElement>("#dots button")[step].click(),
    settle: () => {
      const pending = [...timers.values()];
      timers.clear();
      pending.forEach((callback) => callback());
      flushFrames();
    },
  };
}

describe("onboarding guide navigation", () => {
  it.each(["en", "zh"])("keeps the destination counter stable through intermediate scroll events in %s", (locale) => {
    const guide = renderGuide(locale);
    try {
      guide.jump(7);
      for (let step = 0; step <= 7; step++) {
        guide.scroll(step);
        expect(guide.counter()).toBe("07");
        expect(guide.w.document.querySelector(".slide.is-active")?.id).toBe("step-7");
      }
      guide.w.dispatchEvent(new guide.w.Event("scrollend"));
      guide.flushFrames();
      guide.scroll(8);
      expect(guide.counter()).toBe("08");
    } finally {
      guide.w.close();
    }
  });

  it("ignores a scroll frame queued before a new navigation", () => {
    const guide = renderGuide();
    try {
      guide.w.dispatchEvent(new guide.w.Event("scroll"));
      guide.jump(7);
      guide.flushFrames();
      expect(guide.counter()).toBe("07");
    } finally {
      guide.w.close();
    }
  });

  it("uses the latest destination for rapid navigation and ignores an earlier scrollend", () => {
    const guide = renderGuide();
    try {
      guide.jump(7);
      guide.w.document.getElementById("nextBtn")!.click();
      guide.scroll(7);
      guide.w.dispatchEvent(new guide.w.Event("scrollend"));
      guide.flushFrames();
      expect(guide.counter()).toBe("08");
      guide.scroll(8);
      guide.w.dispatchEvent(new guide.w.Event("scrollend"));
      guide.flushFrames();
      guide.scroll(9);
      expect(guide.counter()).toBe("09");
    } finally {
      guide.w.close();
    }
  });

  it.each(["wheel", "touchstart"])("returns control to manual scrolling on %s", (type) => {
    const guide = renderGuide();
    try {
      guide.jump(7);
      guide.scroll(3);
      const event = new guide.w.Event(type);
      Object.defineProperty(event, "touches", { value: [{ clientX: 0, clientY: 0 }] });
      guide.w.dispatchEvent(event);
      guide.flushFrames();
      expect(guide.w.scrollTo).toHaveBeenLastCalledWith({ top: 2400, behavior: "instant" });
      expect(guide.counter()).toBe("03");
      guide.scroll(4);
      expect(guide.counter()).toBe("04");
    } finally {
      guide.w.close();
    }
  });

  it("resumes actual-position tracking when scrolling settles without scrollend", () => {
    const guide = renderGuide();
    try {
      guide.jump(7);
      guide.scroll(3);
      expect(guide.counter()).toBe("07");
      guide.settle();
      expect(guide.counter()).toBe("03");
      guide.scroll(4);
      expect(guide.counter()).toBe("04");
    } finally {
      guide.w.close();
    }
  });

  it("honors reduced motion despite the document's smooth-scroll CSS", () => {
    const guide = renderGuide("en", true);
    try {
      guide.jump(7);
      expect(guide.w.HTMLElement.prototype.scrollIntoView).toHaveBeenLastCalledWith({
        behavior: "instant",
        block: "start",
      });
    } finally {
      guide.w.close();
    }
  });
});
