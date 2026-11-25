import { useEffect, useRef, useState } from "react";
import { FileArchive, X } from "lucide-react";
import {
  TextArea,
  Button,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  InlineError,
} from "@/components/ui";
import type { FeedbackResult } from "@/api/feedback";
import type { TranslateFn } from "@/models/conversations";
import {
  FEEDBACK_ATTACHMENT_ACCEPT,
  MAX_FEEDBACK_ARCHIVES,
  MAX_FEEDBACK_DESCRIPTION_LENGTH,
  MAX_FEEDBACK_IMAGES,
} from "@/models/feedback";
import styles from "./FeedbackDialog.module.css";

type Props = {
  open: boolean;
  busy: boolean;
  description: string;
  images: File[];
  archives: File[];
  receipt: FeedbackResult | null;
  error: string;
  t: TranslateFn;
  onOpenChange: (open: boolean) => void;
  onDescriptionChange: (value: string) => void;
  onAddAttachments: (files: File[]) => void;
  onRemoveAttachment: (kind: "image" | "archive", index: number) => void;
  onSubmit: () => void;
};

export function FeedbackDialog(props: Props) {
  const { t, images, archives, busy, open, receipt, onAddAttachments } = props;
  const attachmentInputRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);
  const addAttachments = useRef(onAddAttachments);
  useEffect(() => {
    addAttachments.current = onAddAttachments;
  }, [onAddAttachments]);
  useEffect(() => {
    if (!open || busy || receipt) return;
    function paste(event: ClipboardEvent) {
      const files = Array.from(event.clipboardData?.files || []);
      if (!files.length) return;
      event.preventDefault();
      addAttachments.current(files);
    }
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  }, [open, busy, receipt]);

  return (
    <DialogRoot
      open={props.open}
      onOpenChange={(open) => {
        if (!busy) props.onOpenChange(open);
      }}
    >
      <DialogContent
        className={styles.dialog}
        scrollBody
        onDragOver={(event) => {
          if (!event.dataTransfer.types.includes("Files")) return;
          event.preventDefault();
          event.dataTransfer.dropEffect = busy || receipt ? "none" : "copy";
        }}
        onDrop={(event) => {
          if (!event.dataTransfer.files.length) return;
          event.preventDefault();
          if (!busy && !receipt) onAddAttachments(Array.from(event.dataTransfer.files));
        }}
        onOpenAutoFocus={() => {
          triggerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus({ preventScroll: true });
        }}
      >
        <DialogHeader className={styles.header}>
          <DialogTitle>{t("feedbackTitle")}</DialogTitle>
          <DialogDescription>
            {receipt ? null : t("settingsFeedbackDescription")}
            <span role="status">{receipt ? t("feedbackSuccess") : null}</span>
          </DialogDescription>
        </DialogHeader>
        {receipt ? null : (
          <DialogBody className={styles.body}>
            <label className={styles.label}>
              {t("settingsFeedbackLabel")}
              <TextArea
                value={props.description}
                disabled={busy}
                rows={6}
                maxLength={MAX_FEEDBACK_DESCRIPTION_LENGTH}
                placeholder={t("settingsFeedbackPlaceholder")}
                onChange={(event) => props.onDescriptionChange(event.target.value)}
              />
            </label>
            <div className={styles.label}>
              <input
                ref={attachmentInputRef}
                className={styles.fileInput}
                type="file"
                accept={FEEDBACK_ATTACHMENT_ACCEPT}
                multiple
                disabled={busy}
                tabIndex={-1}
                aria-hidden="true"
                onChange={(event) => {
                  onAddAttachments(Array.from(event.target.files || []));
                  event.target.value = "";
                }}
              />
              <Button
                variant="secondaryGray"
                size="sm"
                className={styles.chooseFilesButton}
                disabled={busy || (images.length >= MAX_FEEDBACK_IMAGES && archives.length >= MAX_FEEDBACK_ARCHIVES)}
                onClick={() => attachmentInputRef.current?.click()}
              >
                {t("feedbackChooseAttachments")}
              </Button>
              <p className={styles.imageHint}>
                {t("feedbackAttachmentHint")}
                <br />
                {t("feedbackImageLimit")} ({images.length}/{MAX_FEEDBACK_IMAGES})
                <br />
                {t("feedbackArchiveLimit")} ({archives.length}/{MAX_FEEDBACK_ARCHIVES})
              </p>
            </div>
            <div className={styles.images}>
              {images.map((file, index) => (
                <div className={styles.image} key={`${index}-${file.name}`}>
                  <Preview file={file} />
                  <Button
                    iconOnly
                    variant="secondaryGray"
                    size="sm"
                    className={styles.removeImage}
                    disabled={busy}
                    aria-label={`${t("feedbackRemoveImage")} ${index + 1}`}
                    onClick={() => props.onRemoveAttachment("image", index)}
                  >
                    <X size={14} strokeWidth={2} aria-hidden="true" />
                  </Button>
                </div>
              ))}
            </div>
            {archives.length ? (
              <ul className={styles.archives}>
                {archives.map((file, index) => (
                  <li className={styles.archive} key={`${index}-${file.name}`}>
                    <FileArchive size={16} aria-hidden="true" />
                    <span title={file.name}>{file.name}</span>
                    <Button
                      iconOnly
                      variant="secondaryGray"
                      size="sm"
                      disabled={busy}
                      aria-label={t("feedbackRemoveAttachment", { file: file.name })}
                      onClick={() => props.onRemoveAttachment("archive", index)}
                    >
                      <X size={14} aria-hidden="true" />
                    </Button>
                  </li>
                ))}
              </ul>
            ) : null}
            {props.error ? <InlineError role="alert">{props.error}</InlineError> : null}
          </DialogBody>
        )}
        <DialogFooter>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => props.onOpenChange(false)}>
            {t("close")}
          </Button>
          {receipt ? null : (
            <Button
              variant="primary"
              size="md"
              loading={busy}
              disabled={busy || (!props.description.trim() && !images.length && !archives.length)}
              onClick={props.onSubmit}
            >
              {t("feedbackSubmit")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

function Preview({ file }: { file: File }) {
  const [url, setURL] = useState("");
  useEffect(() => {
    const next = URL.createObjectURL(file);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return <img src={url || undefined} alt={file.name} />;
}
