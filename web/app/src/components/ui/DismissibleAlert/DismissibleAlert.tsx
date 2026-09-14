import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { X } from "lucide-react";

type DismissibleAlertProps = {
  children: ReactNode;
  className?: string;
  /** When this value changes, the dismissed state resets. */
  messageKey?: string;
  closeLabel?: string;
};

export function DismissibleAlert({
  children,
  className,
  messageKey,
  closeLabel = "Close",
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
        className="dismissible-alert-close-btn"
        onClick={() => setDismissed(true)}
        aria-label={closeLabel}
      >
        <X size={14} aria-hidden="true" />
      </button>
    </div>
  );
}
