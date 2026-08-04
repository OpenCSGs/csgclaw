import { readFileSync } from "node:fs";
import path from "node:path";

const indexHTML = readFileSync(path.resolve(process.cwd(), "index.html"), "utf8");

describe("document bootstrap", () => {
  it("initializes the document for a cloud sandbox mount path without an external bootstrap", () => {
    const originalLocation = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    const originalStoredTheme = localStorage.getItem("csgclaw.im.theme");
    const originalTheme = document.documentElement.dataset.theme;
    const originalColorScheme = document.documentElement.style.colorScheme;

    try {
      window.history.replaceState({}, "", "/v1/sandboxes/test-sandbox?jwt=redacted");
      localStorage.removeItem("csgclaw.im.theme");
      document.querySelectorAll("base").forEach((base) => base.remove());

      const parsedIndex = new DOMParser().parseFromString(indexHTML, "text/html");
      const bootstrap = parsedIndex.querySelector<HTMLScriptElement>("#document-bootstrap");
      const favicon = parsedIndex.querySelector<HTMLLinkElement>('link[rel="icon"]');

      expect(bootstrap).not.toBeNull();
      expect(parsedIndex.querySelector('script[src="document-bootstrap.js"]')).toBeNull();

      window.eval(bootstrap!.textContent ?? "");
      expect(document.baseURI).toBe(`${window.location.origin}/v1/sandboxes/test-sandbox/`);
      expect(new URL(favicon!.getAttribute("href")!, document.baseURI).pathname).toBe(
        "/v1/sandboxes/test-sandbox/favicon.ico",
      );
      expect(document.documentElement.dataset.theme).toBe("dark");
      expect(document.documentElement.style.colorScheme).toBe("dark");
    } finally {
      document.querySelectorAll("base").forEach((base) => base.remove());
      window.history.replaceState({}, "", originalLocation);
      if (originalStoredTheme === null) {
        localStorage.removeItem("csgclaw.im.theme");
      } else {
        localStorage.setItem("csgclaw.im.theme", originalStoredTheme);
      }
      if (originalTheme === undefined) {
        delete document.documentElement.dataset.theme;
      } else {
        document.documentElement.dataset.theme = originalTheme;
      }
      document.documentElement.style.colorScheme = originalColorScheme;
    }
  });
});
