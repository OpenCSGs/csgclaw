import { useEffect, useRef, useState } from "react";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { workspaceHasContextSidebar } from "@/models/routing";
import { SIDEBAR_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import { AppLayout, AppLayoutLoading, AppLayoutMain, AppLayoutOverlays, AppLayoutShell } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { WorkspaceMainPanel } from "../WorkspaceMainPanel";
import { WorkspaceOverlays } from "../WorkspaceOverlays";
import { WorkspaceSidebar } from "../WorkspaceSidebar";
import styles from "./WorkspaceLayout.module.css";
import type { CSSProperties, KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";

const SidebarWidth = {
  collapsedPrimary: 80,
  default: 600,
  max: 720,
  min: 560,
  primary: 300,
  step: 16,
} as const;

function clampSidebarWidth(value: number) {
  return Math.min(SidebarWidth.max, Math.max(SidebarWidth.min, Math.round(value)));
}

function readSidebarWidth() {
  if (typeof window === "undefined") {
    return SidebarWidth.default;
  }
  const stored = Number(window.localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY));
  return Number.isFinite(stored) ? clampSidebarWidth(stored) : SidebarWidth.default;
}

export function WorkspaceLayout() {
  const controller = useWorkspaceControllerContext();
  const [sidebarWidth, setSidebarWidth] = useState(readSidebarWidth);
  const [isSidebarResizing, setIsSidebarResizing] = useState(false);
  const resizeStartRef = useRef<{ pointerX: number; width: number }>({
    pointerX: 0,
    width: SidebarWidth.default,
  });
  const resizePointerIdRef = useRef<number | null>(null);
  const sidebarProps = controller.ready ? controller.sidebarProps : null;
  const isSidebarCollapsed = sidebarProps?.isSidebarCollapsed ?? false;
  const showSidebarContext = controller.ready && workspaceHasContextSidebar(controller.activePane);
  const contextSidebarWidth = showSidebarContext ? Math.max(0, sidebarWidth - SidebarWidth.primary) : 0;
  const primarySidebarWidth = isSidebarCollapsed ? SidebarWidth.collapsedPrimary : SidebarWidth.primary;
  const visibleSidebarWidth = showSidebarContext ? primarySidebarWidth + contextSidebarWidth : primarySidebarWidth;

  useEffect(() => {
    if (showSidebarContext) {
      window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(sidebarWidth));
    }
  }, [showSidebarContext, sidebarWidth]);

  function handleSidebarResizeStart(event: ReactPointerEvent<HTMLDivElement>) {
    if (!showSidebarContext || event.button !== 0) {
      return;
    }
    event.preventDefault();
    resizeStartRef.current = {
      pointerX: event.clientX,
      width: sidebarWidth,
    };
    resizePointerIdRef.current = event.pointerId;
    event.currentTarget.setPointerCapture(event.pointerId);
    setIsSidebarResizing(true);
  }

  function handleSidebarResizeMove(event: ReactPointerEvent<HTMLDivElement>) {
    if (resizePointerIdRef.current !== event.pointerId) {
      return;
    }
    event.preventDefault();
    const delta = event.clientX - resizeStartRef.current.pointerX;
    const nextWidth = clampSidebarWidth(resizeStartRef.current.width + delta);
    setSidebarWidth((current) => (current === nextWidth ? current : nextWidth));
  }

  function endSidebarResize(event: ReactPointerEvent<HTMLDivElement>) {
    if (resizePointerIdRef.current !== event.pointerId) {
      return;
    }
    resizePointerIdRef.current = null;
    setIsSidebarResizing(false);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  function handleSidebarResizeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!showSidebarContext) {
      return;
    }

    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      event.preventDefault();
      const direction = event.key === "ArrowLeft" ? -1 : 1;
      setSidebarWidth((current) => clampSidebarWidth(current + direction * SidebarWidth.step));
    }
    if (event.key === "Home") {
      event.preventDefault();
      setSidebarWidth(SidebarWidth.min);
    }
    if (event.key === "End") {
      event.preventDefault();
      setSidebarWidth(SidebarWidth.max);
    }
  }

  const baseShellClassName = controller.ready ? controller.shellClassName : "";
  const shellClassName = classNames(baseShellClassName, isSidebarResizing && styles.sidebarResizing);
  const shellStyle = {
    "--sidebar-expanded-width": `${showSidebarContext ? sidebarWidth : SidebarWidth.primary}px`,
    "--sidebar-slot-width": `${visibleSidebarWidth}px`,
  } as CSSProperties;

  return (
    <AppLayout ready={controller.ready} loadingFallback={<AppLayoutLoading>{controller.loadingText}</AppLayoutLoading>}>
      <AppLayoutShell className={shellClassName} style={shellStyle}>
        <div className={styles.sidebarShell}>{sidebarProps ? <WorkspaceSidebar {...sidebarProps} /> : null}</div>
        {controller.ready && showSidebarContext ? (
          <div
            className={styles.sidebarResizer}
            role="separator"
            aria-label="Resize sidebar"
            aria-orientation="vertical"
            aria-valuemin={SidebarWidth.min}
            aria-valuemax={SidebarWidth.max}
            aria-valuenow={sidebarWidth}
            tabIndex={0}
            onKeyDown={handleSidebarResizeKeyDown}
            onPointerDown={handleSidebarResizeStart}
            onPointerMove={handleSidebarResizeMove}
            onPointerUp={endSidebarResize}
            onPointerCancel={endSidebarResize}
            onLostPointerCapture={() => {
              resizePointerIdRef.current = null;
              setIsSidebarResizing(false);
            }}
          />
        ) : null}
        <AppLayoutMain>
          <WorkspaceMainPanel />
        </AppLayoutMain>
      </AppLayoutShell>
      <AppLayoutOverlays>
        <WorkspaceOverlays />
      </AppLayoutOverlays>
    </AppLayout>
  );
}
