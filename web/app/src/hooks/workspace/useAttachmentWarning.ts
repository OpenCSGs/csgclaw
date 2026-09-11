import { useCallback, useEffect, useRef, useState } from "react";

export function useAttachmentWarning(scopeKey: string) {
  const [warning, setWarning] = useState("");
  const timerRef = useRef<number | null>(null);

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  useEffect(() => {
    clearTimer();
    setWarning("");
    return clearTimer;
  }, [clearTimer, scopeKey]);

  const showWarning = useCallback(
    (message: string) => {
      clearTimer();
      setWarning(message);
      if (message) {
        timerRef.current = window.setTimeout(() => {
          timerRef.current = null;
          setWarning("");
        }, 5000);
      }
    },
    [clearTimer],
  );

  return [warning, showWarning] as const;
}
