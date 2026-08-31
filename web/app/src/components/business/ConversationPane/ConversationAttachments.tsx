import { useCallback, useEffect, useRef, useState, type WheelEvent } from "react";
import { ChevronLeft, ChevronRight, FileText, X } from "lucide-react";
import { resolveRequestPath } from "@/api/client";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { Button, Tooltip } from "@/components/ui";
import {
  formatAttachmentSize,
  isImageAttachment,
  splitAttachmentFilename,
  type AttachmentDraft,
  type AttachmentPreviewItem,
  type MessageAttachment,
} from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

type AttachmentDraftStatus = "failed" | "idle" | "uploading";

export function AttachmentDraftStrip({
  drafts,
  progress = 0,
  status = "idle",
  t,
  onPreviewAttachment,
  onRemove,
}: {
  drafts: readonly AttachmentDraft[];
  progress?: number;
  status?: AttachmentDraftStatus;
  t: TranslateFn;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  onRemove: (id: string) => void;
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [scrollState, setScrollState] = useState({
    canScrollLeft: false,
    canScrollRight: false,
    overflowing: false,
  });
  const updateScrollState = useCallback(() => {
    const element = scrollRef.current;
    if (!element) return;
    const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
    const nextState = {
      canScrollLeft: element.scrollLeft > 1,
      canScrollRight: element.scrollLeft < maxScrollLeft - 1,
      overflowing: maxScrollLeft > 1,
    };
    setScrollState((current) =>
      current.canScrollLeft === nextState.canScrollLeft &&
      current.canScrollRight === nextState.canScrollRight &&
      current.overflowing === nextState.overflowing
        ? current
        : nextState,
    );
  }, []);

  useEffect(() => {
    updateScrollState();
    const element = scrollRef.current;
    if (!element) return undefined;
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(updateScrollState);
    observer?.observe(element);
    window.addEventListener("resize", updateScrollState);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateScrollState);
    };
  }, [drafts.length, updateScrollState]);

  if (drafts.length === 0) return null;

  function scrollByAttachment(direction: -1 | 1) {
    const element = scrollRef.current;
    if (!element) return;
    const firstAttachment = element.querySelector<HTMLElement>(".attachment-draft");
    const gap = Number.parseFloat(window.getComputedStyle(element).columnGap || "0") || 0;
    const attachmentWidth = firstAttachment?.getBoundingClientRect().width || Math.max(element.clientWidth * 0.75, 160);
    element.scrollLeft += direction * (attachmentWidth + gap);
    updateScrollState();
  }

  function handleWheel(event: WheelEvent<HTMLDivElement>) {
    const element = scrollRef.current;
    if (!element || !scrollState.overflowing || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return;
    const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
    const nextScrollLeft = Math.max(0, Math.min(maxScrollLeft, element.scrollLeft + event.deltaY));
    if (nextScrollLeft === element.scrollLeft) return;
    event.preventDefault();
    element.scrollLeft = nextScrollLeft;
    updateScrollState();
  }

  const previewItems = drafts.map(draftPreviewItem);
  const shellClassName = [
    "attachment-draft-strip-shell",
    scrollState.canScrollLeft ? "can-scroll-left" : "",
    scrollState.canScrollRight ? "can-scroll-right" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={shellClassName}>
      <div
        ref={scrollRef}
        className="attachment-draft-strip"
        role="list"
        aria-label={t("attachments")}
        onScroll={updateScrollState}
        onWheel={handleWheel}
      >
        {drafts.map((draft, index) => (
          <AttachmentDraftItem
            key={draft.id}
            draft={draft}
            progress={progress}
            status={status}
            t={t}
            onPreview={(anchor) => onPreviewAttachment?.({ anchor, index, items: previewItems })}
            onRemove={onRemove}
          />
        ))}
      </div>
      {scrollState.canScrollLeft ? <span className="attachment-scroll-fade is-previous" aria-hidden="true" /> : null}
      {scrollState.canScrollRight ? <span className="attachment-scroll-fade is-next" aria-hidden="true" /> : null}
      {scrollState.canScrollLeft ? (
        <Tooltip content={t("attachmentsScrollPrevious")}>
          <Button
            aria-label={t("attachmentsScrollPrevious")}
            className="attachment-scroll-button is-previous"
            iconOnly
            size="sm"
            variant="secondaryGray"
            onClick={() => scrollByAttachment(-1)}
          >
            <ChevronLeft aria-hidden="true" size={16} />
          </Button>
        </Tooltip>
      ) : null}
      {scrollState.canScrollRight ? (
        <Tooltip content={t("attachmentsScrollNext")}>
          <Button
            aria-label={t("attachmentsScrollNext")}
            className="attachment-scroll-button is-next"
            iconOnly
            size="sm"
            variant="secondaryGray"
            onClick={() => scrollByAttachment(1)}
          >
            <ChevronRight aria-hidden="true" size={16} />
          </Button>
        </Tooltip>
      ) : null}
    </div>
  );
}

