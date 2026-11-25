import { useEffect, useId, useRef, useState } from "react";
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
  LinkButton,
} from "@/components/ui";
import type { FeedbackResult } from "@/api/feedback";
import type { TranslateFn } from "@/models/conversations";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import {
  FEEDBACK_ATTACHMENT_ACCEPT,
  MAX_FEEDBACK_ARCHIVES,
  MAX_FEEDBACK_DESCRIPTION_LENGTH,
  MAX_FEEDBACK_IMAGES,
} from "@/models/feedback";
import styles from "./FeedbackDialog.module.css";
import type { FeedbackAttachmentRejection } from "./useFeedback";

type Props = {
  open: boolean;
  returnFocusTo?: HTMLElement | null;
  busy: boolean;
  description: string;
  images: File[];
  archives: File[];
  receipt: FeedbackResult | null;
  error: string;
  ready?: boolean;
  storageUnavailable?: boolean;
  requestID?: string;
  awaitingStatus?: boolean;
  checking?: boolean;
  site?: string;
  rejections?: FeedbackAttachmentRejection[];
  t: TranslateFn;
  onOpenChange: (open: boolean) => void;
  onDescriptionChange: (value: string) => void;
  onAddAttachments: (files: File[]) => void;
  onRemoveAttachment: (kind: "image" | "archive", index: number) => void;
  onClearRejections?: () => void;
  onCheckStatus?: () => void;
  onDiscardDraft?: () => void;
  onSubmit: () => void;
};

