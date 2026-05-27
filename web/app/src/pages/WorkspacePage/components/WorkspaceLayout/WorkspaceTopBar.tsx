import { SidebarRailControlButton } from "../WorkspaceSidebar/SidebarRailControlButton";

type WorkspaceTopBarProps = {
  isSidebarCollapsed: boolean;
  onCollapseSidebar: () => void;
  onExpandSidebar: () => void;
  collapseSidebarLabel: string;
  expandSidebarLabel: string;
};

export function WorkspaceTopBar({
  isSidebarCollapsed,
  onCollapseSidebar,
  onExpandSidebar,
  collapseSidebarLabel,
  expandSidebarLabel,
}: WorkspaceTopBarProps) {
  return (
    <header className="workspace-topbar">
      <div className={`workspace-topbar-brand ${isSidebarCollapsed ? "" : "expanded"}`} aria-label="CSGClaw">
        {isSidebarCollapsed ? (
          <div className="workspace-topbar-toggle workspace-topbar-expand-trigger">
            <span className="workspace-topbar-collapsed-logo" aria-hidden="true">
              <img
                className="workspace-topbar-logo-collapsed workspace-topbar-logo-collapsed-light"
                src="brand/csgclaw-logo-collapsed.svg"
                alt=""
              />
              <img
                className="workspace-topbar-logo-collapsed workspace-topbar-logo-collapsed-dark"
                src="brand/csgclaw-logo-collapsed-dark.svg"
                alt=""
              />
            </span>
            <SidebarRailControlButton label={expandSidebarLabel} mode="expand" onClick={onExpandSidebar} />
          </div>
        ) : (
          <>
            <img
              className="workspace-topbar-logo workspace-topbar-logo-light"
              src="brand/csgclaw-logo-light.svg"
              alt=""
              aria-hidden="true"
            />
            <img
              className="workspace-topbar-logo workspace-topbar-logo-dark"
              src="brand/csgclaw-logo-dark.svg"
              alt=""
              aria-hidden="true"
            />
            <div className="workspace-topbar-toggle">
              <SidebarRailControlButton label={collapseSidebarLabel} mode="collapse" onClick={onCollapseSidebar} />
            </div>
          </>
        )}
      </div>
    </header>
  );
}
