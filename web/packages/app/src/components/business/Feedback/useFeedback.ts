import { useEffect, useRef, useState } from "react";
import { apiErrorCode } from "@/api/client";
import { readFeedbackStatus, submitFeedback, type FeedbackResult } from "@/api/feedback";
import type { TranslateFn } from "@/models/conversations";
import {
  feedbackAttachmentKind,
  MAX_FEEDBACK_IMAGES,
  MAX_FEEDBACK_ARCHIVES,
  MAX_FEEDBACK_IMAGE_BYTES,
  MAX_FEEDBACK_ARCHIVE_BYTES,
} from "@/models/feedback";
import { localizeAPIError } from "@/shared/i18n";
import { readFeedbackDraft, saveFeedbackDraft, type FeedbackDraft } from "@/shared/storage/feedbackDraft";

const emptyDraft: FeedbackDraft = { description: "", images: [], archives: [] };
const retryableErrors = new Set([
  "feedback_access_denied",
  "feedback_login_required",
  "feedback_invalid",
  "feedback_attachment_invalid",
  "feedback_upload_failed",
  "feedback_upload_too_large",
  "feedback_tracking_unavailable",
]);

export type FeedbackAttachmentRejection = {
  file: string;
  reason:
    | "feedbackAttachmentFormat"
    | "feedbackAttachmentEmpty"
    | "feedbackImageTooLarge"
    | "feedbackArchiveTooLarge"
    | "feedbackImageCount"
    | "feedbackArchiveCount";
};

function attachmentRejection(
  kind: ReturnType<typeof feedbackAttachmentKind>,
  file: File,
  imageCount: number,
  archiveCount: number,
): FeedbackAttachmentRejection["reason"] | undefined {
  if (!kind) return "feedbackAttachmentFormat";
  if (!file.size) return "feedbackAttachmentEmpty";
  if (kind === "image") {
    if (file.size > MAX_FEEDBACK_IMAGE_BYTES) return "feedbackImageTooLarge";
    if (imageCount >= MAX_FEEDBACK_IMAGES) return "feedbackImageCount";
  } else {
    if (file.size > MAX_FEEDBACK_ARCHIVE_BYTES) return "feedbackArchiveTooLarge";
    if (archiveCount >= MAX_FEEDBACK_ARCHIVES) return "feedbackArchiveCount";
  }
}

