import { SidebarRail } from "./SidebarRail";
import { SidebarRailControlButton } from "./SidebarRailControlButton";
import { SidebarUserButton } from "./SidebarUserButton";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import { WorkspaceTabPanels } from "./WorkspaceTabPanels";
import type { WorkspaceSidebarProps } from "./types";

export function WorkspaceSidebar({
  isSidebarCollapsed,
  onCollapseSidebar,
  onExpandSidebar,
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
  t,
  agentItems,
  workerAgentItems,
  notificationAgentItems,
  workspaceTab,
  onWorkspaceTabChange,
  roomCount,
  channels,
  directMessages,
  threadGroups,
  activePane,
  activeThreadRootID,
  currentUserID,
  usersById,
  collapsedWorkspaceGroups,
  onToggleWorkspaceGroup,
  onCreateRoom,
  onCreateAgent,
  onCreateNotificationBot,
  hub,
  onSelectHubTemplate,
  onSelectHub,
  agentsError,
  onSelectConversation,
  onSelectThread,
  onPreviewUser,
  onSelectAgent,
  onPreviewAgent,
  onSelectComputer,
  appVersion,
  upgradeStatus,
  upgradeBusy,
  upgradePhase,
  upgradeError,
  onOpenUpgrade,
}: WorkspaceSidebarProps) {
  const agentCount = agentItems.length || 0;

  return (
    <div className="sidebar-slot">
      <aside
        className={`sidebar ${isSidebarCollapsed ? "collapsed" : ""}`}
        aria-hidden={isSidebarCollapsed}
        inert={isSidebarCollapsed}
      >
        <div className="sidebar-shell">
          <div className="workspace-side-rail" aria-label="Workspace shortcuts">
            <div className="workspace-side-rail-nav">
              <WorkspaceTabBar
                variant="rail"
                workspaceTab={workspaceTab}
                onWorkspaceTabChange={onWorkspaceTabChange}
                roomCount={roomCount}
                agentCount={agentCount}
                onSelectHub={onSelectHub}
                t={t}
              />
            </div>
            <div className="workspace-side-rail-bottom">
              <SidebarRailControlButton label={t("collapseSidebar")} mode="collapse" onClick={onCollapseSidebar} />
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
          <div className="sidebar-main-column">
            <nav className="workspace-nav" aria-label="Workspace">
              <WorkspaceTabPanels
                workspaceTab={workspaceTab}
                channels={channels}
                directMessages={directMessages}
                threadGroups={threadGroups}
                activePane={activePane}
                activeThreadRootID={activeThreadRootID}
                currentUserID={currentUserID}
                usersById={usersById}
                locale={locale}
                t={t}
                collapsedWorkspaceGroups={collapsedWorkspaceGroups}
                onToggleWorkspaceGroup={onToggleWorkspaceGroup}
                onCreateRoom={onCreateRoom}
                onCreateAgent={onCreateAgent}
                onCreateNotificationBot={onCreateNotificationBot}
                hub={hub}
                onSelectHubTemplate={onSelectHubTemplate}
                agentsError={agentsError}
                onSelectConversation={onSelectConversation}
                onSelectThread={onSelectThread}
                onPreviewUser={onPreviewUser}
                agentItems={agentItems}
                workerAgentItems={workerAgentItems}
                notificationAgentItems={notificationAgentItems}
                onSelectAgent={onSelectAgent}
                onPreviewAgent={onPreviewAgent}
                onSelectComputer={onSelectComputer}
              />
            </nav>
          </div>
        </div>
      </aside>
      <SidebarRail
        isSidebarCollapsed={isSidebarCollapsed}
        onExpandSidebar={onExpandSidebar}
        workspaceTab={workspaceTab}
        onWorkspaceTabChange={onWorkspaceTabChange}
        onSelectHub={onSelectHub}
        roomCount={roomCount}
        agentCount={agentCount}
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
  );
}
