import { useMemo, useState } from "react";
import DOMPurify from "dompurify";
import { SyntaxHighlightedText } from "./SyntaxHighlightedText";

const htmlPreviewCSP = [
  "default-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
  "img-src data: blob:",
  "media-src data: blob:",
  "font-src data:",
  "style-src 'unsafe-inline'",
].join("; ");

const urlAttributes = ["action", "background", "cite", "formaction", "href", "poster", "src", "xlink:href"];

function isSafeEmbeddedURL(attribute: string, value: string): boolean {
  const normalized = value.trim();
  if (attribute === "href" || attribute === "xlink:href") {
    return normalized.startsWith("#");
  }
  return /^(?:blob:|data:(?:audio\/|video\/|image\/(?:avif|gif|jpeg|png|webp)(?:[;,]|$)))/i.test(normalized);
}

function sanitizeInlineCSS(css: string): string {
  return css
    .replace(/@import\s+[^;]+;?/gi, "")
    .replace(/url\(\s*(['"]?)(.*?)\1\s*\)/gi, (match, _quote: string, url: string) =>
      isSafeEmbeddedURL("src", url) ? match : "none",
    );
}

function sanitizedHTMLFragment(source: string): string {
  const sourceDocument = new DOMParser().parseFromString(source, "text/html");
  const styles = Array.from(sourceDocument.querySelectorAll("style"))
    .map((style) => style.textContent ?? "")
    .join("\n");
  sourceDocument.querySelectorAll("style").forEach((style) => style.remove());
  const sanitized = DOMPurify.sanitize(sourceDocument.body.innerHTML, {
    // Preserve anchors such as id="forms". This markup is only loaded into an
    // opaque-origin, script-disabled sandbox, never inserted into the app DOM.
    SANITIZE_DOM: false,
    FORBID_ATTR: ["srcdoc"],
    FORBID_TAGS: ["base", "embed", "iframe", "link", "meta", "object", "script"],
  });
  const template = document.createElement("template");
  template.innerHTML = sanitized;
  template.content.querySelectorAll<Element>("*").forEach((element) => {
    element.removeAttribute("srcset");
    if (element.matches("a, area")) {
      element.setAttribute("target", "_self");
      element.removeAttribute("download");
      element.removeAttribute("ping");
    }
    let blockedMediaSource = false;
    urlAttributes.forEach((attribute) => {
      const value = element.getAttribute(attribute);
      if (value !== null && !isSafeEmbeddedURL(attribute, value)) {
        element.removeAttribute(attribute);
        if (attribute === "src" && element.matches("audio, img, source, video")) {
          blockedMediaSource = true;
        }
      }
    });
    if (element.matches("a, area")) {
      // srcdoc otherwise resolves fragment links against the parent app URL.
      for (const attribute of ["href", "xlink:href"]) {
        const fragment = element.getAttribute(attribute)?.trim();
        if (fragment?.startsWith("#")) {
          element.setAttribute(attribute, `about:srcdoc${fragment}`);
        }
      }
    }
    if (blockedMediaSource) {
      element.remove();
      return;
    }
    const style = element.getAttribute("style");
    if (style !== null) {
      const safeStyle = sanitizeInlineCSS(style).trim();
      if (safeStyle) {
        element.setAttribute("style", safeStyle);
      } else {
        element.removeAttribute("style");
      }
    }
  });
  const safeStyles = sanitizeInlineCSS(styles).trim();
  return `${safeStyles ? `<style>${safeStyles}</style>` : ""}${template.innerHTML}`;
}

function htmlPreviewDocument(source: string, scale: number): string {
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${htmlPreviewCSP}">
<style>
  :root { color-scheme: light; zoom: ${scale}; }
  html { min-height: 100%; background: #fff; color: #111827; }
  body { min-height: 100%; margin: 0; padding: 24px; box-sizing: border-box; overflow-wrap: anywhere; }
  img, video { max-width: 100%; height: auto; }
</style>
</head>
<body>${sanitizedHTMLFragment(source)}</body>
</html>`;
}

export default function HtmlPreview({ scale, text, t }: { scale: number; text: string; t: (key: string) => string }) {
  const [mode, setMode] = useState<"preview" | "source">("preview");
  const srcDoc = useMemo(() => htmlPreviewDocument(text, scale), [scale, text]);

  return (
    <div className="document-preview-html-shell">
      <div className="document-preview-mode-switch" role="tablist" aria-label={t("attachmentPreviewViewMode")}>
        <button
          type="button"
          role="tab"
          aria-selected={mode === "preview"}
          className={mode === "preview" ? "active" : ""}
          onClick={() => setMode("preview")}
        >
          {t("workspacePreviewPreviewTab")}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === "source"}
          className={mode === "source" ? "active" : ""}
          onClick={() => setMode("source")}
        >
          {t("workspacePreviewCodeTab")}
        </button>
      </div>
      {mode === "preview" ? (
        <div className="document-preview-html-frame-wrap">
          <iframe
            className="document-preview-html-frame"
            referrerPolicy="no-referrer"
            sandbox=""
            srcDoc={srcDoc}
            title={t("attachmentPreviewHtmlDocument")}
          />
        </div>
      ) : (
        <SyntaxHighlightedText language="html" scale={scale} text={text} t={t} />
      )}
    </div>
  );
}