export function FeedbackDialog(props: Props) {
  const { t, images, archives, busy, open, receipt, onAddAttachments } = props;
  const loading = props.ready === false;
  const awaiting = Boolean(props.awaitingStatus);
  const editsDisabled = busy || loading || awaiting;
  const attachmentHintID = useId();
  const [discarding, setDiscarding] = useState(false);
  const [copyResult, setCopyResult] = useState<"copied" | "failed">();
  const issueURL = safeFeedbackURL(receipt?.issue_url);
  const issueNumber = receipt?.issue_id ? `#${receipt.issue_id}` : "";
  const site = safeFeedbackURL(props.site)?.host;
  const attachmentInputRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);
  const addAttachments = useRef(onAddAttachments);
  useEffect(() => {
    addAttachments.current = onAddAttachments;
  }, [onAddAttachments]);
  useEffect(() => {
    if (!open || editsDisabled || receipt) return;
    function paste(event: ClipboardEvent) {
      const files = Array.from(event.clipboardData?.files || []);
      if (!files.length) return;
      event.preventDefault();
      addAttachments.current(files);
    }
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  }, [open, editsDisabled, receipt]);
  useEffect(() => {
    if (!open || !awaiting) setDiscarding(false);
    if (!open || !receipt) setCopyResult(undefined);
  }, [open, awaiting, receipt]);

  return (
    <DialogRoot
      open={props.open}
      onOpenChange={(open) => {
        if (!busy && !props.checking) props.onOpenChange(open);
      }}
    >
      <DialogContent
        className={styles.dialog}
        scrollBody
        onDragOver={(event) => {
          if (!event.dataTransfer.types.includes("Files")) return;
          event.preventDefault();
          event.dataTransfer.dropEffect = editsDisabled || receipt ? "none" : "copy";
        }}
        onDrop={(event) => {
          if (!event.dataTransfer.files.length) return;
          event.preventDefault();
          if (!editsDisabled && !receipt) onAddAttachments(Array.from(event.dataTransfer.files));
        }}
        onOpenAutoFocus={() => {
          triggerRef.current =
            props.returnFocusTo ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
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
        {receipt ? (
          issueNumber || issueURL ? (
            <DialogBody className={styles.body}>
              <div className={styles.inlineActions}>
                {issueNumber ? <span>{t("feedbackIssueNumber", { number: issueNumber })}</span> : null}
                {issueNumber ? (
                  <Button
                    variant="secondaryGray"
                    size="sm"
                    onClick={async () => {
                      setCopyResult((await writeTextToClipboard(issueNumber)) ? "copied" : "failed");
                    }}
                  >
                    {t("feedbackCopyIssueNumber")}
                  </Button>
                ) : null}
                {issueURL ? (
                  <LinkButton
                    href={issueURL.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="secondaryGray"
                    size="sm"
                  >
                    {t("feedbackViewIssue")}
                  </LinkButton>
                ) : null}
              </div>
              {copyResult ? (
                <p role="status" className={styles.imageHint}>
                  {t(copyResult === "copied" ? "feedbackIssueCopied" : "feedbackIssueCopyFailed")}
                </p>
              ) : null}
            </DialogBody>
          ) : null
        ) : loading ? (
          <DialogBody>
            <p className={styles.imageHint} role="status">
              {t("feedbackDraftLoading")}
            </p>
          </DialogBody>
        ) : (
          <DialogBody className={styles.body}>
            {!busy && awaiting ? (
              <div className={styles.inlineActions}>
                <div role="status" className={styles.submissionStatus}>
                  <p>{props.error || t("feedbackOutcomeUnknown")}</p>
                  {props.requestID ? (
                    <p>
                      {t("feedbackSubmissionReference")} <code>{props.requestID}</code>
                    </p>
                  ) : null}
                </div>
                {!props.checking &&
                  (discarding ? (
                    <>
                      <p>{t("feedbackDiscardUnconfirmedHint")}</p>
                      <Button variant="secondaryGray" size="sm" onClick={() => setDiscarding(false)}>
                        {t("cancel")}
                      </Button>
                      <Button variant="outlineDanger" size="sm" onClick={props.onDiscardDraft}>
                        {t("feedbackDiscardDraft")}
                      </Button>
                    </>
                  ) : (
                    <Button variant="tertiaryGray" size="sm" onClick={() => setDiscarding(true)}>
                      {t("feedbackStartAnother")}
                    </Button>
                  ))}
              </div>
            ) : null}
            {props.storageUnavailable ? (
              <InlineError role="status">{t("feedbackDraftStorageUnavailable")}</InlineError>
            ) : null}
            <label className={styles.label}>
              {t("settingsFeedbackLabel")}
              <TextArea
                value={props.description}
                disabled={busy}
                readOnly={awaiting}
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
                disabled={editsDisabled}
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
                disabled={
                  editsDisabled || (images.length >= MAX_FEEDBACK_IMAGES && archives.length >= MAX_FEEDBACK_ARCHIVES)
                }
                aria-describedby={attachmentHintID}
                onClick={() => attachmentInputRef.current?.click()}
              >
                {t("feedbackChooseAttachments")}
              </Button>
              <p id={attachmentHintID} className={styles.imageHint}>
                {t("feedbackAttachmentHint")}
                <br />
                {t("feedbackImageLimit")} ({images.length}/{MAX_FEEDBACK_IMAGES})
                <br />
                {t("feedbackArchiveLimit")} ({archives.length}/{MAX_FEEDBACK_ARCHIVES})
              </p>
              <p className={styles.imageHint}>
                {site ? t("feedbackArchiveDestination", { site }) : t("feedbackArchiveDestinationGeneric")}
              </p>
              {props.rejections?.length ? (
                <div className={styles.label}>
                  <InlineError role="alert" className={styles.notice}>
                    <p>{t("feedbackRejectedAttachments")}</p>
                    <ul className={styles.rejections}>
                      {props.rejections.map(({ file, reason }, index) => (
                        <li key={`${index}-${file}`}>
                          {file || t("feedbackUnnamedAttachment")}: {t(reason)}
                        </li>
                      ))}
                    </ul>
                  </InlineError>
                  <Button
                    className={styles.chooseFilesButton}
                    variant="secondaryGray"
                    size="sm"
                    onClick={props.onClearRejections}
                    disabled={editsDisabled}
                  >
                    {t("feedbackClearAttachmentWarnings")}
                  </Button>
                </div>
              ) : null}
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
                    disabled={editsDisabled}
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
                      disabled={editsDisabled}
                      aria-label={t("feedbackRemoveAttachment", { file: file.name })}
                      onClick={() => props.onRemoveAttachment("archive", index)}
                    >
                      <X size={14} aria-hidden="true" />
                    </Button>
                  </li>
                ))}
              </ul>
            ) : null}
            {props.error && !awaiting ? <InlineError role="alert">{props.error}</InlineError> : null}
          </DialogBody>
        )}
        <DialogFooter className={styles.footer}>
          <Button
            variant="secondaryGray"
            size="md"
            disabled={busy || props.checking}
            onClick={() => props.onOpenChange(false)}
          >
            {t("close")}
          </Button>
          {receipt || loading ? null : awaiting && !busy ? (
            <Button
              variant="primary"
              size="md"
              loading={props.checking}
              disabled={!props.requestID}
              onClick={props.onCheckStatus}
            >
              {t("feedbackCheckStatus")}
            </Button>
          ) : (
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

function safeFeedbackURL(value?: string): URL | undefined {
  if (!value) return;
  try {
    const url = new URL(value);
    if ((url.protocol === "https:" || url.protocol === "http:") && !url.username && !url.password) return url;
  } catch {
    // A malformed or unsafe URL cannot be displayed as a link.
  }
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
