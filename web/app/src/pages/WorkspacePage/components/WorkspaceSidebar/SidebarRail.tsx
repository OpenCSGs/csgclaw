// @ts-nocheck
import { WORKSPACE_TAB_AGENTS, WORKSPACE_TAB_HUB, WORKSPACE_TAB_MESSAGES } from "@/bootstrap/constants";
import { Button } from "@/components/ui";
import { HubIcon, RoomPlusIcon, RoomsIcon, SidebarToggleIcon, UsersIcon } from "@/components/ui/Icons";

export function SidebarRail({
  isSidebarCollapsed,
  onExpandSidebar,
  workspaceTab,
  onWorkspaceTabChange,
  onSelectHub,
  onCreateRoom,
  t,
}) {
  return (
    <div
      className={`sidebar-rail ${isSidebarCollapsed ? "visible" : ""}`}
      aria-hidden={!isSidebarCollapsed}
      inert={!isSidebarCollapsed}
    >
      <Button
        variant="ghost"
        className="sidebar-expand-button"
        aria-label={t("expandSidebar")}
        title={t("expandSidebar")}
        onClick={onExpandSidebar}
      >
        <span className="sidebar-toggle-mark">
          <SidebarToggleIcon />
        </span>
      </Button>
      <nav className="sidebar-rail-nav" aria-label="Workspace">
        <Button
          variant="ghost"
          className="sidebar-rail-button"
          active={workspaceTab === WORKSPACE_TAB_MESSAGES}
          aria-label={t("messagesTab")}
          title={t("messagesTab")}
          onClick={() => onWorkspaceTabChange(WORKSPACE_TAB_MESSAGES)}
        >
          <span className="sidebar-rail-icon" aria-hidden="true">
            <RoomsIcon />
          </span>
        </Button>
        <Button
          variant="ghost"
          className="sidebar-rail-button"
          active={workspaceTab === WORKSPACE_TAB_AGENTS}
          aria-label={t("agentsTab")}
          title={t("agentsTab")}
          onClick={() => onWorkspaceTabChange(WORKSPACE_TAB_AGENTS)}
        >
          <span className="sidebar-rail-icon" aria-hidden="true">
            <UsersIcon />
          </span>
        </Button>
        <Button
          variant="ghost"
          className="sidebar-rail-button"
          active={workspaceTab === WORKSPACE_TAB_HUB}
          aria-label={t("hubTab")}
          title={t("hubTab")}
          onClick={() => onSelectHub()}
        >
          <span className="sidebar-rail-icon" aria-hidden="true">
            <HubIcon />
          </span>
        </Button>
        <Button
          variant="ghost"
          className="sidebar-rail-button"
          aria-label={t("createRoom")}
          title={t("createRoom")}
          onClick={() => onCreateRoom()}
        >
          <span className="sidebar-rail-icon" aria-hidden="true">
            <RoomPlusIcon />
          </span>
        </Button>
      </nav>
    </div>
  );
}
