import { useCallback, useEffect, useRef, useState } from "react";
import type { CSSProperties, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent, ReactNode } from "react";
import { Bot, MessageCircle, Minus } from "lucide-react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import type { ConversationPaneProps } from "@/components/business/ConversationPane";
import { Button } from "@/components/ui";
import type { LocaleCode, ThreadView, TranslateFn } from "@/models/conversations";
import { placeCaretAtEnd, renderComposerSegments } from "@/models/composer";
import { classNames } from "@/shared/lib/classNames";
import { FloatingChatGuide } from "./FloatingChatGuide";
import { FloatingChatPanel } from "./FloatingChatPanel";
import styles from "./FloatingChat.module.css";

export type FloatingChatProps = {
  avatar?: string | null;
  avatarFallback: string;
  chatProps: ConversationPaneProps | null;
  locale: LocaleCode;
  online?: boolean;
  open: boolean;
  statusLabel?: string;
  subtitle?: string;
  t: TranslateFn;
  threads?: ThreadView[];
  title: string;
  onOpenChange: (open: boolean) => void;
};

type FloatingChatResizeDirection = "left" | "top" | "corner";

type FloatingChatSize = {
  height: number;
  width: number;
};

const FLOATING_CHAT_DEFAULT_SIZE: FloatingChatSize = {
  height: 640,
  width: 640,
};
const FLOATING_CHAT_MIN_HEIGHT = 400;
const FLOATING_CHAT_MIN_WIDTH = 340;
const FLOATING_CHAT_MAX_HEIGHT = 760;
const FLOATING_CHAT_MAX_WIDTH = 840;
const FLOATING_CHAT_VIEWPORT_GAP = 24;
const FLOATING_CHAT_MOBILE_VIEWPORT_GAP = 12;
const FLOATING_CHAT_LAUNCHER_SIZE = 48;
const FLOATING_CHAT_LAUNCHER_DEFAULT_BOTTOM = 104;
const FLOATING_CHAT_LAUNCHER_MIN_BOTTOM = 24;
const FLOATING_CHAT_DRAG_THRESHOLD = 4;
const FLOATING_CHAT_LAYOUT_STORAGE_KEY = "csgclaw:floating-chat:layout:v1";
const FLOATING_CHAT_MANAGER_GUIDE_STORAGE_KEY = "csgclaw:floating-chat:manager-guide:v1";

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function finiteNumber(value: unknown): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }
  return value;
}

function getFloatingChatPanelBounds() {
  if (typeof window === "undefined") {
    return {
      maxHeight: FLOATING_CHAT_MAX_HEIGHT,
      maxWidth: FLOATING_CHAT_MAX_WIDTH,
      minHeight: FLOATING_CHAT_MIN_HEIGHT,
      minWidth: FLOATING_CHAT_MIN_WIDTH,
    };
  }
  const viewportGap = window.innerWidth <= 720 ? FLOATING_CHAT_MOBILE_VIEWPORT_GAP : FLOATING_CHAT_VIEWPORT_GAP;
  const maxWidth = Math.min(FLOATING_CHAT_MAX_WIDTH, Math.max(280, window.innerWidth - viewportGap * 2));
  const maxHeight = Math.min(FLOATING_CHAT_MAX_HEIGHT, Math.max(360, window.innerHeight - viewportGap * 2));
  return {
    maxHeight,
    maxWidth,
    minHeight: Math.min(FLOATING_CHAT_MIN_HEIGHT, maxHeight),
    minWidth: Math.min(FLOATING_CHAT_MIN_WIDTH, maxWidth),
  };
}

function getFloatingChatLauncherMaxBottom() {
  if (typeof window === "undefined") {
    return FLOATING_CHAT_LAUNCHER_DEFAULT_BOTTOM;
  }
  return Math.max(
    FLOATING_CHAT_LAUNCHER_MIN_BOTTOM,
    window.innerHeight - FLOATING_CHAT_LAUNCHER_SIZE - FLOATING_CHAT_VIEWPORT_GAP,
  );
}

function normalizeFloatingChatSize(size: Partial<FloatingChatSize> | null | undefined): FloatingChatSize {
  const bounds = getFloatingChatPanelBounds();
  return {
    height: clamp(finiteNumber(size?.height) ?? FLOATING_CHAT_DEFAULT_SIZE.height, bounds.minHeight, bounds.maxHeight),
    width: clamp(finiteNumber(size?.width) ?? FLOATING_CHAT_DEFAULT_SIZE.width, bounds.minWidth, bounds.maxWidth),
  };
}

