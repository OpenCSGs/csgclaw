import { useCallback, useEffect, useRef, useState, type CSSProperties, type PointerEvent } from "react";
import { X } from "lucide-react";
import { Button, DialogCloseButton, DialogContent, DialogHeader, DialogRoot, DialogTitle } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import { CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import type { RenderedCitation } from "./markdown";

const DEFAULT_PANEL_WIDTH = 400;
const MIN_PANEL_WIDTH = 260;
const MIN_CONVERSATION_WIDTH = 360;

function readPanelWidth() {
  if (typeof window === "undefined") return DEFAULT_PANEL_WIDTH;
  try {
    const stored = Number(window.localStorage.getItem(CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY));
    return Number.isFinite(stored) && stored >= MIN_PANEL_WIDTH ? Math.round(stored) : DEFAULT_PANEL_WIDTH;
  } catch {
    return DEFAULT_PANEL_WIDTH;
  }
}

function persistPanelWidth(width: number) {
  try {
    window.localStorage.setItem(CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY, String(Math.round(width)));
  } catch {
    // Resizing remains available when browser storage is disabled.
  }
}

type CitationSourcesProps = {
  activeID: string | null;
  cited: RenderedCitation[];
  onActiveChange: (id: string | null) => void;
  t?: TranslateFn;
  variant?: "dialog" | "panel";
};

function label(t: TranslateFn | undefined, key: string, fallback: string) {
  if (!t) return fallback;
  const translated = t(key);
  return translated === key ? fallback : translated;
}

export function CitationSources({ activeID, cited, onActiveChange, t, variant = "dialog" }: CitationSourcesProps) {
  const panelRef = useRef<HTMLElement | null>(null);
  const resizeRef = useRef({ pointerID: -1, startWidth: DEFAULT_PANEL_WIDTH, startX: 0 });
  const [panelWidth, setPanelWidth] = useState(readPanelWidth);
  const [resizing, setResizing] = useState(false);
  const active = cited.find((item) => item.id === activeID);
  const title = label(t, "citationDrawerTitle", "来源");
  const closeLabel = label(t, "citationClose", "关闭");

  const clampPanelWidth = useCallback((width: number) => {
    const containerWidth = panelRef.current?.parentElement?.getBoundingClientRect().width || window.innerWidth;
    const maximum = Math.max(0, containerWidth - MIN_CONVERSATION_WIDTH);
    const minimum = Math.min(MIN_PANEL_WIDTH, maximum);
    return Math.max(minimum, Math.min(width, maximum));
  }, []);

  useEffect(() => {
    if (variant !== "panel") return;
    const updateWidth = () => setPanelWidth((current) => clampPanelWidth(current));
    updateWidth();
    const parent = panelRef.current?.parentElement;
    const observer = parent && typeof ResizeObserver !== "undefined" ? new ResizeObserver(updateWidth) : null;
    if (parent) observer?.observe(parent);
    window.addEventListener("resize", updateWidth);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateWidth);
    };
  }, [clampPanelWidth, variant]);

  useEffect(() => {
    if (variant === "panel" && !resizing) persistPanelWidth(panelWidth);
  }, [panelWidth, resizing, variant]);

  const handleResizeEnd = (event: PointerEvent<HTMLDivElement>) => {
    if (resizeRef.current.pointerID !== event.pointerId) return;
    resizeRef.current.pointerID = -1;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    setResizing(false);
  };

  if (variant === "panel") {
    return active ? (
      <aside
        ref={panelRef}
        className={classNames("conversation-citation-panel", resizing && "is-resizing")}
        aria-label={title}
        style={{ "--conversation-citation-panel-width": `${panelWidth}px` } as CSSProperties}
      >
        <div
          className="conversation-citation-panel-resize-handle"
          role="separator"
          tabIndex={0}
          aria-label={label(t, "citationPanelResize", "调整来源面板宽度")}
          aria-orientation="vertical"
          aria-valuenow={Math.round(panelWidth)}
          onKeyDown={(event) => {
            if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
            event.preventDefault();
            setPanelWidth((current) => clampPanelWidth(current + (event.key === "ArrowLeft" ? 24 : -24)));
          }}
          onPointerDown={(event) => {
            resizeRef.current = { pointerID: event.pointerId, startWidth: panelWidth, startX: event.clientX };
            event.currentTarget.setPointerCapture(event.pointerId);
            setResizing(true);
          }}
          onPointerMove={(event) => {
            if (resizeRef.current.pointerID !== event.pointerId) return;
            setPanelWidth(clampPanelWidth(resizeRef.current.startWidth + resizeRef.current.startX - event.clientX));
          }}
          onPointerCancel={handleResizeEnd}
          onPointerUp={handleResizeEnd}
        />
        <header className="conversation-citation-panel-header">
          <div>
            <h2>{title}</h2>
            <p>{label(t, "citationPanelDescription", "查看回答引用的文档与原文片段。")}</p>
          </div>
          <Button
            iconOnly
            size="sm"
            variant="tertiaryGray"
            aria-label={closeLabel}
            title={closeLabel}
            onClick={() => onActiveChange(null)}
          >
            <X aria-hidden="true" size={17} />
          </Button>
        </header>
        <article className="conversation-citation-panel-body">
          <h3>{active.title}</h3>
          {active.snippet ? <p>{active.snippet}</p> : null}
        </article>
      </aside>
    ) : null;
  }

  return (
    <DialogRoot open={Boolean(active)} onOpenChange={(open) => !open && onActiveChange(null)}>
      <DialogContent className="message-citation-drawer">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogCloseButton label={closeLabel} />
        </DialogHeader>
        {active ? (
          <article className="message-citation-source-detail">
            <h3>{active.title}</h3>
            {active.snippet ? <blockquote>{active.snippet}</blockquote> : null}
          </article>
        ) : null}
      </DialogContent>
    </DialogRoot>
  );
}
