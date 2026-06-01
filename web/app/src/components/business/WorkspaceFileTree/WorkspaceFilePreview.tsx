import type { ReactNode } from "react";
import type { WorkspaceFile } from "@/models/workspace";
import "./WorkspaceFilePreview.css";

type WorkspaceFilePreviewProps = {
  binaryText: string;
  className?: string;
  emptyFileText: string;
  emptyHint: string;
  emptyIcon?: ReactNode;
  emptyTitle: string;
  error?: string;
  file?: WorkspaceFile | null;
  loading?: boolean;
  loadingText: string;
  truncatedText: string;
};

export function WorkspaceFilePreview({
  binaryText,
  className = "",
  emptyFileText,
  emptyHint,
  emptyIcon,
  emptyTitle,
  error = "",
  file = null,
  loading = false,
  loadingText,
  truncatedText,
}: WorkspaceFilePreviewProps) {
  const previewText = file && !file.binary ? file.content || emptyFileText : "";
  const lineCount = previewText ? previewText.split(/\r\n|\r|\n/).length : 0;
  const fileMeta = file?.binary ? binaryText : `${file?.size || 0} B${file?.truncated ? ` - ${truncatedText}` : ""}`;

  return (
    <div className={`workspace-file-preview ${className}`.trim()}>
      {error ? (
        <div className="workspace-empty">{error}</div>
      ) : loading ? (
        <div className="workspace-empty">{loadingText}</div>
      ) : !file ? (
        <div className="workspace-preview-empty-state">
          {emptyIcon ? <span className="workspace-preview-empty-icon">{emptyIcon}</span> : null}
          <strong>{emptyTitle}</strong>
          <p>{emptyHint}</p>
        </div>
      ) : (
        <>
          <div className="workspace-preview-file-header">
            <strong>{file.path}</strong>
            <span>{fileMeta}</span>
          </div>
          <div className="workspace-preview-body">
            {file.binary ? (
              <div className="workspace-empty">{binaryText}</div>
            ) : (
              <div className="workspace-preview-code-shell">
                <pre className="workspace-preview-line-numbers">
                  {Array.from({ length: lineCount }, (_, index) => index + 1).join("\n")}
                </pre>
                <pre className="workspace-preview-code">{previewText}</pre>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
