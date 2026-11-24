import { useEffect, useEffectEvent, useRef, type RefObject } from "react";
import { Button } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ConversationActivityPanel.module.css";

export function ActivityPagination({
  count,
  index,
  onChange,
  t,
}: {
  count: number;
  index: number;
  onChange: (next: number) => void;
  t: TranslateFn;
}) {
  if (count < 2) return null;
  return (
    <div className={styles.pageActions}>
      <Button size="sm" variant="tertiaryGray" disabled={index === 0} onClick={() => onChange(index - 1)}>
        {t("activityPreviousPage")}
      </Button>
      <span aria-live="polite">{t("activityPageCount", { page: index + 1, count })}</span>
      <Button size="sm" variant="tertiaryGray" disabled={index + 1 === count} onClick={() => onChange(index + 1)}>
        {t("activityNextPage")}
      </Button>
    </div>
  );
}

export type EarlierActivity = {
  cursor: string;
  load: () => void;
  loading: boolean;
};

export function ActivityHistoryContinuation({
  earlier,
  active,
  busy,
  failed,
  edge,
  scrollRef,
  onBeforeLoad,
  t,
}: {
  earlier: EarlierActivity;
  active: boolean;
  busy: boolean;
  failed: boolean;
  edge: "start" | "end";
  scrollRef: RefObject<HTMLDivElement | null>;
  onBeforeLoad?: () => void;
  t: TranslateFn;
}) {
  const load = () => {
    onBeforeLoad?.();
    earlier.load();
  };
  const reach = useEffectEvent(load);
  const lastWheel = useRef(-Infinity);
  const { cursor } = earlier;

  useEffect(() => {
    const root = scrollRef.current;
    if (!active || busy || failed || !root) return;
    const towardEdge = (delta: number) => (edge === "start" ? delta < 0 : delta > 0);
    const atEdge = () =>
      edge === "start" ? root.scrollTop <= 1 : root.scrollHeight - root.scrollTop - root.clientHeight <= 1;
    let requested = false;
    const trigger = () => {
      if (requested) return;
      requested = true;
      reach();
    };
    let lastScrollTop = root.scrollTop;
    const scroll = () => {
      if (towardEdge(root.scrollTop - lastScrollTop) && atEdge()) trigger();
      lastScrollTop = root.scrollTop;
    };
    const wheel = (event: WheelEvent) => {
      if (!towardEdge(event.deltaY)) return;
      const now = performance.now();
      const newGesture = now - lastWheel.current > 200;
      lastWheel.current = now;
      if (newGesture && atEdge()) trigger();
    };
    let touchStart: number | undefined;
    const beginTouch = (event: TouchEvent) => {
      touchStart = event.touches[0]?.clientY;
    };
    const moveTouch = (event: TouchEvent) => {
      if (touchStart === undefined || !event.touches[0]) return;
      const delta = touchStart - event.touches[0].clientY;
      if (Math.abs(delta) > 8 && towardEdge(delta) && atEdge()) trigger();
    };
    root.addEventListener("scroll", scroll, { passive: true });
    root.addEventListener("wheel", wheel, { passive: true });
    root.addEventListener("touchstart", beginTouch, { passive: true });
    root.addEventListener("touchmove", moveTouch, { passive: true });
    return () => {
      root.removeEventListener("scroll", scroll);
      root.removeEventListener("wheel", wheel);
      root.removeEventListener("touchstart", beginTouch);
      root.removeEventListener("touchmove", moveTouch);
    };
  }, [active, busy, cursor, edge, failed, scrollRef]);

  return (
    <div className={styles.pageActions}>
      {earlier.loading ? (
        <span role="status">{t("agentActivityLoading")}</span>
      ) : (
        <Button size="sm" variant="linkGray" disabled={busy} onClick={load}>
          {t("activityLoadEarlier")}
        </Button>
      )}
    </div>
  );
}
