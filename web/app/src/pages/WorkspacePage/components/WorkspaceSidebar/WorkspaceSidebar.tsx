import { SidebarFooter } from "./SidebarFooter";
import { SidebarHeader } from "./SidebarHeader";
import { SidebarRail } from "./SidebarRail";
import { SidebarUserButton } from "./SidebarUserButton";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import { WorkspaceTabPanels } from "./WorkspaceTabPanels";

export function WorkspaceSidebar({
  isSidebarCollapsed,
  onCollapseSidebar,
  onExpandSidebar,
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
  t,
  currentWorkspaceLabel,
  runningAgentCount,
  agentItems,
  workerAgentItems,
  notificationAgentItems,
  workspaceTab,
  onWorkspaceTabChange,
  roomCount,
  channels,
  directMessages,
  activePane,
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
}) {
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
              <SidebarUserButton
                theme={theme}
                onThemeChange={onThemeChange}
                locale={locale}
                onLocaleChange={onLocaleChange}
                onCollapseSidebar={onCollapseSidebar}
                t={t}
              />
            </div>
          </div>
          <div className="sidebar-main-column">
            <SidebarHeader
              t={t}
              currentWorkspaceLabel={currentWorkspaceLabel}
              runningAgentCount={runningAgentCount}
              agentCount={agentCount}
            />
            <nav className="workspace-nav" aria-label="Workspace">
              <WorkspaceTabPanels
                workspaceTab={workspaceTab}
                channels={channels}
                directMessages={directMessages}
                activePane={activePane}
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
                onPreviewUser={onPreviewUser}
                agentItems={agentItems}
                workerAgentItems={workerAgentItems}
                notificationAgentItems={notificationAgentItems}
                onSelectAgent={onSelectAgent}
                onPreviewAgent={onPreviewAgent}
                onSelectComputer={onSelectComputer}
              />
            </nav>
            <SidebarFooter
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
      </aside>
      <SidebarRail
        isSidebarCollapsed={isSidebarCollapsed}
        onExpandSidebar={onExpandSidebar}
        workspaceTab={workspaceTab}
        onWorkspaceTabChange={onWorkspaceTabChange}
        onSelectHub={onSelectHub}
        onCreateRoom={onCreateRoom}
        theme={theme}
        onThemeChange={onThemeChange}
        locale={locale}
        onLocaleChange={onLocaleChange}
        t={t}
      />
    </div>
  );
}
