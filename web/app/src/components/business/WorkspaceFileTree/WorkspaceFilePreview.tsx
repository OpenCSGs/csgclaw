import workspaceFilePreviewStyles from "./WorkspaceFilePreview.module.css";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { FileSearch } from "lucide-react";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import type { WorkspaceFile } from "@/models/workspace";
import { formatFileSize } from "@/shared/lib/fileSize";
import "./WorkspaceFilePreview.css";

type WorkspaceFileDialogMode = "code" | "preview";

type WorkspaceFilePreviewProps = {
  portalContainer?: HTMLElement | null;
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
  portalContainer,
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
  const fileMeta = file?.binary
    ? binaryText
    : `${formatFileSize(file?.size)}${file?.truncated ? ` - ${truncatedText}` : ""}`;
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
        <div className={workspaceFilePreviewStyles["workspace-preview-empty-state"]}>
          {emptyIcon ? (
            <span className={workspaceFilePreviewStyles["workspace-preview-empty-icon"]}>{emptyIcon}</span>
          ) : null}
          <strong>{emptyTitle}</strong>
          <p>{emptyHint}</p>
        </div>
      ) : (
        <>
          <div className={workspaceFilePreviewStyles["workspace-preview-file-header"]}>
            <strong>{file.path}</strong>
            <div className={workspaceFilePreviewStyles["workspace-preview-file-actions"]}>
              <span className={workspaceFilePreviewStyles["workspace-preview-file-meta"]}>{fileMeta}</span>
              {!file.binary ? (
                <Tooltip content={previewText}>
                  <Button
                    iconOnly
                    aria-label={previewText}
                    className={workspaceFilePreviewStyles["workspace-preview-open-button"]}
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
              <DialogContent
                className={workspaceFilePreviewStyles["workspace-preview-dialog"]}
                portalContainer={portalContainer}
              >
                <DialogHeader className={workspaceFilePreviewStyles["workspace-preview-dialog-header"]}>
                  <div className={workspaceFilePreviewStyles["workspace-preview-dialog-heading"]}>
                    <DialogTitle>{file.path}</DialogTitle>
                    <DialogDescription className="sr-only">{file.path}</DialogDescription>
                  </div>
                  <div className={workspaceFilePreviewStyles["workspace-preview-dialog-actions"]}>
                    {markdownFile ? (
                      <div
                        className={workspaceFilePreviewStyles["workspace-preview-mode-switch"]}
                        role="tablist"
                        aria-label={viewToggleLabel}
                      >
                        <Tooltip content={previewText}>
                          <Button
                            role="tab"
                            aria-selected={activeDialogMode === "preview"}
                            active={activeDialogMode === "preview"}
                            className={workspaceFilePreviewStyles["workspace-preview-mode-button"]}
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
                            className={workspaceFilePreviewStyles["workspace-preview-mode-button"]}
                            size="sm"
                            variant="tertiaryGray"
                            onClick={() => setDialogMode("code")}
                          >
                            {codeText}
                          </Button>
                        </Tooltip>
                      </div>
                    ) : null}
                    <DialogCloseButton label={closeText} size="sm" variant="tertiaryGray" iconOnly />
                  </div>
                </DialogHeader>
                <DialogBody className={workspaceFilePreviewStyles["workspace-preview-dialog-body"]}>
                  {activeDialogMode === "preview" ? (
                    <div
                      className={workspaceFilePreviewStyles["workspace-preview-markdown"]}
                      dangerouslySetInnerHTML={{ __html: renderedMarkdown }}
                    />
                  ) : (
                    <WorkspaceFileCodeView
                      className={workspaceFilePreviewStyles["workspace-preview-dialog-code-view"]}
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
    <div className={`${workspaceFilePreviewStyles["workspace-preview-code-shell"]} ${className}`.trim()}>
      <pre className={workspaceFilePreviewStyles["workspace-preview-line-numbers"]}>
        {Array.from({ length: count }, (_, index) => index + 1).join("\n")}
      </pre>
      <pre className={workspaceFilePreviewStyles["workspace-preview-code"]}>{text}</pre>
    </div>
  );
}

function isMarkdownPath(path: string): boolean {
  const normalized = path.trim().toLowerCase();
  return normalized.endsWith(".md") || normalized.endsWith(".markdown");
}
