import { SidebarFooter } from "./SidebarFooter";
import { SidebarHeader } from "./SidebarHeader";
import { SidebarRail } from "./SidebarRail";
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
        <SidebarHeader
          theme={theme}
          onThemeChange={onThemeChange}
          locale={locale}
          onLocaleChange={onLocaleChange}
          t={t}
          currentWorkspaceLabel={currentWorkspaceLabel}
          runningAgentCount={runningAgentCount}
          agentCount={agentCount}
          onCollapseSidebar={onCollapseSidebar}
        />
        <nav className="workspace-nav" aria-label="Workspace">
          <WorkspaceTabBar
            workspaceTab={workspaceTab}
            onWorkspaceTabChange={onWorkspaceTabChange}
            roomCount={roomCount}
            agentCount={agentCount}
            onSelectHub={onSelectHub}
            t={t}
          />
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
      </aside>
      <SidebarRail
        isSidebarCollapsed={isSidebarCollapsed}
        onExpandSidebar={onExpandSidebar}
        workspaceTab={workspaceTab}
        onWorkspaceTabChange={onWorkspaceTabChange}
        onSelectHub={onSelectHub}
        onCreateRoom={onCreateRoom}
        t={t}
      />
    </div>
  );
}