function readFloatingChatLayout() {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const raw = window.localStorage.getItem(FLOATING_CHAT_LAYOUT_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    return JSON.parse(raw) as {
      launcherBottom?: unknown;
      panelSize?: Partial<FloatingChatSize> | null;
    };
  } catch {
    return {};
  }
}

function writeFloatingChatLayout(nextLayout: { launcherBottom?: number; panelSize?: FloatingChatSize }) {
  if (typeof window === "undefined") {
    return;
  }
  const current = readFloatingChatLayout();
  window.localStorage.setItem(
    FLOATING_CHAT_LAYOUT_STORAGE_KEY,
    JSON.stringify({
      ...current,
      ...nextLayout,
    }),
  );
}

function readFloatingChatManagerGuideSeen() {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return window.localStorage.getItem(FLOATING_CHAT_MANAGER_GUIDE_STORAGE_KEY) === "seen";
  } catch {
    return false;
  }
}

function writeFloatingChatManagerGuideSeen() {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(FLOATING_CHAT_MANAGER_GUIDE_STORAGE_KEY, "seen");
  } catch {
    // Storage can be unavailable in strict privacy modes; keep the UI usable.
  }
}

function getInitialFloatingChatLauncherBottom() {
  const layout = readFloatingChatLayout();
  return clamp(
    finiteNumber(layout.launcherBottom) ?? FLOATING_CHAT_LAUNCHER_DEFAULT_BOTTOM,
    FLOATING_CHAT_LAUNCHER_MIN_BOTTOM,
    getFloatingChatLauncherMaxBottom(),
  );
}

function getInitialFloatingChatPanelSize() {
  return normalizeFloatingChatSize(readFloatingChatLayout().panelSize);
}

