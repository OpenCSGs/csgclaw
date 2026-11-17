import documentPreviewPanelStyles from "./DocumentPreviewPanel.module.css";
import { useMemo, useState } from "react";
import DOMPurify from "dompurify";
import { Check, Copy, WrapText } from "lucide-react";
import { Button, SegmentedControl, Tooltip } from "@/components/ui";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import { HtmlSourceView } from "./HtmlSourceView";

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
  const [wrap, setWrap] = useState(true);
  const [copyResult, setCopyResult] = useState<{ text: string; ok: boolean }>();
  const srcDoc = useMemo(() => htmlPreviewDocument(text, scale), [scale, text]);
  const copied = copyResult?.text === text ? copyResult.ok : undefined;
  const copyStatus = copied === undefined ? "" : t(copied ? "copiedToClipboard" : "copyTextFailed");

  return (
    <div className={documentPreviewPanelStyles["document-preview-html-shell"]}>
      <div className={documentPreviewPanelStyles["document-preview-mode-switch"]}>
        {mode === "source" ? (
          <div className={documentPreviewPanelStyles["document-preview-source-actions"]}>
            <Tooltip content={t("attachmentPreviewWrapSource")}>
              <Button
                active={wrap}
                aria-label={t("attachmentPreviewWrapSource")}
                aria-pressed={wrap}
                iconOnly
                onClick={() => setWrap(!wrap)}
                size="sm"
                variant="tertiaryGray"
              >
                <WrapText size={16} />
              </Button>
            </Tooltip>
            <Tooltip content={copyStatus || t("attachmentPreviewCopySource")}>
              <Button
                aria-label={t("attachmentPreviewCopySource")}
                disabled={!text}
                iconOnly
                onClick={() => {
                  setCopyResult(undefined);
                  void writeTextToClipboard(text).then((ok) => setCopyResult({ text, ok }));
                }}
                size="sm"
                variant="tertiaryGray"
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </Button>
            </Tooltip>
            <span role="status" className={copied === false ? undefined : "sr-only"}>
              {copyStatus}
            </span>
          </div>
        ) : null}
        <SegmentedControl
          label={t("attachmentPreviewViewMode")}
          size="sm"
          variant="text"
          value={mode}
          onChange={(next) => {
            setMode(next);
            setCopyResult(undefined);
          }}
          options={[
            { value: "preview", label: t("workspacePreviewPreviewTab") },
            { value: "source", label: t("workspacePreviewCodeTab") },
          ]}
        />
      </div>
      {mode === "preview" ? (
        <div className={documentPreviewPanelStyles["document-preview-html-frame-wrap"]}>
          <iframe
            className={documentPreviewPanelStyles["document-preview-html-frame"]}
            referrerPolicy="no-referrer"
            sandbox=""
            srcDoc={srcDoc}
            title={t("attachmentPreviewHtmlDocument")}
          />
        </div>
      ) : (
        <HtmlSourceView label={t("workspacePreviewCodeTab")} scale={scale} text={text} wrap={wrap} />
      )}
    </div>
  );
}
