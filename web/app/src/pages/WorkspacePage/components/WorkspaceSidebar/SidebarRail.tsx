import { SidebarRailControlButton } from "./SidebarRailControlButton";
import { SidebarUserButton } from "./SidebarUserButton";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import type { SidebarRailProps } from "./types";

export function SidebarRail({
  isSidebarCollapsed,
  onExpandSidebar,
  workspaceTab,
  onWorkspaceTabChange,
  onSelectHub,
  roomCount,
  agentCount,
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
  appVersion,
  upgradeStatus,
  upgradeBusy,
  upgradePhase,
  upgradeError,
  onOpenUpgrade,
  t,
}: SidebarRailProps) {
  return (
    <div
      className={`sidebar-rail ${isSidebarCollapsed ? "visible" : ""}`}
      aria-hidden={!isSidebarCollapsed}
      inert={!isSidebarCollapsed}
    >
      <nav className="sidebar-rail-nav" aria-label="Workspace">
        <WorkspaceTabBar
          variant="rail"
          workspaceTab={workspaceTab}
          onWorkspaceTabChange={onWorkspaceTabChange}
          roomCount={roomCount}
          agentCount={agentCount}
          onSelectHub={onSelectHub}
          t={t}
        />
      </nav>
      <div className="sidebar-rail-bottom">
        <SidebarRailControlButton label={t("expandSidebar")} mode="expand" onClick={onExpandSidebar} />
        <SidebarUserButton
          theme={theme}
          onThemeChange={onThemeChange}
          locale={locale}
          onLocaleChange={onLocaleChange}
          appVersion={appVersion}
          upgradeStatus={upgradeStatus}
          upgradeBusy={upgradeBusy}
          upgradePhase={upgradePhase}
          upgradeError={upgradeError}
          onOpenUpgrade={onOpenUpgrade}
          t={t}
        />
      </div>
    </div>
  );
}
