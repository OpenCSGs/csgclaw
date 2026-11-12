import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";

const { JSDOM } = createRequire(import.meta.url)("jsdom") as {
  JSDOM: new (html: string, options: Record<string, unknown>) => { window: Window & typeof globalThis };
};
const asset = (name: string) => readFileSync(resolve("public/onboarding-guide", name), "utf8");
const html = asset("guide.html");

function renderGuide(locale: string, reformat = false) {
  const dom = new JSDOM(html, {
    url: `http://localhost/onboarding-guide/guide.html?locale=${locale}`,
    runScripts: "outside-only",
    pretendToBeVisual: true,
  });
  const w = dom.window;
  w.matchMedia = vi.fn(() => ({ matches: true }) as MediaQueryList);
  w.scrollTo = vi.fn();
  w.HTMLElement.prototype.scrollIntoView = vi.fn();
  if (reformat) {
    const walker = w.document.createTreeWalker(w.document.body, w.NodeFilter.SHOW_TEXT);
    let node;
    while ((node = walker.nextNode())) {
      if (!node.parentElement?.closest("script,style"))
        node.textContent = node.textContent?.replace(/ /g, "\n  ") || "";
    }
  }
  w.eval(asset("i18n.js"));
  w.eval(asset("guide.js"));
  return w;
}

function renderedText(w: Window & typeof globalThis) {
  const walker = w.document.createTreeWalker(w.document.body, w.NodeFilter.SHOW_TEXT);
  const text: string[] = [];
  let node;
  while ((node = walker.nextNode())) {
    if (!node.parentElement?.closest("script,style")) text.push(node.textContent || "");
  }
  return text.join(" ");
}

describe("onboarding guide localization", () => {
  it.each(["en", "zh"])("resolves every declared message and accessible label in %s", (locale) => {
    const w = renderGuide(locale);
    try {
      const { t } = (w as unknown as { CSGClawGuideI18n: { t: (key: string) => string } }).CSGClawGuideI18n;
      const attributes = ["data-i18n", "data-i18n-html", "data-i18n-aria-label", "data-i18n-title", "data-i18n-alt"];
      const original = new JSDOM(html, {});
      try {
        original.window.document.querySelectorAll(attributes.map((name) => `[${name}]`).join(",")).forEach((e) => {
          for (const attribute of attributes) {
            const key = e.getAttribute(attribute);
            if (key) expect(t(key), `${locale}: ${key}`).toBeTypeOf("string");
          }
        });
      } finally {
        original.window.close();
      }
      expect(w.document.documentElement.lang).toBe(locale === "en" ? "en" : "zh-CN");
      expect(w.document.title).toContain(locale === "en" ? "Onboarding Guide" : "新手引导");
      expect(w.document.querySelector('meta[name="description"]')?.getAttribute("content")).toContain(
        locale === "en" ? "A 10-step guide" : "10 步认识",
      );
      expect(renderedText(w)).not.toContain("undefined");
      expect(w.document.querySelector("#step-3 .lead strong")).not.toBeNull();
      const tabs = w.document.querySelectorAll<HTMLButtonElement>("#dots button");
      expect(tabs).toHaveLength(11);
      tabs.forEach((tab, index) => {
        expect(tab.getAttribute("aria-label")).toBe(
          index === 0 ? (locale === "en" ? "Cover" : "封面") : locale === "en" ? `Step ${index}` : `第 ${index} 步`,
        );
        tab.click();
        expect(w.document.querySelector(".slide.is-active")?.id).toBe(index === 0 ? "cover" : `step-${index}`);
      });
    } finally {
      w.close();
    }
  });

  it.each([false, true])(
    "has no untranslated English-mode content, including after whitespace changes (%s)",
    (reformat) => {
      const w = renderGuide("en", reformat);
      try {
        expect(renderedText(w)).not.toMatch(/[\u4e00-\u9fff、，。；：「」]/);
        for (const e of w.document.querySelectorAll("[aria-label],[title],[alt]")) {
          for (const attribute of ["aria-label", "title", "alt"]) {
            expect(e.getAttribute(attribute) || "").not.toMatch(/[\u4e00-\u9fff]/);
          }
        }
        expect(w.document.querySelector(".cover-lead")?.textContent).toContain("This 10-step guide");
        expect(w.document.querySelector(".demo-prompt-example")?.textContent).toContain("no more than 1,500 words");
        expect(w.document.querySelector("#step-7 .lead")?.textContent).toContain("Configure a model for each agent");
        expect(w.document.querySelector(".ghostline")?.textContent).toContain("Your files stay yours");
      } finally {
        w.close();
      }
    },
  );

  it("defaults unsupported locales to Chinese and translates decorative UI copy", () => {
    const w = renderGuide("unsupported");
    try {
      expect(w.document.documentElement.lang).toBe("zh-CN");
      expect(w.document.querySelector(".final-card .k")?.textContent).toBe("你的专属 AI 团队");
      expect(w.document.querySelector(".demo-foot")?.textContent).toContain("提示词");
    } finally {
      w.close();
    }
  });
});
