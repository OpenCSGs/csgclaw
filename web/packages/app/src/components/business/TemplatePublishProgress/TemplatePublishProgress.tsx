import { useEffect, useRef, useState } from "react";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TemplatePublishProgress.module.css";

export type TemplatePublishProgressProps = {
  active: boolean;
  complete?: boolean;
  failed?: boolean;
  labels: {
    preparing: string;
    uploading: string;
    complete: string;
    failed: string;
  };
};

export function TemplatePublishProgress({
  active,
  complete = false,
  failed = false,
  labels,
}: TemplatePublishProgressProps) {
  const [percent, setPercent] = useState(0);
  const startedAtRef = useRef(0);
  const frameRef = useRef<number | null>(null);

  useEffect(() => {
    if (!active) return;
    startedAtRef.current = performance.now();
    setPercent(0);
  }, [active]);

  useEffect(() => {
    if (!active) return;
    const tick = () => {
      const elapsed = performance.now() - startedAtRef.current;
      const eased = 96 * (1 - Math.exp(-elapsed / 520));
      const next = Math.max(0, Math.min(99, elapsed < 1400 ? eased : Math.min(99, 96 + (elapsed - 1400) / 360)));
      setPercent((current) => (next > current ? next : current));
      frameRef.current = requestAnimationFrame(tick);
    };
    frameRef.current = requestAnimationFrame(tick);
    return () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    };
  }, [active]);

  if (!active && !complete && !failed) return null;
  const displayedPercent = complete ? 100 : Math.floor(percent);
  const label = failed
    ? labels.failed
    : complete
      ? labels.complete
      : percent < 12
        ? labels.preparing
        : labels.uploading;

  return (
    <div
      className={classNames(styles.progress, failed && styles.failed)}
      data-template-publish-progress=""
      role="status"
      aria-live="polite"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={displayedPercent}
      aria-label={label}
    >
      <div className={styles.header}>
        <span>{label}</span>
        <strong>{displayedPercent}%</strong>
      </div>
      <div className={styles.track} aria-hidden="true">
        <div className={styles.fill} style={{ width: `${displayedPercent}%` }} />
      </div>
    </div>
  );
}
