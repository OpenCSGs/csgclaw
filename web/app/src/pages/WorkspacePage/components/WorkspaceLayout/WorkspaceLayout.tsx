import { useEffect, useRef, useState } from "react";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { SIDEBAR_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import { AppLayout, AppLayoutLoading, AppLayoutMain, AppLayoutOverlays, AppLayoutShell } from "@/components/ui";
import { WorkspaceMainPanel } from "../WorkspaceMainPanel";
import { WorkspaceOverlays } from "../WorkspaceOverlays";
import { WorkspaceSidebar } from "../WorkspaceSidebar";
import { WorkspaceTopBar } from "./WorkspaceTopBar";
import type { CSSProperties, KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";

const SidebarWidth = {
  default: 352,
  max: 520,
  min: 292,
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
  const sidebarProps = controller.ready ? controller.sidebarProps : null;
  const isSidebarCollapsed = sidebarProps?.isSidebarCollapsed ?? false;

  useEffect(() => {
    if (!isSidebarCollapsed) {
      window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(sidebarWidth));
    }
  }, [isSidebarCollapsed, sidebarWidth]);

  useEffect(() => {
    if (!isSidebarResizing) {
      return undefined;
    }

    function handlePointerMove(event: PointerEvent) {
      const delta = event.clientX - resizeStartRef.current.pointerX;
      setSidebarWidth(clampSidebarWidth(resizeStartRef.current.width + delta));
    }

    function handlePointerUp() {
      setIsSidebarResizing(false);
    }

    document.documentElement.classList.add("workspace-sidebar-is-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("pointercancel", handlePointerUp);

    return () => {
      document.documentElement.classList.remove("workspace-sidebar-is-resizing");
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
    };
  }, [isSidebarResizing]);

  function handleSidebarResizeStart(event: ReactPointerEvent<HTMLDivElement>) {
    if (isSidebarCollapsed) {
      return;
    }
    event.preventDefault();
    resizeStartRef.current = {
      pointerX: event.clientX,
      width: sidebarWidth,
    };
    setIsSidebarResizing(true);
  }

  function handleSidebarResizeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (isSidebarCollapsed) {
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
  const shellClassName = `${baseShellClassName} ${isSidebarResizing ? "sidebar-resizing" : ""}`.trim();
  const shellStyle = {
    "--sidebar-expanded-width": `${sidebarWidth}px`,
    ...(isSidebarCollapsed ? {} : { "--sidebar-slot-width": `${sidebarWidth}px` }),
  } as CSSProperties;

  return (
    <AppLayout ready={controller.ready} loadingFallback={<AppLayoutLoading>{controller.loadingText}</AppLayoutLoading>}>
      <AppLayoutShell className={shellClassName} style={shellStyle}>
        <div className="workspace-sidebar-shell">
          {sidebarProps ? (
            <WorkspaceTopBar
              isSidebarCollapsed={isSidebarCollapsed}
              onCollapseSidebar={sidebarProps.onCollapseSidebar}
              onExpandSidebar={sidebarProps.onExpandSidebar}
              collapseSidebarLabel={sidebarProps.t("collapseSidebar")}
              expandSidebarLabel={sidebarProps.t("expandSidebar")}
            />
          ) : null}
          {sidebarProps ? <WorkspaceSidebar {...sidebarProps} /> : null}
        </div>
        {controller.ready ? (
          <div
            className="workspace-sidebar-resizer"
            role="separator"
            aria-label="Resize sidebar"
            aria-orientation="vertical"
            aria-valuemin={SidebarWidth.min}
            aria-valuemax={SidebarWidth.max}
            aria-valuenow={sidebarWidth}
            tabIndex={isSidebarCollapsed ? -1 : 0}
            onKeyDown={handleSidebarResizeKeyDown}
            onPointerDown={handleSidebarResizeStart}
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
