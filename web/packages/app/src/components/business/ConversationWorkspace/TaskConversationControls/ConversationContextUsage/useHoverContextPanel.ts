import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type MouseEvent } from "react";

// Keep the interactive panel open while crossing its portal gap, without
// moving focus away from the composer just to inspect context usage.
export function useHoverContextPanel() {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pointerOpened = useRef(false);
  const restoreFocus = useRef(false);
  function cancelClose() {
    if (closeTimer.current !== null) clearTimeout(closeTimer.current);
    closeTimer.current = null;
  }
  function onOpenChange(next: boolean) {
    cancelClose();
    if (!next) restoreFocus.current ||= panelRef.current?.contains(document.activeElement) ?? false;
    setOpen(next);
  }
  function showOnHover(event: PointerEvent) {
    if (event.pointerType === "touch") return;
    cancelClose();
    if (!open) {
      pointerOpened.current = true;
      restoreFocus.current = false;
    }
    setOpen(true);
  }
  function scheduleClose() {
    cancelClose();
    if (!pointerOpened.current) return;
    closeTimer.current = setTimeout(() => {
      if (panelRef.current?.contains(document.activeElement)) return;
      restoreFocus.current = false;
      setOpen(false);
    }, 180);
  }
  function onTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (!["Enter", " ", "ArrowDown"].includes(event.key)) return;
    pointerOpened.current = false;
    cancelClose();
    if (open) {
      event.preventDefault();
      panelRef.current?.querySelector<HTMLElement>("button:not(:disabled), summary")?.focus();
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      setOpen(true);
    }
  }
  useEffect(
    () => () => {
      if (closeTimer.current !== null) clearTimeout(closeTimer.current);
    },
    [],
  );
  return {
    open,
    onOpenChange,
    triggerProps: {
      ref: triggerRef,
      onPointerEnter: showOnHover,
      onPointerDown: () => {
        restoreFocus.current = true;
      },
      onPointerLeave: scheduleClose,
      onKeyDown: onTriggerKeyDown,
      onClick: (event: MouseEvent<HTMLButtonElement>) => {
        if (open) event.preventDefault();
      },
    },
    panelProps: {
      ref: panelRef,
      onPointerDownCapture: () => {
        restoreFocus.current = true;
      },
      onPointerEnter: cancelClose,
      onPointerLeave: scheduleClose,
      onFocusCapture: cancelClose,
      onOpenAutoFocus: (event: Event) => {
        if (pointerOpened.current) event.preventDefault();
      },
      onCloseAutoFocus: (event: Event) => {
        if (pointerOpened.current) {
          event.preventDefault();
          if (restoreFocus.current) triggerRef.current?.focus();
        }
      },
    },
  };
}
