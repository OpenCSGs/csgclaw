import { SidebarUserButton } from "./SidebarUserButton";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import { WorkspaceTabPanels } from "./WorkspaceTabPanels";
import type { WorkspaceSidebarProps } from "./types";

export function WorkspaceSidebar({
  isSidebarCollapsed,
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
  taskCount,
  roomCount,
  channels,
  directMessages,
  threadGroups,
  activePane,
  activeThreadRootID,
  currentUserID,
  usersById,
  collapsedWorkspaceGroups,
  showUpgradeControls,
  onToggleWorkspaceGroup,
  onCreateRoom,
  onCreateAgent,
  onCreateNotificationParticipant,
  onOpenCreateTeam,
  onOpenCreateTask,
  hub,
  onSelectHubTemplate,
  onSelectHub,
  onSelectTask,
  onViewTaskDetails,
  onSelectTeam,
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
  onOpenConfigSettings,
  taskItems,
  teams,
  planningTaskID = "",
  startingTaskID = "",
}: WorkspaceSidebarProps) {
  const agentCount = agentItems.length || 0;

  return (
    <div className="sidebar-slot">
      <div className="workspace-side-rail" aria-label="Workspace shortcuts">
        <div className="workspace-side-rail-nav">
          <WorkspaceTabBar
            variant="rail"
            workspaceTab={workspaceTab}
            onWorkspaceTabChange={onWorkspaceTabChange}
            taskCount={taskCount}
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
            appVersion={appVersion}
            upgradeStatus={upgradeStatus}
            upgradeBusy={upgradeBusy}
            upgradePhase={upgradePhase}
            upgradeError={upgradeError}
            showUpgradeControls={showUpgradeControls}
            onOpenUpgrade={onOpenUpgrade}
            onOpenConfigSettings={onOpenConfigSettings}
            t={t}
          />
        </div>
      </div>
      <aside
        className={`sidebar-main-column ${isSidebarCollapsed ? "collapsed" : ""}`}
        aria-hidden={isSidebarCollapsed}
        inert={isSidebarCollapsed}
      >
        <nav className="workspace-nav" aria-label="Workspace">
          <WorkspaceTabPanels
            workspaceTab={workspaceTab}
            taskCount={taskCount}
            taskItems={taskItems}
            teams={teams}
            planningTaskID={planningTaskID}
            startingTaskID={startingTaskID}
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
            onCreateNotificationParticipant={onCreateNotificationParticipant}
            onOpenCreateTeam={onOpenCreateTeam}
            onOpenCreateTask={onOpenCreateTask}
            hub={hub}
            onSelectHubTemplate={onSelectHubTemplate}
            onSelectTask={onSelectTask}
            onViewTaskDetails={onViewTaskDetails}
            onSelectTeam={onSelectTeam}
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
      </aside>
    </div>
  );
}