export function FloatingChat({
  avatar,
  avatarFallback,
  chatProps,
  open,
  statusLabel = "",
  subtitle = "",
  t,
  title,
  onOpenChange,
}: FloatingChatProps) {
  const [launcherBottom, setLauncherBottom] = useState(getInitialFloatingChatLauncherBottom);
  const [launcherDragX, setLauncherDragX] = useState(0);
  const [launcherDragging, setLauncherDragging] = useState(false);
  const [managerGuideSeen, setManagerGuideSeen] = useState(readFloatingChatManagerGuideSeen);
  const [panelResizing, setPanelResizing] = useState(false);
  const [panelSize, setPanelSize] = useState(getInitialFloatingChatPanelSize);
  const launcherBottomRef = useRef(launcherBottom);
  const launcherDraggedRef = useRef(false);
  const panelSizeRef = useRef(panelSize);
  const accessibleTitle = title || t("floatingChatTitleFallback");

  useEffect(() => {
    if (!open) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onOpenChange(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onOpenChange, open]);

  useEffect(() => {
    launcherBottomRef.current = launcherBottom;
  }, [launcherBottom]);

  useEffect(() => {
    panelSizeRef.current = panelSize;
  }, [panelSize]);

  useEffect(() => {
    const handleResize = () => {
      const bounds = getFloatingChatPanelBounds();
      setPanelSize((current) => ({
        height: clamp(current.height, bounds.minHeight, bounds.maxHeight),
        width: clamp(current.width, bounds.minWidth, bounds.maxWidth),
      }));
      setLauncherBottom((current) => {
        const maxBottom = getFloatingChatLauncherMaxBottom();
        return clamp(current, FLOATING_CHAT_LAUNCHER_MIN_BOTTOM, maxBottom);
      });
    };
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const handleLauncherPointerDown = useCallback((event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) {
      return;
    }
    const startX = event.clientX;
    const startY = event.clientY;
    const startBottom = launcherBottomRef.current;
    let moved = false;

    try {
      event.currentTarget.setPointerCapture(event.pointerId);
    } catch {
      // Ignore browsers that do not keep capture on this element.
    }

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const deltaX = moveEvent.clientX - startX;
      const deltaY = moveEvent.clientY - startY;
      if (!moved && Math.hypot(deltaX, deltaY) < FLOATING_CHAT_DRAG_THRESHOLD) {
        return;
      }
      moved = true;
      setLauncherDragging(true);
      const maxBottom = getFloatingChatLauncherMaxBottom();
      const nextBottom = clamp(startBottom - deltaY, FLOATING_CHAT_LAUNCHER_MIN_BOTTOM, maxBottom);
      const maxLeftDrag = Math.max(0, window.innerWidth - FLOATING_CHAT_LAUNCHER_SIZE - FLOATING_CHAT_VIEWPORT_GAP * 2);
      launcherBottomRef.current = nextBottom;
      setLauncherBottom(nextBottom);
      setLauncherDragX(clamp(deltaX, -maxLeftDrag, FLOATING_CHAT_VIEWPORT_GAP));
    };

    const finishDrag = () => {
      launcherDraggedRef.current = moved;
      if (moved) {
        writeFloatingChatLayout({ launcherBottom: launcherBottomRef.current });
      }
      setLauncherDragging(false);
      setLauncherDragX(0);
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", finishDrag);
      window.removeEventListener("pointercancel", finishDrag);
      window.setTimeout(() => {
        launcherDraggedRef.current = false;
      }, 0);
    };

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", finishDrag);
    window.addEventListener("pointercancel", finishDrag);
  }, []);

  const markManagerGuideSeen = useCallback(() => {
    setManagerGuideSeen(true);
    writeFloatingChatManagerGuideSeen();
  }, []);

  const handleLauncherClick = useCallback(
    (event: ReactMouseEvent<HTMLButtonElement>) => {
      if (launcherDraggedRef.current) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }
      markManagerGuideSeen();
      onOpenChange(true);
    },
    [markManagerGuideSeen, onOpenChange],
  );

  const handleGuideOpen = useCallback(() => {
    markManagerGuideSeen();
    onOpenChange(true);
  }, [markManagerGuideSeen, onOpenChange]);

  const handlePanelResizeStart = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>, direction: FloatingChatResizeDirection) => {
      event.preventDefault();
      event.stopPropagation();

      const startX = event.clientX;
      const startY = event.clientY;
      const startSize = panelSizeRef.current;
      setPanelResizing(true);

      try {
        event.currentTarget.setPointerCapture(event.pointerId);
      } catch {
        // Window-level listeners below keep the drag stable after capture loss.
      }

      const handlePointerMove = (moveEvent: PointerEvent) => {
        const bounds = getFloatingChatPanelBounds();
        const nextWidth =
          direction === "left" || direction === "corner"
            ? startSize.width - (moveEvent.clientX - startX)
            : startSize.width;
        const nextHeight =
          direction === "top" || direction === "corner"
            ? startSize.height - (moveEvent.clientY - startY)
            : startSize.height;

        const nextSize = {
          height: clamp(nextHeight, bounds.minHeight, bounds.maxHeight),
          width: clamp(nextWidth, bounds.minWidth, bounds.maxWidth),
        };
        panelSizeRef.current = nextSize;
        setPanelSize(nextSize);
      };

      const finishResize = () => {
        writeFloatingChatLayout({ panelSize: panelSizeRef.current });
        setPanelResizing(false);
        window.removeEventListener("pointermove", handlePointerMove);
        window.removeEventListener("pointerup", finishResize);
        window.removeEventListener("pointercancel", finishResize);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      };

      const cursorByDirection: Record<FloatingChatResizeDirection, string> = {
        corner: "nwse-resize",
        left: "ew-resize",
        top: "ns-resize",
      };
      document.body.style.cursor = cursorByDirection[direction];
      document.body.style.userSelect = "none";
      window.addEventListener("pointermove", handlePointerMove);
      window.addEventListener("pointerup", finishResize);
      window.addEventListener("pointercancel", finishResize);
    },
    [],
  );

  const handlePickPrompt = useCallback(
    (text: string) => {
      const editor = chatProps?.editorRef.current;
      if (!editor || !chatProps) {
        return;
      }
      renderComposerSegments(editor, [{ text, type: "text" }]);
      chatProps.onSyncComposer();
      placeCaretAtEnd(editor);
    },
    [chatProps],
  );

  const collapsedRootStyle = {
    "--floating-chat-collapsed-bottom": `${launcherBottom}px`,
    "--floating-chat-drag-x": `${launcherDragX}px`,
  } as CSSProperties;

  const panelStyle = {
    "--floating-chat-panel-height": `${panelSize.height}px`,
    "--floating-chat-panel-width": `${panelSize.width}px`,
  } as CSSProperties;
  const showManagerGuide = !managerGuideSeen && !launcherDragging;

  if (!open) {
    return (
      <div
        className={classNames(
          styles.root,
          styles.collapsed,
          showManagerGuide && styles.guided,
          launcherDragging && styles.dragging,
        )}
        style={collapsedRootStyle}
      >
        {showManagerGuide ? (
          <FloatingChatGuide
            title={t("floatingChatGuideTitle")}
            dismissLabel={t("floatingChatGuideDismiss")}
            onDismiss={markManagerGuideSeen}
            onOpen={handleGuideOpen}
          />
        ) : null}
        <button
          type="button"
          className={styles.launcher}
          aria-label={t("floatingChatOpen")}
          title={t("floatingChatOpen")}
          onClick={handleLauncherClick}
          onPointerDown={handleLauncherPointerDown}
        >
          <MessageCircle size={21} strokeWidth={2.1} aria-hidden="true" />
        </button>
      </div>
    );
  }

  const headerAccessory = (
    <Button
      className={classNames("icon-button", styles.collapseButton)}
      aria-label={t("floatingChatCollapse")}
      title={t("floatingChatCollapse")}
      onClick={() => onOpenChange(false)}
    >
      <span className="icon-button-mark" aria-hidden="true">
        <Minus size={17} strokeWidth={2.2} />
      </span>
    </Button>
  );

  return (
    <div className={classNames(styles.root, styles.expanded)}>
      <FloatingChatFrame
        label={accessibleTitle}
        resizing={panelResizing}
        style={panelStyle}
        onResizeStart={handlePanelResizeStart}
      >
        {chatProps?.conversation ? (
          <FloatingChatPanel
            agentName={accessibleTitle}
            chatProps={chatProps}
            headerAccessory={headerAccessory}
            onPickPrompt={handlePickPrompt}
          />
        ) : (
          <div className={styles.emptyPanel}>
            <div className={styles.emptyHeader}>
              <span className={styles.emptyAvatar} aria-hidden="true">
                <AgentAvatarContent avatar={avatar} fallback={avatarFallback} />
              </span>
              <div className={styles.emptyTitleBlock}>
                <div className={styles.emptyTitle}>{accessibleTitle}</div>
                {subtitle || statusLabel ? <div className={styles.emptySubtitle}>{subtitle || statusLabel}</div> : null}
              </div>
            </div>
            <div className={styles.emptyBody}>
              <span className={styles.emptyMark} aria-hidden="true">
                <Bot size={20} strokeWidth={2} />
              </span>
              <strong>{t("floatingChatUnavailable")}</strong>
            </div>
          </div>
        )}
      </FloatingChatFrame>
    </div>
  );
}

