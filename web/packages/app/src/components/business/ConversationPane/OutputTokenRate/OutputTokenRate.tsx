import { useEffect, useRef, useState } from "react";
import type { TranslateFn } from "@/models/conversations";
import styles from "./OutputTokenRate.module.css";

const sampleLifetime = 2500;
const animationDuration = 420;

export function OutputTokenRate({ rate, sampledAt, t }: { rate?: number; sampledAt?: number; t: TranslateFn }) {
  const current = useRef<number | undefined>(undefined);
  const [display, setDisplay] = useState<number>();
  useEffect(() => {
    const remaining = sampledAt ? sampledAt + sampleLifetime - Date.now() : 0;
    if (!rate || remaining <= 0) {
      current.current = undefined;
      setDisplay(undefined);
      return;
    }
    const from = current.current ?? rate;
    setDisplay(from);
    let frame = 0;
    if (from !== rate && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      const start = performance.now();
      const animate = (now: number) => {
        const fraction = Math.min(1, (now - start) / animationDuration);
        const eased = 1 - (1 - fraction) ** 3;
        current.current = from + (rate - from) * eased;
        setDisplay(current.current);
        if (fraction < 1) frame = window.requestAnimationFrame(animate);
      };
      frame = window.requestAnimationFrame(animate);
    } else {
      current.current = rate;
      setDisplay(rate);
    }
    const expires = window.setTimeout(() => {
      window.cancelAnimationFrame(frame);
      current.current = undefined;
      setDisplay(undefined);
    }, remaining);
    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(expires);
    };
  }, [rate, sampledAt]);

  if (!rate || display === undefined) return null;
  const rounded = Math.max(1, Math.round(display));
  const accessibleLabel = t("tokenRateLabel", { rate: rounded });
  const tier = rounded >= 35 ? "fast" : rounded >= 20 ? "medium" : "slow";
  return (
    <span className={styles.rate} data-tier={tier} title={accessibleLabel} aria-label={accessibleLabel}>
      <span className={styles.pulse} aria-hidden="true" />
      <span aria-hidden="true">{rounded}</span>
      <span className={styles.unit} aria-hidden="true">
        t/s
      </span>
    </span>
  );
}
