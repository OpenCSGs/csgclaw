import styles from "./DismissibleAlert.module.css";
import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { X } from "lucide-react";

type DismissibleAlertProps = {
  children: ReactNode;
  className?: string;
  /** When this value changes, the dismissed state resets. */
  messageKey?: string;
  closeLabel?: string;
  closeIcon?: ReactNode;
  onDismiss?: () => void;
};

export function DismissibleAlert({
  children,
  className,
  messageKey,
  closeLabel = "Close",
  closeIcon,
  onDismiss,
}: DismissibleAlertProps) {
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    setDismissed(false);
  }, [messageKey]);

  if (dismissed) return null;

  return (
    <div className={className} aria-live="polite">
      {children}
      <button
        type="button"
        className={styles["dismissible-alert-close-btn"]}
        onClick={() => {
          setDismissed(true);
          onDismiss?.();
        }}
        aria-label={closeLabel}
      >
        {closeIcon ?? <X size={14} aria-hidden="true" />}
      </button>
    </div>
  );
}
