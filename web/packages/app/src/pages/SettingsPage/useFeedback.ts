import { useEffect, useRef, useState } from "react";
import { submitFeedback } from "@/api/feedback";
import type { TranslateFn } from "@/models/conversations";
import { feedbackImagesValid, MAX_FEEDBACK_IMAGES } from "@/models/feedback";
import { localizeAPIError } from "@/shared/i18n";

export function useFeedback(t: TranslateFn, accountScope = "") {
  const [description, setDescription] = useState("");
  const [images, setImages] = useState<File[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const inFlight = useRef(false);
  const generation = useRef(0);

  useEffect(() => {
    generation.current += 1;
    inFlight.current = false;
    setDescription("");
    setImages([]);
    setBusy(false);
    setError("");
    setSuccess(false);
    return () => {
      generation.current += 1;
    };
  }, [accountScope]);

  async function submit(): Promise<boolean> {
    if (inFlight.current) return false;
    const currentGeneration = generation.current;
    inFlight.current = true;
    setBusy(true);
    setSuccess(false);
    setError("");
    try {
      const result = await submitFeedback(description, images);
      if (currentGeneration !== generation.current) return false;
      if (!result.success) {
        setError(t("feedbackFailed"));
        return false;
      }
      setSuccess(true);
      setDescription("");
      setImages([]);
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
    setSuccess(false);
    setDescription(value);
  }

  function changeImages(value: File[]) {
    if (inFlight.current) return;
    const nextImages = value.slice(0, MAX_FEEDBACK_IMAGES);
    if (!feedbackImagesValid(nextImages)) {
      setError(t("feedbackImageLimit"));
      return;
    }
    setError("");
    setSuccess(false);
    setImages(nextImages);
  }

  return {
    description,
    images,
    busy,
    error,
    success,
    submit,
    changeDescription,
    changeImages,
  };
}
