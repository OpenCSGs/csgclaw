import { useCallback, useEffect, useLayoutEffect, useId, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { LongMessageCollapseIcon, LongMessageExpandIcon } from "@/components/ui/Icons";
import { classNames } from "@/shared/lib/classNames";
import type { TranslateFn } from "@/models/conversations";
import { prepareMermaidBlocks, renderMermaidBlocks } from "./mermaid";

const COLLAPSE_MAX_HEIGHT_PX = 200;
const COLLAPSE_LINE_COUNT = 8;
const COLLAPSE_HEIGHT_TOLERANCE_PX = 1;
type LongMessageCollapseProps = {
  expanded?: boolean;
  html: string;
  onExpandedChange?: (expanded: boolean) => void;
  t: TranslateFn;
};

type LongMessageMetrics = {
  collapseHeight: number;
  expandedHeight: number;
  shouldCollapse: boolean;
};

export function LongMessageCollapse({ expanded, html, onExpandedChange, t }: LongMessageCollapseProps) {
  const contentRef = useRef<HTMLDivElement | null>(null);
  const [internalExpanded, setInternalExpanded] = useState(false);
  const [metrics, setMetrics] = useState<LongMessageMetrics | null>(null);
  const [renderPass, setRenderPass] = useState(0);
  const contentId = useId();
  const isExpandedControlled = typeof expanded === "boolean";
  const isExpanded = isExpandedControlled ? expanded : internalExpanded;
  const setExpandedState = useCallback(
    (value: boolean) => {
      if (!isExpandedControlled) {
        setInternalExpanded(value);
      }
      onExpandedChange?.(value);
    },
    [isExpandedControlled, onExpandedChange],
  );

  useEffect(() => {
    const container = contentRef.current;
    if (!container) {
      return undefined;
    }

    const diagrams = prepareMermaidBlocks(container);
    let cancelled = false;
    renderMermaidBlocks(diagrams)
      ?.then(() => {
        if (!cancelled) {
          setRenderPass((value) => value + 1);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          console.warn("Failed to render Mermaid diagram", error);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [html]);

  useLayoutEffect(() => {
    const element = contentRef.current;
    if (!element) {
      return undefined;
    }

    let frame = 0;
    const measure = () => {
      const nextMetrics = calculateMetrics(element);
      setMetrics((current) => {
        if (
          current?.shouldCollapse &&
          nextMetrics.shouldCollapse &&
          !isExpanded &&
          current.collapseHeight === nextMetrics.collapseHeight
        ) {
          return current;
        }
        if (nextMetrics.shouldCollapse) {
          return metricsEqual(current, nextMetrics) ? current : nextMetrics;
        }
        if (!current?.shouldCollapse) {
          return current;
        }
        return metricsEqual(current, nextMetrics) ? current : nextMetrics;
      });
      if (!nextMetrics.shouldCollapse && isExpanded) {
        setExpandedState(false);
      }
    };

    measure();

    const resizeObserver =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            if (typeof window.requestAnimationFrame !== "function") {
              measure();
              return;
            }
            if (typeof window.cancelAnimationFrame === "function") {
              window.cancelAnimationFrame(frame);
            }
            frame = window.requestAnimationFrame(measure);
          });

    resizeObserver?.observe(element);

    return () => {
      if (typeof window.cancelAnimationFrame === "function") {
        window.cancelAnimationFrame(frame);
      }
      resizeObserver?.disconnect();
    };
  }, [html, isExpanded, renderPass, setExpandedState]);

  if (!metrics?.shouldCollapse) {
    return (
      <div
        ref={contentRef}
        className="message-content long-message-content"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    );
  }

  const collapsed = !isExpanded;
  const toggleExpanded = () => {
    if (!isExpanded) {
      const element = contentRef.current;
      if (element) {
        const nextExpandedHeight = Math.ceil(element.scrollHeight);
        setMetrics((current) =>
          current
            ? {
                ...current,
                expandedHeight: nextExpandedHeight,
              }
            : current,
        );
      }
    }
    setExpandedState(!isExpanded);
  };
  const containerStyle = {
    maxHeight: `${isExpanded ? metrics.expandedHeight : metrics.collapseHeight}px`,
    transitionDuration: "0ms",
  } as const;
  const toggleLabel = isExpanded ? t("messageLongCollapse") : t("messageLongExpand");
  const ToggleIcon = isExpanded ? LongMessageCollapseIcon : LongMessageExpandIcon;

  return (
    <div className={classNames("long-message-collapse", collapsed && "is-collapsed", isExpanded && "is-expanded")}>
      <div
        ref={contentRef}
        id={contentId}
        className="message-content long-message-content"
        style={containerStyle}
        dangerouslySetInnerHTML={{ __html: html }}
      />
      <div className="long-message-actions">
        <Button
          type="button"
          variant="secondaryGray"
          size="sm"
          aria-controls={contentId}
          aria-expanded={isExpanded}
          className="long-message-toggle"
          onClick={toggleExpanded}
        >
          <ToggleIcon className="long-message-toggle-icon" aria-hidden="true" />
          <span>{toggleLabel}</span>
        </Button>
      </div>
    </div>
  );
}

function calculateMetrics(element: HTMLDivElement): LongMessageMetrics {
  const expandedHeight = measureNaturalExpandedHeight(element);
  const computedStyle = window.getComputedStyle(element);
  const lineHeight = measureLineHeight(computedStyle);
  const collapseHeight = Math.max(1, Math.min(COLLAPSE_MAX_HEIGHT_PX, Math.ceil(lineHeight * COLLAPSE_LINE_COUNT)));
  const hasMediaContent = hasImageLikeContent(element);
  const shouldCollapse =
    !hasMediaContent &&
    expandedHeight > Math.min(collapseHeight, COLLAPSE_MAX_HEIGHT_PX) + COLLAPSE_HEIGHT_TOLERANCE_PX;

  return {
    collapseHeight,
    expandedHeight,
    shouldCollapse,
  };
}

function metricsEqual(current: LongMessageMetrics | null, next: LongMessageMetrics): boolean {
  return (
    current?.collapseHeight === next.collapseHeight &&
    Math.abs(current.expandedHeight - next.expandedHeight) <= COLLAPSE_HEIGHT_TOLERANCE_PX &&
    current.shouldCollapse === next.shouldCollapse
  );
}

function measureNaturalExpandedHeight(element: HTMLDivElement): number {
  return Math.ceil(element.scrollHeight);
}

function hasImageLikeContent(element: HTMLDivElement): boolean {
  return Boolean(element.querySelector("img, picture, video"));
}

function measureLineHeight(computedStyle: CSSStyleDeclaration): number {
  const rawLineHeight = Number.parseFloat(computedStyle.lineHeight);
  if (Number.isFinite(rawLineHeight) && rawLineHeight > 0) {
    return rawLineHeight;
  }
  const fontSize = Number.parseFloat(computedStyle.fontSize);
  if (Number.isFinite(fontSize) && fontSize > 0) {
    return fontSize * 1.5;
  }
  return 24;
}
