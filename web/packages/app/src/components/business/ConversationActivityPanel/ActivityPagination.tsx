import { useEffect, useEffectEvent, useRef, type ReactNode, type RefObject } from "react";
import { Button } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ConversationActivityPanel.module.css";

export function ActivityPagination({
  count,
  index,
  onChange,
  scrollRef,
  active,
  t,
}: {
  count: number;
  index: number;
  onChange: (next: number) => void;
  scrollRef: RefObject<HTMLDivElement | null>;
  active: boolean;
  t: TranslateFn;
}) {
  if (count < 2) return null;
  return (
    <>
      <div className={styles.pageActions}>
        <Button size="sm" variant="tertiaryGray" disabled={index === 0} onClick={() => onChange(index - 1)}>
          {t("activityPreviousPage")}
        </Button>
        <span aria-live="polite">{t("activityPageCount", { page: index + 1, count })}</span>
        <Button size="sm" variant="tertiaryGray" disabled={index + 1 === count} onClick={() => onChange(index + 1)}>
          {t("activityNextPage")}
        </Button>
      </div>
      {index + 1 < count ? (
        <ActivityBoundary key={index} active={active} scrollRef={scrollRef} onReach={() => onChange(index + 1)} />
      ) : null}
    </>
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
  t,
}: {
  earlier: EarlierActivity;
  active: boolean;
  busy: boolean;
  failed: boolean;
  edge: "start" | "end";
  scrollRef: RefObject<HTMLDivElement | null>;
  t: TranslateFn;
}) {
  return (
    <ActivityBoundary
      key={earlier.cursor}
      active={active && !busy && !failed}
      edge={edge}
      scrollRef={scrollRef}
      onReach={earlier.load}
    >
      {earlier.loading ? (
        <span className={styles.pageActions} role="status">
          {t("agentActivityLoading")}
        </span>
      ) : failed ? (
        <div className={styles.pageActions}>
          <Button size="sm" variant="linkGray" disabled={busy} onClick={earlier.load}>
            {t("activityLoadEarlier")}
          </Button>
        </div>
      ) : null}
    </ActivityBoundary>
  );
}

function ActivityBoundary({
  active,
  edge = "end",
  scrollRef,
  onReach,
  children,
}: {
  active: boolean;
  edge?: "start" | "end";
  scrollRef: RefObject<HTMLDivElement | null>;
  onReach: () => void;
  children?: ReactNode;
}) {
  const markerRef = useRef<HTMLDivElement>(null);
  const reach = useEffectEvent(onReach);

  useEffect(() => {
    const marker = markerRef.current;
    const root = scrollRef.current;
    if (!active || !marker || !root) return;
    let reached = false;
    const trigger = () => {
      if (reached) return;
      reached = true;
      reach();
    };

    // In chronological order, older records are above the viewport. Wait for
    // an upward scroll in an already scrollable list so opening it at the top
    // does not fetch all the older pages in succession.
    let lastScrollTop = root.scrollTop;
    const handleScroll = () => {
      const top = root.scrollTop;
      if (edge === "start" ? top < lastScrollTop && top <= 1 : root.scrollHeight - top - root.clientHeight <= 1)
        trigger();
      lastScrollTop = top;
    };
    root.addEventListener("scroll", handleScroll, { passive: true });
    const observer =
      typeof IntersectionObserver === "undefined"
        ? null
        : new IntersectionObserver(
            (entries) => {
              if (
                entries.some((entry) => entry.isIntersecting) &&
                (edge === "end" || root.scrollHeight <= root.clientHeight)
              )
                trigger();
            },
            { root },
          );
    observer?.observe(marker);
    return () => {
      reached = true;
      observer?.disconnect();
      root.removeEventListener("scroll", handleScroll);
    };
  }, [active, edge, scrollRef]);

  return (
    <div className={styles.activityBoundary} ref={markerRef}>
      {children}
    </div>
  );
}
