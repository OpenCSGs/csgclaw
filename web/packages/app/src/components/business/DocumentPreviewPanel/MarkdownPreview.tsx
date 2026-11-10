import documentPreviewPanelStyles from "./DocumentPreviewPanel.module.css";
import { useMemo, useState } from "react";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";
import { SegmentedControl } from "@/components/ui";

export function MarkdownPreview({ scale, text, t }: { scale: number; text: string; t: (key: string) => string }) {
  const [mode, setMode] = useState<"preview" | "source">("preview");
  const html = useMemo(() => renderMarkdown(text), [text]);
  return (
    <div className={documentPreviewPanelStyles["document-preview-markdown-shell"]}>
      <div className={documentPreviewPanelStyles["document-preview-markdown-main"]}>
        <div className={documentPreviewPanelStyles["document-preview-mode-switch"]}>
          <SegmentedControl
            label={t("attachmentPreviewViewMode")}
            size="sm"
            variant="text"
            value={mode}
            onChange={setMode}
            options={[
              { value: "preview", label: t("workspacePreviewPreviewTab") },
              { value: "source", label: t("workspacePreviewCodeTab") },
            ]}
          />
        </div>
        {mode === "preview" ? (
          <article
            className={`${documentPreviewPanelStyles["document-preview-markdown"]} message-content`}
            style={{ fontSize: `${scale}em` }}
            dangerouslySetInnerHTML={{ __html: html }}
          />
        ) : (
          <pre className={documentPreviewPanelStyles["document-preview-text"]} style={{ fontSize: `${scale}em` }}>
            {text}
          </pre>
        )}
      </div>
    </div>
  );
}
