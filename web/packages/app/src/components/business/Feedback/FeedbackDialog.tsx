import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { CircleAlert, FileArchive, Info, X } from "lucide-react";
import {
  TextArea,
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  DialogTrigger,
  IconButton,
  InlineError,
  LinkButton,
  Tooltip,
} from "@/components/ui";
import type { FeedbackResult } from "@/api/feedback";
import type { TranslateFn } from "@/models/conversations";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import {
  FEEDBACK_ATTACHMENT_ACCEPT,
  MAX_FEEDBACK_ARCHIVES,
  MAX_FEEDBACK_ARCHIVE_BYTES,
  MAX_FEEDBACK_DESCRIPTION_LENGTH,
  MAX_FEEDBACK_IMAGE_BYTES,
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
  onClearError?: () => void;
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
  let submissionMessage = "";
  if (busy) submissionMessage = t("feedbackSubmitting");
  else if (props.checking) submissionMessage = t("feedbackCheckingStatus");
  else if (awaiting) submissionMessage = props.error || t("feedbackOutcomeUnknown");
  let attachmentDisabledReason = "";
  if (busy) attachmentDisabledReason = t("feedbackSubmitting");
  else if (props.checking) attachmentDisabledReason = t("feedbackCheckingStatus");
  else if (awaiting) attachmentDisabledReason = t("feedbackAttachmentsAwaitingStatus");
  else if (images.length >= MAX_FEEDBACK_IMAGES && archives.length >= MAX_FEEDBACK_ARCHIVES)
    attachmentDisabledReason = t("feedbackAttachmentsFull");
  const attachmentHintID = useId();
  const attachmentUsageID = useId();
  const archiveDestinationID = useId();
  const [discarding, setDiscarding] = useState(false);
  const [copyResult, setCopyResult] = useState<"copied" | "failed">();
  const issueURL = safeFeedbackURL(receipt?.issue_url);
  const issueNumber = receipt?.issue_id ? `#${receipt.issue_id}` : "";
  const site = safeFeedbackURL(props.site)?.host;
  const countWarnings = new Set<FeedbackAttachmentRejection["reason"]>();
  const fileRejections = props.rejections?.filter(({ reason }) => {
    if (reason !== "feedbackImageCount" && reason !== "feedbackArchiveCount") return true;
    countWarnings.add(reason);
    return false;
  });
  const bodyRef = useRef<HTMLDivElement>(null);
  const attachmentInputRef = useRef<HTMLInputElement>(null);
  const chooseFilesButtonRef = useRef<HTMLButtonElement>(null);
  const descriptionRef = useRef<HTMLTextAreaElement>(null);
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
  useEffect(() => {
    if (open && bodyRef.current && (submissionMessage || props.error)) bodyRef.current.scrollTop = 0;
  }, [open, submissionMessage, props.error]);

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
        <DialogHeader>
          <div className={styles.headerCopy}>
            <DialogTitle>{t("feedbackTitle")}</DialogTitle>
            <DialogDescription>
              {receipt ? null : t("settingsFeedbackDescription")}
              <span role="status">{receipt ? t("feedbackSuccess") : null}</span>
            </DialogDescription>
          </div>
          <DialogCloseButton
            label={t("close")}
            size="sm"
            variant="tertiaryGray"
            iconOnly
            disabled={busy || props.checking}
          />
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
          <DialogBody ref={bodyRef} className={styles.body}>
            <div role="status" className={styles.submissionStatus} data-warning={awaiting && !busy && !props.checking}>
              {submissionMessage ? (
                <>
                  <Info size={16} aria-hidden="true" />
                  <span>{submissionMessage}</span>
                </>
              ) : null}
            </div>
            {props.error && !awaiting ? (
              <FeedbackErrorNotice
                dismissLabel={t("feedbackDismissSubmissionError")}
                onDismiss={
                  props.onClearError &&
                  (() => {
                    props.onClearError?.();
                    descriptionRef.current?.focus({ preventScroll: true });
                  })
                }
              >
                {props.error}
              </FeedbackErrorNotice>
            ) : null}
            {!busy && awaiting ? (
              <>
                <div className={styles.submissionActions}>
                  {props.requestID ? (
                    <p className={styles.imageHint}>
                      {t("feedbackSubmissionReference")} <code>{props.requestID}</code>
                    </p>
                  ) : null}
                  {!discarding ? (
                    <Button
                      variant="secondaryGray"
                      size="sm"
                      disabled={props.checking}
                      onClick={() => setDiscarding(true)}
                    >
                      {t("feedbackStartAnother")}
                    </Button>
                  ) : null}
                </div>
                {discarding && !props.checking ? (
                  <div className={styles.inlineActions}>
                    <p>{t("feedbackDiscardUnconfirmedHint")}</p>
                    <Button variant="secondaryGray" size="sm" onClick={() => setDiscarding(false)}>
                      {t("cancel")}
                    </Button>
                    <Button variant="outlineDanger" size="sm" onClick={props.onDiscardDraft}>
                      {t("feedbackDiscardDraft")}
                    </Button>
                  </div>
                ) : null}
              </>
            ) : null}
            {props.storageUnavailable ? (
              <InlineError role="status">{t("feedbackDraftStorageUnavailable")}</InlineError>
            ) : null}
            <label className={styles.label}>
              {t("settingsFeedbackLabel")}
              <TextArea
                ref={descriptionRef}
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
              <div className={styles.attachmentControls}>
                <Tooltip content={attachmentDisabledReason} contentProps={{ side: "top", align: "start" }}>
                  <span
                    className={styles.chooseFilesButton}
                    role={attachmentDisabledReason ? "group" : undefined}
                    aria-label={attachmentDisabledReason ? t("feedbackChooseAttachments") : undefined}
                    tabIndex={attachmentDisabledReason ? 0 : undefined}
                  >
                    <Button
                      ref={chooseFilesButtonRef}
                      variant="secondaryGray"
                      size="sm"
                      disabled={Boolean(attachmentDisabledReason)}
                      aria-describedby={`${attachmentHintID} ${attachmentUsageID}`}
                      onClick={() => attachmentInputRef.current?.click()}
                    >
                      {t("feedbackChooseAttachments")}
                    </Button>
                  </span>
                </Tooltip>
                <p id={attachmentHintID} className={styles.imageHint}>
                  {t("feedbackAttachmentHint")}
                </p>
              </div>
              <div id={attachmentUsageID} className={`${styles.attachmentUsage} ${styles.imageHint}`}>
                <span>
                  {t("feedbackImageUsage", {
                    count: images.length,
                    limit: MAX_FEEDBACK_IMAGES,
                    size: MAX_FEEDBACK_IMAGE_BYTES / 1024 ** 2,
                  })}
                </span>
                <span>
                  {t("feedbackArchiveUsage", {
                    count: archives.length,
                    limit: MAX_FEEDBACK_ARCHIVES,
                    size: MAX_FEEDBACK_ARCHIVE_BYTES / 1024 ** 2,
                  })}
                </span>
              </div>
              <details className={`${styles.attachmentFormats} ${styles.imageHint}`}>
                <summary>{t("feedbackAttachmentFormats")}</summary>
                <div className={styles.attachmentUsage}>
                  <span>{t("feedbackImageFormats")}</span>
                  <span>{t("feedbackArchiveFormats")}</span>
                </div>
              </details>
              {props.rejections?.length ? (
                <FeedbackErrorNotice
                  dismissLabel={t("feedbackDismissAttachmentWarnings")}
                  onDismiss={
                    props.onClearRejections &&
                    (() => {
                      props.onClearRejections?.();
                      const chooser = chooseFilesButtonRef.current;
                      (chooser && !chooser.disabled ? chooser : descriptionRef.current)?.focus();
                    })
                  }
                >
                  <p className={styles.noticeTitle}>{t("feedbackRejectedAttachments")}</p>
                  <ul className={styles.rejections}>
                    {Array.from(countWarnings, (reason) => (
                      <li key={reason}>{t(reason)}</li>
                    ))}
                    {fileRejections?.map(({ file, reason }, index) => (
                      <li key={`${index}-${file}`}>
                        {file || t("feedbackUnnamedAttachment")}: {t(reason)}
                      </li>
                    ))}
                  </ul>
                </FeedbackErrorNotice>
              ) : null}
            </div>
            {images.length ? (
              <div className={styles.images}>
                {images.map((file, index) => (
                  <div className={styles.image} key={`${index}-${file.name}`}>
                    <Preview file={file} t={t} />
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
            ) : null}
            {archives.length ? (
              <div className={styles.label}>
                <p id={archiveDestinationID} className={`${styles.archiveHint} ${styles.imageHint}`}>
                  <Info size={14} aria-hidden="true" />
                  <span>
                    {site ? t("feedbackArchiveDestination", { site }) : t("feedbackArchiveDestinationGeneric")}
                  </span>
                </p>
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
              </div>
            ) : null}
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
              aria-describedby={archives.length ? archiveDestinationID : undefined}
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

function FeedbackErrorNotice({
  children,
  dismissLabel,
  onDismiss,
}: {
  children: ReactNode;
  dismissLabel: string;
  onDismiss?: () => void;
}) {
  return (
    <div className={styles.errorNotice}>
      <CircleAlert className={styles.noticeIcon} size={16} aria-hidden="true" />
      <div className={styles.noticeContent} role="alert">
        {children}
      </div>
      {onDismiss ? (
        <IconButton
          variant="tertiaryGray"
          size="sm"
          label={dismissLabel}
          icon={<X />}
          iconSize={16}
          onClick={onDismiss}
        />
      ) : null}
    </div>
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

function Preview({ file, t }: { file: File; t: TranslateFn }) {
  const [url, setURL] = useState("");
  useEffect(() => {
    const next = URL.createObjectURL(file);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return (
    <DialogRoot>
      <Tooltip content={file.name} contentProps={{ side: "top" }}>
        <DialogTrigger asChild>
          <Button
            variant="ghost"
            className={styles.previewTrigger}
            aria-label={t("feedbackPreviewImage", { file: file.name })}
          >
            <img src={url || undefined} alt={file.name} />
          </Button>
        </DialogTrigger>
      </Tooltip>
      <DialogContent
        className={styles.previewDialog}
        scrollBody
        aria-describedby={undefined}
        onPaste={(event) => event.stopPropagation()}
        onDragOver={(event) => {
          event.preventDefault();
          event.stopPropagation();
          event.dataTransfer.dropEffect = "none";
        }}
        onDrop={(event) => {
          event.preventDefault();
          event.stopPropagation();
        }}
      >
        <DialogHeader>
          <DialogTitle className={styles.previewTitle} title={file.name}>
            {file.name}
          </DialogTitle>
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
        </DialogHeader>
        <DialogBody className={styles.previewBody}>
          <img src={url || undefined} alt={file.name} />
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}
