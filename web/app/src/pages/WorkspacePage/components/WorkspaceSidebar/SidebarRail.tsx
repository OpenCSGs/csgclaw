import { Button } from "@/components/ui";
import { HubIcon, RoomPlusIcon, RoomsIcon, SidebarToggleIcon, UsersIcon } from "@/components/ui/Icons";
import { WorkspaceTabs } from "@/models/routing";
import { SidebarUserButton } from "./SidebarUserButton";

export function SidebarRail({
  isSidebarCollapsed,
  onExpandSidebar,
  workspaceTab,
  onWorkspaceTabChange,
  onSelectHub,
  onCreateRoom,
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
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
          active={workspaceTab === WorkspaceTabs.messages}
          aria-label={t("messagesTab")}
          title={t("messagesTab")}
          onClick={() => onWorkspaceTabChange(WorkspaceTabs.messages)}
        >
          <span className="sidebar-rail-icon" aria-hidden="true">
            <RoomsIcon />
          </span>
        </Button>
        <Button
          variant="ghost"
          className="sidebar-rail-button"
          active={workspaceTab === WorkspaceTabs.agents}
          aria-label={t("agentsTab")}
          title={t("agentsTab")}
          onClick={() => onWorkspaceTabChange(WorkspaceTabs.agents)}
        >
          <span className="sidebar-rail-icon" aria-hidden="true">
            <UsersIcon />
          </span>
        </Button>
        <Button
          variant="ghost"
          className="sidebar-rail-button"
          active={workspaceTab === WorkspaceTabs.hub}
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
      <div className="sidebar-rail-bottom">
        <SidebarUserButton
          theme={theme}
          onThemeChange={onThemeChange}
          locale={locale}
          onLocaleChange={onLocaleChange}
          onCollapseSidebar={onExpandSidebar}
          sidebarActionLabel={t("expandSidebar")}
          t={t}
        />
      </div>
    </div>
  );
}
