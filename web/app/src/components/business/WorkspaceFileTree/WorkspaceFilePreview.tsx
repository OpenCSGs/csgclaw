import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { FileSearch, X } from "lucide-react";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";
import {
  Button,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import type { WorkspaceFile } from "@/models/workspace";
import "./WorkspaceFilePreview.css";

type WorkspaceFileDialogMode = "code" | "preview";

type WorkspaceFilePreviewProps = {
  binaryText: string;
  className?: string;
  closeText: string;
  codeText: string;
  emptyFileText: string;
  emptyHint: string;
  emptyIcon?: ReactNode;
  emptyTitle: string;
  error?: string;
  file?: WorkspaceFile | null;
  loading?: boolean;
  loadingText: string;
  previewText: string;
  truncatedText: string;
  viewToggleLabel: string;
};

export function WorkspaceFilePreview({
  binaryText,
  className = "",
  closeText,
  codeText,
  emptyFileText,
  emptyHint,
  emptyIcon,
  emptyTitle,
  error = "",
  file = null,
  loading = false,
  loadingText,
  previewText,
  truncatedText,
  viewToggleLabel,
}: WorkspaceFilePreviewProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogMode, setDialogMode] = useState<WorkspaceFileDialogMode>("preview");
  const fileText = file && !file.binary ? file.content || emptyFileText : "";
  const lineCount = fileText ? fileText.split(/\r\n|\r|\n/).length : 0;
  const fileMeta = file?.binary ? binaryText : `${file?.size || 0} B${file?.truncated ? ` - ${truncatedText}` : ""}`;
  const markdownFile = Boolean(file && !file.binary && isMarkdownPath(file.path));
  const activeDialogMode: WorkspaceFileDialogMode = markdownFile ? dialogMode : "code";
  const renderedMarkdown = useMemo(() => (markdownFile ? renderMarkdown(fileText) : ""), [fileText, markdownFile]);

  useEffect(() => {
    setDialogOpen(false);
    setDialogMode(markdownFile ? "preview" : "code");
  }, [file?.path, markdownFile]);

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
            <div className="workspace-preview-file-actions">
              <span className="workspace-preview-file-meta">{fileMeta}</span>
              {!file.binary ? (
                <Tooltip content={previewText}>
                  <Button
                    iconOnly
                    aria-label={previewText}
                    className="workspace-preview-open-button"
                    size="sm"
                    variant="tertiaryGray"
                    onClick={() => setDialogOpen(true)}
                  >
                    <FileSearch size={16} strokeWidth={2} aria-hidden="true" />
                  </Button>
                </Tooltip>
              ) : null}
            </div>
          </div>
          <div className="workspace-preview-body">
            {file.binary ? (
              <div className="workspace-empty">{binaryText}</div>
            ) : (
              <WorkspaceFileCodeView text={fileText} lineCount={lineCount} />
            )}
          </div>
          {!file.binary ? (
            <DialogRoot open={dialogOpen} onOpenChange={setDialogOpen}>
              <DialogContent className="workspace-preview-dialog">
                <DialogHeader className="workspace-preview-dialog-header">
                  <div className="workspace-preview-dialog-heading">
                    <DialogTitle>{file.path}</DialogTitle>
                    <DialogDescription className="sr-only">{file.path}</DialogDescription>
                  </div>
                  <div className="workspace-preview-dialog-actions">
                    {markdownFile ? (
                      <div className="workspace-preview-mode-switch" role="tablist" aria-label={viewToggleLabel}>
                        <Tooltip content={previewText}>
                          <Button
                            role="tab"
                            aria-selected={activeDialogMode === "preview"}
                            active={activeDialogMode === "preview"}
                            className="workspace-preview-mode-button"
                            size="sm"
                            variant="tertiaryGray"
                            onClick={() => setDialogMode("preview")}
                          >
                            {previewText}
                          </Button>
                        </Tooltip>
                        <Tooltip content={codeText}>
                          <Button
                            role="tab"
                            aria-selected={activeDialogMode === "code"}
                            active={activeDialogMode === "code"}
                            className="workspace-preview-mode-button"
                            size="sm"
                            variant="tertiaryGray"
                            onClick={() => setDialogMode("code")}
                          >
                            {codeText}
                          </Button>
                        </Tooltip>
                      </div>
                    ) : null}
                    <Tooltip content={closeText}>
                      <DialogClose asChild>
                        <button type="button" className="workspace-preview-close-button" aria-label={closeText}>
                          <X size={18} strokeWidth={2} aria-hidden="true" />
                        </button>
                      </DialogClose>
                    </Tooltip>
                  </div>
                </DialogHeader>
                <DialogBody className="workspace-preview-dialog-body">
                  {activeDialogMode === "preview" ? (
                    <div
                      className="workspace-preview-markdown"
                      dangerouslySetInnerHTML={{ __html: renderedMarkdown }}
                    />
                  ) : (
                    <WorkspaceFileCodeView
                      className="workspace-preview-dialog-code-view"
                      text={fileText}
                      lineCount={lineCount}
                    />
                  )}
                </DialogBody>
              </DialogContent>
            </DialogRoot>
          ) : null}
        </>
      )}
    </div>
  );
}

function WorkspaceFileCodeView({
  className = "",
  lineCount,
  text,
}: {
  className?: string;
  lineCount?: number;
  text: string;
}) {
  const count = lineCount ?? (text ? text.split(/\r\n|\r|\n/).length : 0);
  return (
    <div className={`workspace-preview-code-shell ${className}`.trim()}>
      <pre className="workspace-preview-line-numbers">
        {Array.from({ length: count }, (_, index) => index + 1).join("\n")}
      </pre>
      <pre className="workspace-preview-code">{text}</pre>
    </div>
  );
}

function isMarkdownPath(path: string): boolean {
  const normalized = path.trim().toLowerCase();
  return normalized.endsWith(".md") || normalized.endsWith(".markdown");
}
