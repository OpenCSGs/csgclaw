import { useCallback, useEffect, useRef, useState, type WheelEvent } from "react";
import { ChevronLeft, ChevronRight, FileText, X } from "lucide-react";
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
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [scrollState, setScrollState] = useState({
    canScrollLeft: false,
    canScrollRight: false,
    overflowing: false,
  });

  const updateScrollState = useCallback(() => {
    const element = scrollRef.current;
    if (!element) {
      return;
    }
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
    if (!element) {
      return undefined;
    }
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => updateScrollState());
    observer?.observe(element);
    window.addEventListener("resize", updateScrollState);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateScrollState);
    };
  }, [drafts.length, updateScrollState]);

  if (drafts.length === 0) {
    return null;
  }

  function scrollByAttachment(direction: -1 | 1) {
    const element = scrollRef.current;
    if (!element) {
      return;
    }
    const firstAttachment = element.querySelector<HTMLElement>(".attachment-draft");
    const gap = Number.parseFloat(window.getComputedStyle(element).columnGap || "0") || 0;
    const attachmentWidth = firstAttachment?.getBoundingClientRect().width || Math.max(element.clientWidth * 0.75, 160);
    element.scrollLeft += direction * (attachmentWidth + gap);
    updateScrollState();
  }

  function handleWheel(event: WheelEvent<HTMLDivElement>) {
    const element = scrollRef.current;
    if (!element || !scrollState.overflowing || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) {
      return;
    }
    const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
    const nextScrollLeft = Math.max(0, Math.min(maxScrollLeft, element.scrollLeft + event.deltaY));
    if (nextScrollLeft === element.scrollLeft) {
      return;
    }
    event.preventDefault();
    element.scrollLeft = nextScrollLeft;
    updateScrollState();
  }

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
        {drafts.map((draft) => (
          <AttachmentDraftItem key={draft.id} draft={draft} t={t} onRemove={onRemove} />
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
