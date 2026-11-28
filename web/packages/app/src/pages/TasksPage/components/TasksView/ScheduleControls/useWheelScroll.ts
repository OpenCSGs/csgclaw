import { useEffect, type RefObject, type WheelEvent as ReactWheelEvent } from "react";

// Convert line-based wheel events to the approximate height of an option.
const LINE_HEIGHT_PX = 30;

export function useWheelScroll(ref: RefObject<HTMLElement | null>, active: unknown = true) {
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const onWheel = (event: WheelEvent) => {
      if (event.ctrlKey || event.metaKey) return;
      const max = element.scrollHeight - element.clientHeight;
      if (max <= 0) return;
      const delta = wheelPixels(event, element);
      if (!delta) return;
      const before = element.scrollTop;
      const next = Math.min(max, Math.max(0, before + delta));
      // Leave boundary events to the surrounding popover or dialog.
      if ((delta < 0 && before <= 0) || (delta > 0 && before >= max - 0.5)) return;
      event.preventDefault();
      event.stopPropagation();
      element.scrollTop = next;
    };
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => element.removeEventListener("wheel", onWheel);
  }, [ref, active]);
}

// Popovers are portaled outside the modal, so native scroll chaining cannot reach the form.
export function handoffWheelToDialog(event: ReactWheelEvent<HTMLElement>, trigger: HTMLElement | null) {
  const wheel = event.nativeEvent;
  if (wheel.ctrlKey || wheel.metaKey) return;
  const popover = event.currentTarget;
  const delta = wheelPixels(wheel, popover);
  const limit = popover.scrollHeight - popover.clientHeight;
  if (!delta || (delta < 0 && popover.scrollTop > 0) || (delta > 0 && popover.scrollTop < limit - 0.5)) return;

  const dialog = trigger?.closest<HTMLElement>(".csg-dialog-content");
  const scroller = dialog?.hasAttribute("data-scroll-body")
    ? dialog.querySelector<HTMLElement>(":scope > .csg-dialog-body")
    : dialog;
  if (scroller) scroller.scrollTop += wheelPixels(wheel, scroller);
}

function wheelPixels(event: WheelEvent, element: HTMLElement): number {
  if (event.deltaMode === 1) return event.deltaY * LINE_HEIGHT_PX;
  if (event.deltaMode === 2) return event.deltaY * element.clientHeight;
  return event.deltaY;
}
