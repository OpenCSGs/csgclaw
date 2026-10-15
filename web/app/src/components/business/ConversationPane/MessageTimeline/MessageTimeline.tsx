import { useEffect, useId, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { PopoverAnchor, PopoverContent, PopoverRoot } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import {
  messageTimelineMarks,
  nearestMessageIndex,
  timelineMarkPresentation,
  type MessageTimelineItem,
} from "./timelineUtils";
import { useMessageTimeline } from "./useMessageTimeline";
import styles from "./MessageTimeline.module.css";

type MessageTimelineProps = {
  items: readonly MessageTimelineItem[];
  scrollRef: RefObject<HTMLElement | null>;
  t: TranslateFn;
};

export function MessageTimeline({ items, scrollRef, t }: MessageTimelineProps) {
  const { readingIndex, navigate, jumpToLatest } = useMessageTimeline(items, scrollRef);
  const [open, setOpen] = useState(false);
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const [pointerRatio, setPointerRatio] = useState<number | null>(null);
  const [keyboardNavigation, setKeyboardNavigation] = useState(false);
  const railRef = useRef<HTMLDivElement>(null);
  const previewID = useId();
  const previewRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);
  const keyboardFocus = useRef(false);
  const pointerFocus = useRef(false);
  const pointerFrame = useRef<number | null>(null);
  const pendingPointer = useRef(0);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const selectedIndex = Math.max(0, Math.min(items.length - 1, previewIndex ?? readingIndex));
  const selected = items[selectedIndex];
  const marks = messageTimelineMarks(items.length);
  const activeMark = marks[nearestMessageIndex(marks, selectedIndex)];
  const expandedRatio = open ? (pointerRatio ?? selectedIndex / Math.max(1, items.length - 1)) : null;

  function cancelClose() {
    if (closeTimer.current) clearTimeout(closeTimer.current);
  }
  function showPreview() {
    cancelClose();
    setOpen(true);
  }
  function scheduleClose() {
    cancelClose();
    closeTimer.current = setTimeout(() => {
      if (
        draggingRef.current ||
        previewRef.current?.contains(document.activeElement) ||
        (keyboardFocus.current && railRef.current === document.activeElement)
      )
        return;
      setOpen(false);
      setPreviewIndex(null);
      setPointerRatio(null);
    }, 140);
  }
  function stopDragging() {
    draggingRef.current = false;
    setDragging(false);
  }
  useEffect(
    () => () => {
      if (closeTimer.current) clearTimeout(closeTimer.current);
      if (pointerFrame.current !== null) cancelAnimationFrame(pointerFrame.current);
    },
    [],
  );

  useEffect(() => {
    const rail = railRef.current;
    if (!rail) return;
    let delta = 0;
    const onWheel = (event: WheelEvent) => {
      if (event.ctrlKey || !event.deltaY) return;
      event.preventDefault();
      delta += event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rail.clientHeight : 1);
      if (Math.abs(delta) < 32) return;
      const index = Math.max(0, Math.min(items.length - 1, selectedIndex + Math.sign(delta)));
      delta = 0;
      setPreviewIndex(index);
      setPointerRatio(index / Math.max(1, items.length - 1));
      setOpen(true);
      navigate(index);
    };
    rail.addEventListener("wheel", onWheel, { passive: false });
    return () => rail.removeEventListener("wheel", onWheel);
  }, [items.length, navigate, selectedIndex]);

  function ratioAt(clientY: number) {
    const rect = railRef.current?.getBoundingClientRect();
    if (!rect) return 0;
    const ratio = Math.max(0, Math.min(1, (clientY - rect.top) / Math.max(1, rect.height)));
    return ratio;
  }
  function updatePointer(clientY: number, immediate = false) {
    const ratio = ratioAt(clientY);
    pendingPointer.current = ratio;
    const apply = () => {
      pointerFrame.current = null;
      setPointerRatio(pendingPointer.current);
      setPreviewIndex(Math.round(pendingPointer.current * (items.length - 1)));
    };
    if (immediate) {
      if (pointerFrame.current !== null) cancelAnimationFrame(pointerFrame.current);
      apply();
    } else if (pointerFrame.current === null) {
      pointerFrame.current = requestAnimationFrame(apply);
    }
    return Math.round(ratio * (items.length - 1));
  }
  function focusRail(fromPointer: boolean) {
    pointerFocus.current = fromPointer;
    if (fromPointer) {
      keyboardFocus.current = false;
      setKeyboardNavigation(false);
    }
    railRef.current?.focus({ preventScroll: true });
    pointerFocus.current = false;
  }
  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (pointerFrame.current !== null) {
      cancelAnimationFrame(pointerFrame.current);
      pointerFrame.current = null;
    }
    keyboardFocus.current = true;
    setKeyboardNavigation(true);
    setPointerRatio(null);
    let index = selectedIndex;
    switch (event.key) {
      case "Enter":
      case " ":
        navigate(index);
        break;
      case "ArrowUp":
        index -= 1;
        break;
      case "ArrowDown":
        index += 1;
        break;
      case "PageUp":
        index -= 10;
        break;
      case "PageDown":
        index += 10;
        break;
      case "Home":
        index = 0;
        break;
      case "End":
        event.preventDefault();
        setPreviewIndex(null);
        showPreview();
        jumpToLatest();
        return;
      case "Escape":
        setOpen(false);
        setPreviewIndex(null);
        return;
      default:
        return;
    }
    event.preventDefault();
    index = Math.max(0, Math.min(items.length - 1, index));
    setPreviewIndex(index);
    showPreview();
    navigate(index);
  }

  if (items.length < 2) return null;

  return (
    <nav
      className={styles.timeline}
      aria-label={t("messageTimelineLabel")}
      style={{ height: Math.max(30, (marks.length - 1) * 12) }}
    >
      <PopoverRoot
        open={open}
        onOpenChange={(next) => {
          cancelClose();
          setOpen(next);
          if (!next) {
            setPreviewIndex(null);
            setPointerRatio(null);
          }
        }}
      >
        <div
          ref={railRef}
          className={styles.rail}
          role="slider"
          tabIndex={0}
          aria-label={t("messageTimelineScrub")}
          aria-orientation="vertical"
          aria-expanded={open}
          aria-haspopup="dialog"
          aria-controls={open ? previewID : undefined}
          aria-valuemin={1}
          aria-valuemax={items.length}
          aria-valuenow={Math.min(items.length, readingIndex + 1)}
          aria-valuetext={items[readingIndex]?.label}
          data-dragging={dragging || undefined}
          data-keyboard-focus={keyboardNavigation || undefined}
          onClick={(event) => event.preventDefault()}
          onFocus={() => {
            keyboardFocus.current = !pointerFocus.current;
            setKeyboardNavigation(keyboardFocus.current);
            showPreview();
          }}
          onBlur={() => {
            keyboardFocus.current = false;
            setKeyboardNavigation(false);
            scheduleClose();
          }}
          onKeyDown={handleKeyDown}
          onPointerEnter={(event) => {
            keyboardFocus.current = false;
            setKeyboardNavigation(false);
            updatePointer(event.clientY, true);
            showPreview();
          }}
          onPointerLeave={scheduleClose}
          onPointerDown={(event) => {
            if (event.button !== 0) return;
            event.preventDefault();
            draggingRef.current = true;
            focusRail(true);
            showPreview();
            const index = updatePointer(event.clientY, true);
            navigate(index);
            setDragging(true);
            try {
              event.currentTarget.setPointerCapture(event.pointerId);
            } catch {
              /* Clicking also works without pointer capture. */
            }
          }}
          onPointerMove={(event) => {
            const index = updatePointer(event.clientY);
            if (draggingRef.current) navigate(index);
          }}
          onPointerUp={(event) => {
            stopDragging();
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              event.currentTarget.releasePointerCapture(event.pointerId);
            const rect = event.currentTarget.getBoundingClientRect();
            if (
              event.clientX < rect.left ||
              event.clientX > rect.right ||
              event.clientY < rect.top ||
              event.clientY > rect.bottom
            )
              scheduleClose();
          }}
          onPointerCancel={() => {
            stopDragging();
            scheduleClose();
          }}
          onLostPointerCapture={stopDragging}
        >
          {marks.map((index) => {
            const position = index / (items.length - 1);
            const motion = timelineMarkPresentation(position, expandedRatio, marks.length);
            return (
              <span
                key={index}
                className={styles.mark}
                data-active={(open && index === activeMark) || undefined}
                aria-hidden="true"
                style={{
                  top: `${position * 100}%`,
                  width: motion.width,
                }}
              />
            );
          })}
        </div>
        <PopoverAnchor asChild>
          <span
            className={styles.anchor}
            aria-hidden="true"
            style={{ top: `${(expandedRatio ?? selectedIndex / (items.length - 1)) * 100}%` }}
          />
        </PopoverAnchor>
        <PopoverContent
          ref={previewRef}
          id={previewID}
          className={styles.preview}
          side="left"
          align="center"
          sideOffset={12}
          aria-label={t("messageTimelinePreview")}
          onPointerEnter={cancelClose}
          onPointerLeave={scheduleClose}
          onFocusCapture={cancelClose}
          onBlurCapture={scheduleClose}
          onInteractOutside={(event) => {
            const target = event.detail.originalEvent.target;
            if (target instanceof Node && railRef.current?.contains(target)) event.preventDefault();
          }}
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => event.preventDefault()}
        >
          <button
            type="button"
            className={styles.previewLink}
            aria-label={t("messageTimelineJump", { message: selected.label })}
            onClick={(event) => {
              navigate(selectedIndex);
              focusRail(event.detail > 0);
              setOpen(false);
              setPreviewIndex(null);
            }}
          >
            <strong className={styles.title}>{selected.label}</strong>
            {selected.preview ? <span className={styles.summary}>{selected.preview}</span> : null}
            <span className={styles.position}>
              {selectedIndex + 1} / {items.length}
            </span>
          </button>
        </PopoverContent>
      </PopoverRoot>
    </nav>
  );
}
