import { useMemo, useState } from "react";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";

export function MarkdownPreview({ scale, text, t }: { scale: number; text: string; t: (key: string) => string }) {
  const [mode, setMode] = useState<"preview" | "source">("preview");
  const html = useMemo(() => renderMarkdown(text), [text]);
  const headings = useMemo(
    () =>
      text
        .split(/\r?\n/)
        .map((line) => /^(#{1,6})\s+(.+)$/.exec(line))
        .filter((match): match is RegExpExecArray => Boolean(match))
        .map((match) => ({ depth: match[1].length, title: match[2].trim() })),
    [text],
  );
  return (
    <div className="document-preview-markdown-shell">
      {headings.length > 0 ? (
        <aside className="document-preview-markdown-outline" aria-label={t("attachmentPreviewOutline")}>
          {headings.map((heading, index) => (
            <span key={`${heading.title}-${index}`} style={{ paddingInlineStart: `${(heading.depth - 1) * 10}px` }}>
              {heading.title}
            </span>
          ))}
        </aside>
      ) : null}
      <div className="document-preview-markdown-main">
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
          <article
            className="document-preview-markdown message-content"
            style={{ fontSize: `${scale}em` }}
            dangerouslySetInnerHTML={{ __html: html }}
          />
        ) : (
          <pre className="document-preview-text" style={{ fontSize: `${scale}em` }}>
            {text}
          </pre>
        )}
      </div>
    </div>
  );
}
