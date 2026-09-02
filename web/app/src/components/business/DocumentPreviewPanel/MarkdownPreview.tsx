import { useMemo, useState } from "react";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";

export function MarkdownPreview({ scale, text, t }: { scale: number; text: string; t: (key: string) => string }) {
  const [mode, setMode] = useState<"preview" | "source">("preview");
  const html = useMemo(() => renderMarkdown(text), [text]);
  return (
    <div className="document-preview-markdown-shell">
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
