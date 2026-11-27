import { useEffect, useEffectEvent, useState, useSyncExternalStore } from "react";

const reducedMotionQuery = "(prefers-reduced-motion: reduce)";
const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" });
const revealInterval = 24;
const maxAnimatedTail = 240;

function prefersReducedMotion() {
  return window.matchMedia?.(reducedMotionQuery).matches ?? false;
}

function subscribeToReducedMotion(listener: () => void) {
  const media = window.matchMedia?.(reducedMotionQuery);
  media?.addEventListener("change", listener);
  return () => media?.removeEventListener("change", listener);
}

const subscribeIdle = () => () => {};
const noReducedMotion = () => false;

export function useStreamingText(text: string, streaming: boolean) {
  const reducedMotion = useSyncExternalStore(
    streaming ? subscribeToReducedMotion : subscribeIdle,
    streaming ? prefersReducedMotion : noReducedMotion,
    noReducedMotion,
  );
  // Remounts and corrected snapshots should show the text already received.
  const [visible, setVisible] = useState(text);
  const remaining = text.length - visible.length;
  const animate = streaming && !reducedMotion && remaining <= maxAnimatedTail && text.startsWith(visible);
  if (!animate && visible !== text) setVisible(text);

  const reveal = useEffectEvent(() => {
    // Catch up to arriving batches without splitting emoji or accented letters.
    const step = Math.max(1, Math.ceil(remaining / 3));
    for (const { index, segment } of graphemes.segment(text.slice(visible.length))) {
      const end = index + segment.length;
      if (end >= step) {
        setVisible(text.slice(0, visible.length + end));
        return;
      }
    }
  });
  const pending = animate && remaining > 0;
  useEffect(() => {
    if (!pending) return;
    const timer = window.setTimeout(() => reveal(), revealInterval);
    return () => window.clearTimeout(timer);
  }, [pending, visible]);

  return animate ? visible : text;
}
