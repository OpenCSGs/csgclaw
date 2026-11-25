import { useEffect, useRef, useState } from "react";
import { submitFeedback, type FeedbackResult } from "@/api/feedback";
import type { TranslateFn } from "@/models/conversations";
import {
  feedbackAttachmentKind,
  MAX_FEEDBACK_IMAGES,
  MAX_FEEDBACK_ARCHIVES,
  MAX_FEEDBACK_IMAGE_BYTES,
  MAX_FEEDBACK_ARCHIVE_BYTES,
} from "@/models/feedback";
import { localizeAPIError } from "@/shared/i18n";

export function useFeedback(t: TranslateFn, accountScope = "") {
  const [description, setDescription] = useState("");
  const [images, setImages] = useState<File[]>([]);
  const [archives, setArchives] = useState<File[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [receipt, setReceipt] = useState<FeedbackResult | null>(null);
  const inFlight = useRef(false);
  const generation = useRef(0);

  useEffect(() => {
    generation.current += 1;
    inFlight.current = false;
    setDescription("");
    setImages([]);
    setArchives([]);
    setBusy(false);
    setError("");
    setReceipt(null);
    return () => {
      generation.current += 1;
    };
  }, [accountScope]);

  async function submit(): Promise<boolean> {
    if (inFlight.current) return false;
    const currentGeneration = generation.current;
    inFlight.current = true;
    setBusy(true);
    setReceipt(null);
    setError("");
    try {
      const result = await submitFeedback(description, images, archives);
      if (currentGeneration !== generation.current) return false;
      if (!result.success) {
        setError(t("feedbackFailed"));
        return false;
      }
      setReceipt(result);
      setDescription("");
      setImages([]);
      setArchives([]);
      return true;
    } catch (cause) {
      if (currentGeneration !== generation.current) return false;
      setError(localizeAPIError(cause, t, t("feedbackFailed")));
      return false;
    } finally {
      if (currentGeneration === generation.current) {
        inFlight.current = false;
        setBusy(false);
      }
    }
  }

  function changeDescription(value: string) {
    if (inFlight.current) return;
    setReceipt(null);
    setDescription(value);
  }

  function addAttachments(files: File[]) {
    if (inFlight.current || !files.length) return;
    const nextImages = [...images];
    const nextArchives = [...archives];
    const rejected: string[] = [];
    for (const file of files) {
      const kind = feedbackAttachmentKind(file);
      const target = kind === "image" ? nextImages : nextArchives;
      const maxCount = kind === "image" ? MAX_FEEDBACK_IMAGES : MAX_FEEDBACK_ARCHIVES;
      const maxBytes = kind === "image" ? MAX_FEEDBACK_IMAGE_BYTES : MAX_FEEDBACK_ARCHIVE_BYTES;
      if (!kind || file.size === 0 || file.size > maxBytes || target.length >= maxCount) {
        rejected.push(file.name || t("feedbackUnnamedAttachment"));
        continue;
      }
      target.push(file);
    }
    setImages(nextImages);
    setArchives(nextArchives);
    setError(rejected.length ? t("feedbackAttachmentsRejected", { files: rejected.join(", ") }) : "");
  }

  function removeAttachment(kind: "image" | "archive", index: number) {
    if (inFlight.current) return;
    const remove = (files: File[]) => files.filter((_, position) => position !== index);
    if (kind === "image") setImages(remove);
    else setArchives(remove);
    setError("");
  }

  return {
    description,
    images,
    busy,
    error,
    archives,
    receipt,
    clearReceipt: () => setReceipt(null),
    submit,
    changeDescription,
    addAttachments,
    removeAttachment,
  };
}
