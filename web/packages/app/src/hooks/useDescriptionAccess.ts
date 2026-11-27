import { useEffect, useState } from "react";
import { fetchDescriptionAccess } from "@/api/descriptionAccess";

const DESCRIPTION_ACCESS_TIMEOUT_MS = 5000;

type DescriptionAccessStatus = "checking" | "allowed" | "denied" | "error";

export function useDescriptionAccess(enabled: boolean) {
  const [status, setStatus] = useState<DescriptionAccessStatus>("checking");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    const timeout = window.setTimeout(() => {
      controller.abort();
      setStatus("error");
    }, DESCRIPTION_ACCESS_TIMEOUT_MS);
    void fetchDescriptionAccess(controller.signal)
      .then(
        (response) => {
          if (controller.signal.aborted) return;
          setStatus(response?.allowed === true ? "allowed" : response?.allowed === false ? "denied" : "error");
        },
        () => {
          if (!controller.signal.aborted) setStatus("error");
        },
      )
      .finally(() => window.clearTimeout(timeout));
    return () => {
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [attempt, enabled]);

  function retry() {
    setStatus("checking");
    setAttempt((current) => current + 1);
  }

  return { status: enabled ? status : "checking", retry };
}
