import { useEffect, useState } from "react";
import { FileText, X } from "lucide-react";
import { resolveRequestPath } from "@/api/client";
import { Button, Tooltip } from "@/components/ui";
import {
  formatAttachmentSize,
  isImageAttachment,
  splitAttachmentFilename,
  type AttachmentDraft,
  type MessageAttachment,
} from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

type AttachmentDraftStatus = "failed" | "idle" | "uploading";

export function AttachmentDraftStrip({
  drafts,
  progress = 0,
  status = "idle",
  t,
  onRemove,
}: {
  drafts: readonly AttachmentDraft[];
  progress?: number;
  status?: AttachmentDraftStatus;
  t: TranslateFn;
  onRemove: (id: string) => void;
}) {
  if (drafts.length === 0) {
    return null;
  }
  return (
    <div className="attachment-draft-strip" role="list" aria-label={t("attachments")}>
      {drafts.map((draft) => (
        <AttachmentDraftItem
          key={draft.id}
          draft={draft}
          progress={progress}
          status={status}
          t={t}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}

function AttachmentDraftItem({
  draft,
  progress,
  status,
  t,
  onRemove,
}: {
  draft: AttachmentDraft;
  progress: number;
  status: AttachmentDraftStatus;
  t: TranslateFn;
  onRemove: (id: string) => void;
}) {
  const previewURL = useObjectURL(draft.file);
  const removeLabel = t("removeAttachmentNamed", { name: draft.name });
  const previewLabel = t("previewAttachmentNamed", { name: draft.name });
  const filename = splitAttachmentFilename(draft.name);
  return (
    <div
      className={`attachment-draft ${isImageAttachment(draft) ? "is-image" : ""} is-${status}`.trim()}
      role="listitem"
    >
      <a
        aria-label={previewLabel}
        className="attachment-draft-preview-link"
        href={previewURL || undefined}
        target="_blank"
        rel="noreferrer"
        title={previewLabel}
      >
        {isImageAttachment(draft) && previewURL ? (
          <img className="attachment-draft-preview" src={previewURL} alt="" />
        ) : (
          <span className="attachment-file-icon" aria-hidden="true">
            <FileText size={18} />
          </span>
        )}
        <span className="attachment-draft-meta">
          <span className="attachment-name" title={draft.name}>
            <span className="attachment-name-stem">{filename.stem}</span>
            {filename.extension ? <span className="attachment-name-extension">{filename.extension}</span> : null}
          </span>
          <span className="attachment-size">
            {status === "uploading"
              ? t("attachmentUploadingProgress", { progress: Math.max(0, Math.min(100, Math.round(progress))) })
              : status === "failed"
                ? t("attachmentUploadFailed")
                : formatAttachmentSize(draft.sizeBytes)}
          </span>
        </span>
      </a>
      <Tooltip content={removeLabel}>
        <Button
          aria-label={removeLabel}
          className="attachment-remove-button"
          disabled={status === "uploading"}
          iconOnly
          size="sm"
          variant="tertiaryGray"
          onClick={() => onRemove(draft.id)}
        >
          <X aria-hidden="true" size={14} />
        </Button>
      </Tooltip>
      {status === "uploading" ? (
        <span
          className="attachment-upload-progress"
          role="progressbar"
          aria-label={t("attachmentUploadingNamed", { name: draft.name })}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.max(0, Math.min(100, Math.round(progress)))}
        >
          <span style={{ width: `${Math.max(0, Math.min(100, progress))}%` }} />
        </span>
      ) : null}
    </div>
  );
}

export function MessageAttachments({
  attachments,
  t,
}: {
  attachments?: readonly MessageAttachment[] | null;
  t: TranslateFn;
}) {
  if (!attachments?.length) {
    return null;
  }
  const images = attachments.filter(isImageAttachment);
  const files = attachments.filter((attachment) => !isImageAttachment(attachment));
  return (
    <div className="message-attachments">
      {images.length > 0 ? (
        <div className="message-attachment-grid">
          {images.map((attachment) => {
            const downloadURL = resolveRequestPath(attachment.download_url);
            const previewURL = resolveRequestPath(attachment.preview_url || attachment.download_url);
            return (
              <Tooltip key={attachment.id} content={attachment.name}>
                <a className="message-image-attachment" href={downloadURL} target="_blank" rel="noreferrer">
                  <img
                    src={previewURL}
                    alt={attachment.name}
                    decoding="async"
                    loading="lazy"
                    referrerPolicy="no-referrer"
                  />
                </a>
              </Tooltip>
            );
          })}
        </div>
      ) : null}
      {files.length > 0 ? (
        <div className="message-file-list">
          {files.map((attachment) => (
            <Tooltip key={attachment.id} content={attachment.name}>
              <a
                className="message-file-attachment"
                href={resolveRequestPath(attachment.download_url)}
                download
                referrerPolicy="no-referrer"
              >
                <span className="attachment-file-icon" aria-hidden="true">
                  <FileText size={18} />
                </span>
                <span className="attachment-draft-meta">
                  <span className="attachment-name truncate">{attachment.name || t("attachment")}</span>
                  <span className="attachment-size">{formatAttachmentSize(attachment.size_bytes)}</span>
                </span>
              </a>
            </Tooltip>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function useObjectURL(file: File | null): string {
  const [url, setURL] = useState("");
  useEffect(() => {
    if (!file) {
      setURL("");
      return undefined;
    }
    const nextURL = URL.createObjectURL(file);
    setURL(nextURL);
    return () => URL.revokeObjectURL(nextURL);
  }, [file]);
  return url;
}
