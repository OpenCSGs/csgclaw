import { useEffect, useRef } from "react";
import { Button } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { ResourceContinuation } from "@/hooks/workspace/useInfiniteAgentResources";
import styles from "./ResourceLoadMore.module.css";

export function ResourceLoadMore({ continuation, t }: { continuation?: ResourceContinuation; t: TranslateFn }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = ref.current;
    if (
      !element ||
      !continuation?.hasMore ||
      continuation.loading ||
      continuation.failed ||
      typeof IntersectionObserver === "undefined"
    )
      return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void continuation.loadMore();
      },
      { root: element.closest(".agent-profile-scroll-region"), rootMargin: "200px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [continuation]);
  if (!continuation) return null;
  return (
    <div ref={ref} className={styles.root} aria-live="polite">
      {continuation.loading ? (
        <span role="status">{t("resourceLoading")}</span>
      ) : continuation.failed ? (
        <Button size="sm" onClick={continuation.retry}>
          {t("retry")}
        </Button>
      ) : continuation.hasMore ? (
        <Button variant="secondaryGray" size="sm" onClick={() => void continuation.loadMore()}>
          {t("resourceLoadMore")}
        </Button>
      ) : continuation.total > 0 ? (
        <div className={styles.allLoaded} role="status">
          <span className={styles.divider} aria-hidden="true"></span>
          <span className={styles.allLoadedText}>{t("resourceAllLoaded", { total: continuation.total })}</span>
          <span className={styles.divider} aria-hidden="true"></span>
        </div>
      ) : null}
    </div>
  );
}
