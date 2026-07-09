import { useEffect, useMemo, useRef, useState } from "react";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { workspaceHasContextSidebar } from "@/models/routing";
import { PRIMARY_SIDEBAR_WIDTH_STORAGE_KEY, SIDEBAR_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import { AppLayout, AppLayoutLoading, AppLayoutMain, AppLayoutOverlays, AppLayoutShell } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { WorkspaceMainPanel } from "../WorkspaceMainPanel";
import { WorkspaceOverlays } from "../WorkspaceOverlays";
import { WorkspaceSidebar } from "../WorkspaceSidebar";
import {
  SidebarWidth,
  clampPrimarySidebarWidth,
  normalizeStoredPrimarySidebarWidth,
  workspacePrimarySidebarLabels,
  workspacePrimarySidebarWidth,
  workspacePrimarySidebarWidthBounds,
  workspaceSidebarWidthBounds,
} from "./sidebarDimensions";
import styles from "./WorkspaceLayout.module.css";
import type { CSSProperties, KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";

function clampSidebarWidth(value: number, bounds: { max: number; min: number }) {
  return Math.min(bounds.max, Math.max(bounds.min, Math.round(value)));
}

function readSidebarWidth() {
  if (typeof window === "undefined") {
    return SidebarWidth.default;
  }
  const stored = Number(window.localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY));
  return Number.isFinite(stored) ? Math.round(stored) : SidebarWidth.default;
}

function readPrimarySidebarWidthOverride() {
  if (typeof window === "undefined") {
    return null;
  }
  return normalizeStoredPrimarySidebarWidth(window.localStorage.getItem(PRIMARY_SIDEBAR_WIDTH_STORAGE_KEY));
}

export function WorkspaceLayout() {
  const controller = useWorkspaceControllerContext();
  const [sidebarWidth, setSidebarWidth] = useState(readSidebarWidth);
  const [primarySidebarWidthOverride, setPrimarySidebarWidthOverride] = useState<number | null>(
    readPrimarySidebarWidthOverride,
  );
  const [isSidebarResizing, setIsSidebarResizing] = useState(false);
  const [isPrimarySidebarResizing, setIsPrimarySidebarResizing] = useState(false);
  const resizeStartRef = useRef<{ pointerX: number; width: number }>({
    pointerX: 0,
    width: SidebarWidth.default,
  });
  const primaryResizeStartRef = useRef<{ pointerX: number; width: number }>({
    pointerX: 0,
    width: SidebarWidth.primaryFallback,
  });
  const resizePointerIdRef = useRef<number | null>(null);
  const primaryResizePointerIdRef = useRef<number | null>(null);
  const sidebarProps = controller.ready ? controller.sidebarProps : null;
  const isSidebarCollapsed = sidebarProps?.isSidebarCollapsed ?? false;
  const showSidebarContext = controller.ready && workspaceHasContextSidebar(controller.activePane);
  const autoPrimarySidebarWidth = useMemo(
    () =>
      sidebarProps
        ? workspacePrimarySidebarWidth(workspacePrimarySidebarLabels(sidebarProps.t))
        : SidebarWidth.primaryFallback,
    [sidebarProps],
  );
  const primarySidebarWidthBounds = useMemo(() => workspacePrimarySidebarWidthBounds(), []);
  const primarySidebarExpandedWidth = primarySidebarWidthOverride ?? autoPrimarySidebarWidth;
  const sidebarWidthBounds = useMemo(() => workspaceSidebarWidthBounds(), []);
  const expandedSidebarWidth = clampSidebarWidth(sidebarWidth, sidebarWidthBounds);
  const contextSidebarWidth = showSidebarContext ? Math.max(0, expandedSidebarWidth - primarySidebarExpandedWidth) : 0;
  const primarySidebarVisibleWidth = isSidebarCollapsed ? SidebarWidth.collapsedPrimary : primarySidebarExpandedWidth;
  const visibleSidebarWidth = showSidebarContext
    ? primarySidebarVisibleWidth + contextSidebarWidth
    : primarySidebarVisibleWidth;

  useEffect(() => {
    setSidebarWidth((current) => {
      const next = clampSidebarWidth(current, sidebarWidthBounds);
      return next === current ? current : next;
    });
  }, [sidebarWidthBounds]);

  useEffect(() => {
    if (showSidebarContext) {
      window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(expandedSidebarWidth));
    }
  }, [expandedSidebarWidth, showSidebarContext]);

  function setPrimarySidebarUserWidth(width: number) {
    const nextWidth = clampPrimarySidebarWidth(width);
    setPrimarySidebarWidthOverride((current) => (current === nextWidth ? current : nextWidth));
    window.localStorage.setItem(PRIMARY_SIDEBAR_WIDTH_STORAGE_KEY, String(nextWidth));
  }

  function handleSidebarResizeStart(event: ReactPointerEvent<HTMLDivElement>) {
    if (!showSidebarContext || event.button !== 0) {
      return;
    }
    event.preventDefault();
    resizeStartRef.current = {
      pointerX: event.clientX,
      width: expandedSidebarWidth,
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
    const nextWidth = clampSidebarWidth(resizeStartRef.current.width + delta, sidebarWidthBounds);
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

  function handlePrimarySidebarResizeStart(event: ReactPointerEvent<HTMLDivElement>) {
    if (isSidebarCollapsed || event.button !== 0) {
      return;
    }
    event.preventDefault();
    primaryResizeStartRef.current = {
      pointerX: event.clientX,
      width: primarySidebarExpandedWidth,
    };
    primaryResizePointerIdRef.current = event.pointerId;
    event.currentTarget.setPointerCapture(event.pointerId);
    setIsPrimarySidebarResizing(true);
  }

  function handlePrimarySidebarResizeMove(event: ReactPointerEvent<HTMLDivElement>) {
    if (primaryResizePointerIdRef.current !== event.pointerId) {
      return;
    }
    event.preventDefault();
    const delta = event.clientX - primaryResizeStartRef.current.pointerX;
    setPrimarySidebarUserWidth(primaryResizeStartRef.current.width + delta);
  }

  function endPrimarySidebarResize(event: ReactPointerEvent<HTMLDivElement>) {
    if (primaryResizePointerIdRef.current !== event.pointerId) {
      return;
    }
    primaryResizePointerIdRef.current = null;
    setIsPrimarySidebarResizing(false);
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
      setSidebarWidth((current) => clampSidebarWidth(current + direction * SidebarWidth.step, sidebarWidthBounds));
    }
    if (event.key === "Home") {
      event.preventDefault();
      setSidebarWidth(sidebarWidthBounds.min);
    }
    if (event.key === "End") {
      event.preventDefault();
      setSidebarWidth(sidebarWidthBounds.max);
    }
  }

  function handlePrimarySidebarResizeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (isSidebarCollapsed) {
      return;
    }

    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      event.preventDefault();
      const direction = event.key === "ArrowLeft" ? -1 : 1;
      setPrimarySidebarUserWidth(primarySidebarExpandedWidth + direction * SidebarWidth.step);
    }
    if (event.key === "Home") {
      event.preventDefault();
      setPrimarySidebarUserWidth(primarySidebarWidthBounds.min);
    }
    if (event.key === "End") {
      event.preventDefault();
      setPrimarySidebarUserWidth(primarySidebarWidthBounds.max);
    }
  }

  const baseShellClassName = controller.ready ? controller.shellClassName : "";
  const shellClassName = classNames(
    baseShellClassName,
    isSidebarResizing && styles.sidebarResizing,
    isPrimarySidebarResizing && styles.primarySidebarResizing,
    isPrimarySidebarResizing && "workspace-primary-sidebar-resizing",
  );
  const shellStyle = {
    "--workspace-primary-sidebar-width": `${primarySidebarExpandedWidth}px`,
    "--sidebar-expanded-width": `${showSidebarContext ? expandedSidebarWidth : primarySidebarExpandedWidth}px`,
    "--sidebar-slot-width": `${visibleSidebarWidth}px`,
  } as CSSProperties;

  return (
    <AppLayout ready={controller.ready} loadingFallback={<AppLayoutLoading>{controller.loadingText}</AppLayoutLoading>}>
      <AppLayoutShell className={shellClassName} style={shellStyle}>
        <div className={styles.sidebarShell}>
          {sidebarProps ? <WorkspaceSidebar {...sidebarProps} /> : null}
          {controller.ready && !isSidebarCollapsed ? (
            <div
              className={styles.primarySidebarResizer}
              role="separator"
              aria-label="Resize primary sidebar"
              aria-orientation="vertical"
              aria-valuemin={primarySidebarWidthBounds.min}
              aria-valuemax={primarySidebarWidthBounds.max}
              aria-valuenow={primarySidebarExpandedWidth}
              tabIndex={0}
              onKeyDown={handlePrimarySidebarResizeKeyDown}
              onPointerDown={handlePrimarySidebarResizeStart}
              onPointerMove={handlePrimarySidebarResizeMove}
              onPointerUp={endPrimarySidebarResize}
              onPointerCancel={endPrimarySidebarResize}
              onLostPointerCapture={() => {
                primaryResizePointerIdRef.current = null;
                setIsPrimarySidebarResizing(false);
              }}
            />
          ) : null}
        </div>
        {controller.ready && showSidebarContext ? (
          <div
            className={styles.sidebarResizer}
            role="separator"
            aria-label="Resize sidebar"
            aria-orientation="vertical"
            aria-valuemin={sidebarWidthBounds.min}
            aria-valuemax={sidebarWidthBounds.max}
            aria-valuenow={expandedSidebarWidth}
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
