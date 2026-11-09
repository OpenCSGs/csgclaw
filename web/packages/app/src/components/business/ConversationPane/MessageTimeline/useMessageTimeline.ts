import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { MESSAGE_LIST_NAVIGATE_EVENT } from "@/shared/constants/workspace";
import { nearestMessageIndex, type MessageTimelineItem } from "./timelineUtils";

type MeasuredMessage = { node: HTMLElement; top: number };

export function useMessageTimeline(items: readonly MessageTimelineItem[], scrollRef: RefObject<HTMLElement | null>) {
  const [readingIndex, setReadingIndex] = useState(0);
  const measuredRef = useRef<MeasuredMessage[]>([]);
  const positionsRef = useRef<number[]>([]);
  const navigationRef = useRef<{ index: number; scrollTop: number } | null>(null);

  useEffect(() => {
    const scroll = scrollRef.current;
    if (!scroll || items.length < 2) return;
    let measureFrame = 0;
    let scrollFrame = 0;
    const applyScroll = () => {
      scrollFrame = 0;
      const positions = positionsRef.current;
      let index = nearestMessageIndex(positions, scroll.scrollTop + 36);
      if (positions[index] > scroll.scrollTop + 36 && index > 0) index -= 1;
      if (scroll.scrollTop + scroll.clientHeight >= scroll.scrollHeight - 2) index = items.length - 1;
      if (navigationRef.current && Math.abs(scroll.scrollTop - navigationRef.current.scrollTop) < 1) {
        index = Math.min(items.length - 1, navigationRef.current.index);
      } else {
        navigationRef.current = null;
      }
      setReadingIndex(Math.max(0, index));
    };
    const measure = () => {
      measureFrame = 0;
      const nodes = new Map(
        Array.from(scroll.querySelectorAll<HTMLElement>("[data-message-id]")).map((node) => [
          node.dataset.messageId,
          node,
        ]),
      );
      const origin = scroll.getBoundingClientRect().top + scroll.clientTop - scroll.scrollTop;
      measuredRef.current = items.flatMap((item) => {
        const node = nodes.get(item.id);
        return node ? [{ node, top: node.getBoundingClientRect().top - origin }] : [];
      });
      positionsRef.current = measuredRef.current.map((message) => message.top);
      applyScroll();
    };
    const scheduleMeasure = () => {
      if (!measureFrame) measureFrame = requestAnimationFrame(measure);
    };
    const onScroll = () => {
      if (!scrollFrame) scrollFrame = requestAnimationFrame(applyScroll);
    };
    const resize = new ResizeObserver(scheduleMeasure);
    const observeContent = () => {
      resize.disconnect();
      resize.observe(scroll);
      Array.from(scroll.children).forEach((child) => resize.observe(child));
      scheduleMeasure();
    };
    const mutation = new MutationObserver(observeContent);
    mutation.observe(scroll, { childList: true });
    observeContent();
    scroll.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      resize.disconnect();
      mutation.disconnect();
      scroll.removeEventListener("scroll", onScroll);
      cancelAnimationFrame(measureFrame);
      cancelAnimationFrame(scrollFrame);
    };
  }, [items, scrollRef]);

  const navigate = useCallback(
    (index: number) => {
      const scroll = scrollRef.current;
      const message = measuredRef.current[index];
      if (!scroll || !message) return;
      const top = Math.max(0, Math.min(message.top - 32, scroll.scrollHeight - scroll.clientHeight));
      if (navigationRef.current?.index === index && Math.abs(scroll.scrollTop - top) < 1) return;
      scroll.dispatchEvent(new Event(MESSAGE_LIST_NAVIGATE_EVENT));
      scroll.scrollTo({ top: Math.max(0, message.top - 32), behavior: "auto" });
      navigationRef.current = { index, scrollTop: scroll.scrollTop };
      setReadingIndex(index);
      if (!window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
        const bubble = message.node.querySelector<HTMLElement>(".message-bubble") || message.node;
        bubble.animate?.(
          [
            { outline: "2px solid transparent", outlineOffset: "4px" },
            { outline: "2px solid var(--primary)", outlineOffset: "4px", offset: 0.2 },
            { outline: "2px solid transparent", outlineOffset: "4px" },
          ],
          { duration: 1100, easing: "ease-out" },
        );
      }
    },
    [scrollRef],
  );

  const jumpToLatest = useCallback(() => {
    const scroll = scrollRef.current;
    if (!scroll) return;
    scroll.dispatchEvent(new CustomEvent(MESSAGE_LIST_NAVIGATE_EVENT, { detail: "follow" }));
    navigationRef.current = null;
    scroll.scrollTo({ top: scroll.scrollHeight, behavior: "auto" });
    setReadingIndex(items.length - 1);
  }, [items.length, scrollRef]);

  return { readingIndex, navigate, jumpToLatest };
}
