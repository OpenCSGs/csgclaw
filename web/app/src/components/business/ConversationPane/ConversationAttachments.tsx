import { useEffect, useState } from "react";
import { FileText, X } from "lucide-react";
import { resolveRequestPath } from "@/api/client";
import { Button, Tooltip } from "@/components/ui";
import {
  formatAttachmentSize,
  isImageAttachment,
  type AttachmentDraft,
  type MessageAttachment,
} from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

export function AttachmentDraftStrip({
  drafts,
  t,
  onRemove,
}: {
  drafts: readonly AttachmentDraft[];
  t: TranslateFn;
  onRemove: (id: string) => void;
}) {
  if (drafts.length === 0) {
    return null;
  }
  return (
    <div className="attachment-draft-strip" role="list" aria-label={t("attachments")}>
      {drafts.map((draft) => (
        <AttachmentDraftItem key={draft.id} draft={draft} t={t} onRemove={onRemove} />
      ))}
    </div>
  );
}

function AttachmentDraftItem({
  draft,
  t,
  onRemove,
}: {
  draft: AttachmentDraft;
  t: TranslateFn;
  onRemove: (id: string) => void;
}) {
  const previewURL = useObjectURL(isImageAttachment(draft) ? draft.file : null);
  const removeLabel = t("removeAttachmentNamed", { name: draft.name });
  return (
    <div className={`attachment-draft ${isImageAttachment(draft) ? "is-image" : ""}`.trim()} role="listitem">
      {previewURL ? (
        <img className="attachment-draft-preview" src={previewURL} alt="" />
      ) : (
        <span className="attachment-file-icon" aria-hidden="true">
          <FileText size={18} />
        </span>
      )}
      <span className="attachment-draft-meta">
        <span className="attachment-name truncate" title={draft.name}>
          {draft.name}
        </span>
        <span className="attachment-size">{formatAttachmentSize(draft.sizeBytes)}</span>
      </span>
      <Tooltip content={removeLabel}>
        <Button
          aria-label={removeLabel}
          className="attachment-remove-button"
          iconOnly
          size="sm"
          variant="tertiaryGray"
          onClick={() => onRemove(draft.id)}
        >
          <X aria-hidden="true" size={14} />
        </Button>
      </Tooltip>
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