type FloatingChatFrameProps = {
  children: ReactNode;
  label: string;
  resizing: boolean;
  style: CSSProperties;
  onResizeStart: (event: ReactPointerEvent<HTMLDivElement>, direction: FloatingChatResizeDirection) => void;
};

function FloatingChatFrame({ children, label, resizing, style, onResizeStart }: FloatingChatFrameProps) {
  return (
    <section className={classNames(styles.panel, resizing && styles.resizing)} aria-label={label} style={style}>
      <FloatingChatResizeHandles onResizeStart={onResizeStart} />
      {children}
    </section>
  );
}

type FloatingChatResizeHandlesProps = {
  onResizeStart: (event: ReactPointerEvent<HTMLDivElement>, direction: FloatingChatResizeDirection) => void;
};

function FloatingChatResizeHandles({ onResizeStart }: FloatingChatResizeHandlesProps) {
  return (
    <>
      <div
        aria-hidden="true"
        className={classNames(styles.resizeHandle, styles.left)}
        onPointerDown={(event) => onResizeStart(event, "left")}
      />
      <div
        aria-hidden="true"
        className={classNames(styles.resizeHandle, styles.top)}
        onPointerDown={(event) => onResizeStart(event, "top")}
      />
      <div
        aria-hidden="true"
        className={classNames(styles.resizeHandle, styles.corner)}
        onPointerDown={(event) => onResizeStart(event, "corner")}
      />
    </>
  );
}