function AttachmentDraftItem({
  draft,
  progress,
  status,
  t,
  onPreview,
  onRemove,
}: {
  draft: AttachmentDraft;
  progress: number;
  status: AttachmentDraftStatus;
  t: TranslateFn;
  onPreview: (anchor: HTMLElement) => void;
  onRemove: (id: string) => void;
}) {
  const previewURL = useObjectURL(draft.file);
  const removeLabel = t("removeAttachmentNamed", { name: draft.name });
  const previewLabel = t("previewAttachmentNamed", { name: draft.name });
  const filename = splitAttachmentFilename(draft.name);
  const size =
    status === "uploading"
      ? t("attachmentUploadingProgress", { progress: Math.max(0, Math.min(100, Math.round(progress))) })
      : status === "failed"
        ? t("attachmentUploadFailed")
        : formatAttachmentSize(draft.sizeBytes);
  return (
    <div
      className={`attachment-draft ${isImageAttachment(draft) ? "is-image" : ""} is-${status}`.trim()}
      role="listitem"
    >
      <button
        type="button"
        aria-label={previewLabel}
        className="attachment-draft-preview-link"
        title={previewLabel}
        onClick={(event) => onPreview(event.currentTarget)}
      >
        {isImageAttachment(draft) && previewURL ? (
          <img className="attachment-draft-preview" src={previewURL} alt="" />
        ) : (
          <span className="attachment-file-icon" aria-hidden="true">
            <FileText size={18} />
          </span>
        )}
        <AttachmentMeta name={draft.name} size={size} filename={filename} />
      </button>
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
  onPreviewAttachment,
}: {
  attachments?: readonly MessageAttachment[] | null;
  t: TranslateFn;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
}) {
  if (!attachments?.length) return null;
  const items = attachments.map(messagePreviewItem);
  const indexed = attachments.map((attachment, index) => ({ attachment, index }));
  const images = indexed.filter(({ attachment }) => isImageAttachment(attachment));
  const files = indexed.filter(({ attachment }) => !isImageAttachment(attachment));
  return (
    <div className="message-attachments">
      {images.length > 0 ? (
        <div className="message-attachment-grid">
          {images.map(({ attachment, index }) => (
            <Tooltip key={attachment.id} content={attachment.name}>
              <button
                type="button"
                className="message-image-attachment"
                aria-label={t("previewAttachmentNamed", { name: attachment.name })}
                onClick={(event) => onPreviewAttachment?.({ anchor: event.currentTarget, index, items })}
              >
                <img
                  src={resolveRequestPath(attachment.preview_url || attachment.download_url)}
                  alt={attachment.name}
                  decoding="async"
                  loading="lazy"
                  referrerPolicy="no-referrer"
                />
              </button>
            </Tooltip>
          ))}
        </div>
      ) : null}
      {files.length > 0 ? (
        <div className="message-file-list">
          {files.map(({ attachment, index }) => {
            const filename = splitAttachmentFilename(attachment.name || t("attachment"));
            return (
              <Tooltip key={attachment.id} content={attachment.name}>
                <button
                  type="button"
                  className="message-file-attachment"
                  aria-label={t("previewAttachmentNamed", { name: attachment.name })}
                  onClick={(event) => onPreviewAttachment?.({ anchor: event.currentTarget, index, items })}
                >
                  <span className="attachment-file-icon" aria-hidden="true">
                    <FileText size={18} />
                  </span>
                  <AttachmentMeta
                    name={attachment.name || t("attachment")}
                    size={formatAttachmentSize(attachment.size_bytes)}
                    filename={filename}
                  />
                </button>
              </Tooltip>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}

function AttachmentMeta({
  name,
  size,
  filename,
}: {
  name: string;
  size: string;
  filename: ReturnType<typeof splitAttachmentFilename>;
}) {
  return (
    <span className="attachment-draft-meta">
      <span className="attachment-name" title={name}>
        <span className="attachment-name-stem">{filename.stem}</span>
        {filename.extension ? <span className="attachment-name-extension">{filename.extension}</span> : null}
      </span>
      <span className="attachment-size">{size}</span>
    </span>
  );
}

function draftPreviewItem(draft: AttachmentDraft): AttachmentPreviewItem {
  return { file: draft.file, id: draft.id, mediaType: draft.mediaType, name: draft.name, sizeBytes: draft.sizeBytes };
}

function messagePreviewItem(attachment: MessageAttachment): AttachmentPreviewItem {
  return {
    downloadURL: resolveRequestPath(attachment.download_url),
    id: attachment.id,
    mediaType: attachment.media_type,
    name: attachment.name,
    previewURL: resolveRequestPath(attachment.preview_url || attachment.download_url),
    sizeBytes: attachment.size_bytes,
  };
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
