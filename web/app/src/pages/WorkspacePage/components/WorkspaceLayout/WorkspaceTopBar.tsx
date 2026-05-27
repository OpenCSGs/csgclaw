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
      <div className="workspace-topbar-brand" aria-label="CSGClaw">
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
          <SidebarRailControlButton
            label={isSidebarCollapsed ? expandSidebarLabel : collapseSidebarLabel}
            mode={isSidebarCollapsed ? "expand" : "collapse"}
            onClick={isSidebarCollapsed ? onExpandSidebar : onCollapseSidebar}
          />
        </div>
      </div>
    </header>
  );
}
