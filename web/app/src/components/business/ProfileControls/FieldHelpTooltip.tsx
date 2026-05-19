import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { MouseEvent as ReactMouseEvent } from "react";

export type FieldHelpTooltipProps = {
  detail?: string;
  summary?: string;
};

export function FieldHelpTooltip({ detail, summary }: FieldHelpTooltipProps) {
  const body = [summary, detail]
    .map((value) => String(value ?? "").trim())
    .filter(Boolean)
    .join("\n\n");
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ x: 0, y: 0 });
  const closeTimerRef = useRef<number | null>(null);

  function clearCloseTimer() {
    if (closeTimerRef.current != null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  }

  function clamp(x: number, y: number) {
    const margin = 10;
    const width = 360;
    const height = 240;
    return {
      x: Math.max(margin, Math.min(x, window.innerWidth - width - margin)),
      y: Math.max(margin, Math.min(y, window.innerHeight - height - margin)),
    };
  }

  function scheduleClose() {
    clearCloseTimer();
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null;
      setOpen(false);
    }, 320);
  }

  function handleEnter(event: ReactMouseEvent<HTMLButtonElement>) {
    clearCloseTimer();
    setPosition(clamp(event.clientX + 14, event.clientY + 14));
    setOpen(true);
  }

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    function handleMove(event: MouseEvent) {
      setPosition(clamp(event.clientX + 14, event.clientY + 14));
    }
    window.addEventListener("mousemove", handleMove);
    return () => window.removeEventListener("mousemove", handleMove);
  }, [open]);

  useEffect(() => () => clearCloseTimer(), []);

  if (!body) {
    return null;
  }

  return (
    <span className="field-help-tooltip-root">
      <button
        type="button"
        className="field-help-trigger"
        aria-label={body}
        onMouseEnter={handleEnter}
        onMouseLeave={scheduleClose}
      >
        ?
      </button>
      {open
        ? createPortal(
            <div
              className="field-help-flyout"
              style={{ left: `${position.x}px`, top: `${position.y}px` }}
              role="tooltip"
            >
              {body}
            </div>,
            document.body,
          )
        : null}
    </span>
  );
}