function newRequestID(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

export function useFeedback(t: TranslateFn, accountScope = "") {
  const [draft, setDraft] = useState(emptyDraft);
  const draftRef = useRef(draft);
  const savedDraft = useRef(emptyDraft);
  const [loadedScope, setLoadedScope] = useState<string | null>(accountScope ? null : "");
  const [draftSaving, setDraftSaving] = useState(false);
  const [storageUnavailable, setStorageUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const [rejections, setRejections] = useState<FeedbackAttachmentRejection[]>([]);
  const [receipt, setReceipt] = useState<FeedbackResult | null>(null);
  const inFlight = useRef(false);
  const generation = useRef(0);
  const saveSequence = useRef(0);
  const ready = loadedScope === accountScope;
  const hasDraft = Boolean(draft.description || draft.images.length || draft.archives.length || draft.awaitingStatus);

  useEffect(() => {
    const current = ++generation.current;
    inFlight.current = false;
    draftRef.current = emptyDraft;
    savedDraft.current = emptyDraft;
    setLoadedScope(null);
    setDraft(emptyDraft);
    setDraftSaving(false);
    setStorageUnavailable(false);
    setBusy(false);
    setChecking(false);
    setError("");
    setRejections([]);
    setReceipt(null);
    if (accountScope) {
      void readFeedbackDraft(accountScope).then(
        (saved) => {
          if (current !== generation.current) return;
          draftRef.current = saved ?? emptyDraft;
          savedDraft.current = draftRef.current;
          setDraft(draftRef.current);
          setLoadedScope(accountScope);
        },
        () => {
          if (current !== generation.current) return;
          setStorageUnavailable(true);
          setLoadedScope(accountScope);
        },
      );
    } else {
      setLoadedScope("");
    }
    return () => {
      generation.current += 1;
    };
  }, [accountScope]);

  useEffect(() => {
    if (!ready || !hasDraft || (!draftSaving && !storageUnavailable)) return;
    function protectDraft(event: BeforeUnloadEvent) {
      event.preventDefault();
      event.returnValue = "";
    }
    window.addEventListener("beforeunload", protectDraft);
    return () => window.removeEventListener("beforeunload", protectDraft);
  }, [draftSaving, hasDraft, ready, storageUnavailable]);

  async function updateDraft(next: FeedbackDraft) {
    if (!next.requestID && (next.description || next.images.length || next.archives.length)) {
      next = { ...next, requestID: newRequestID() };
    }
    draftRef.current = next;
    setDraft(next);
    if (!accountScope) return;
    const current = generation.current;
    const sequence = ++saveSequence.current;
    setDraftSaving(true);
    let failed = false;
    try {
      await saveFeedbackDraft(
        accountScope,
        next,
        savedDraft.current.images !== next.images || savedDraft.current.archives !== next.archives,
      );
      if (current === generation.current) savedDraft.current = next;
    } catch {
      failed = true;
    }
    if (current !== generation.current || sequence !== saveSequence.current) return;
    setStorageUnavailable(failed);
    setDraftSaving(false);
  }

  function acceptReceipt(result: FeedbackResult) {
    setReceipt(result);
    setRejections([]);
    void updateDraft(emptyDraft);
  }

  async function submit(): Promise<boolean> {
    const current = draftRef.current;
    if (
      !ready ||
      inFlight.current ||
      current.awaitingStatus ||
      (!current.description.trim() && !current.images.length && !current.archives.length)
    )
      return false;
    const attempt = current.requestID || newRequestID();
    const currentGeneration = generation.current;
    inFlight.current = true;
    setBusy(true);
    setReceipt(null);
    setError("");
    try {
      await updateDraft({ ...current, requestID: attempt, awaitingStatus: true });
      if (currentGeneration !== generation.current) return false;
      const result = await submitFeedback(current.description, current.images, current.archives, attempt);
      if (currentGeneration !== generation.current || !result?.success) return false;
      acceptReceipt(result);
      return true;
    } catch (cause) {
      if (currentGeneration !== generation.current) return false;
      const code = apiErrorCode(cause);
      if (retryableErrors.has(code)) {
        void updateDraft({ ...current, requestID: attempt, awaitingStatus: false });
        const fileName = (cause as { fileName?: string }).fileName;
        setError(
          code === "feedback_attachment_invalid" && fileName
            ? t("feedbackInvalidAttachmentDetail", { file: fileName })
            : localizeAPIError(cause, t, t("feedbackFailed")),
        );
      }
      return false;
    } finally {
      if (currentGeneration === generation.current) {
        inFlight.current = false;
        setBusy(false);
      }
    }
  }

  async function checkStatus() {
    const current = draftRef.current;
    if (!ready || inFlight.current || !current.requestID || !current.awaitingStatus) return;
    const currentGeneration = generation.current;
    inFlight.current = true;
    setChecking(true);
    setError("");
    try {
      const status = await readFeedbackStatus(current.requestID);
      if (currentGeneration !== generation.current) return;
      if (status?.state === "submitted" && status.receipt?.success) {
        acceptReceipt(status.receipt);
      } else if (status?.state === "not_found") {
        // The original request may still arrive. Keep its ID on the next send.
        void updateDraft({ ...current, awaitingStatus: false });
        setError(t("feedbackRetryReady"));
      } else {
        setError(t("feedbackStillUnknown"));
      }
    } catch (cause) {
      if (currentGeneration === generation.current) {
        const failure = t("feedbackStatusFailed");
        setError(apiErrorCode(cause) ? localizeAPIError(cause, t, failure) : failure);
      }
    } finally {
      if (currentGeneration === generation.current) {
        inFlight.current = false;
        setChecking(false);
      }
    }
  }

  function changeDescription(description: string) {
    if (!ready || inFlight.current || draftRef.current.awaitingStatus) return;
    void updateDraft({ ...draftRef.current, description });
  }

  function addAttachments(files: File[]) {
    if (!ready || inFlight.current || draftRef.current.awaitingStatus || !files.length) return;
    const current = draftRef.current;
    const images = [...current.images];
    const archives = [...current.archives];
    const accepted = new Set<string>();
    const rejected: FeedbackAttachmentRejection[] = [];
    for (const file of files) {
      const kind = feedbackAttachmentKind(file);
      const name = file.name;
      const reason = attachmentRejection(kind, file, images.length, archives.length);
      if (reason) {
        rejected.push({ file: name, reason });
      } else {
        (kind === "image" ? images : archives).push(file);
        accepted.add(name);
      }
    }
    setRejections((previous) => [
      ...previous.filter(({ file }) => !accepted.has(file) && !rejected.some((item) => item.file === file)),
      ...rejected,
    ]);
    void updateDraft({ ...current, images, archives });
  }

  function removeAttachment(kind: "image" | "archive", index: number) {
    if (!ready || inFlight.current || draftRef.current.awaitingStatus) return;
    const current = draftRef.current;
    const key = kind === "image" ? "images" : "archives";
    const remaining = current[key].filter((_, position) => position !== index);
    const limit = kind === "image" ? MAX_FEEDBACK_IMAGES : MAX_FEEDBACK_ARCHIVES;
    const countWarning = kind === "image" ? "feedbackImageCount" : "feedbackArchiveCount";
    if (remaining.length < limit) {
      setRejections((previous) => previous.filter(({ reason }) => reason !== countWarning));
    }
    void updateDraft({ ...current, [key]: remaining });
  }

  function discardDraft() {
    if (inFlight.current) return;
    setError("");
    setRejections([]);
    void updateDraft(emptyDraft);
  }

  return {
    ...(ready ? draft : emptyDraft),
    ready,
    storageUnavailable,
    rejections,
    clearRejections: () => setRejections([]),
    busy,
    checking,
    error,
    clearError: () => setError(""),
    receipt,
    clearReceipt: () => setReceipt(null),
    submit,
    checkStatus,
    discardDraft,
    changeDescription,
    addAttachments,
    removeAttachment,
  };
}
