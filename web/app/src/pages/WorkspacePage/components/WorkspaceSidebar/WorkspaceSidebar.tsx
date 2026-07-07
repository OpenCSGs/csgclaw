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
  modelProviders = null,
  modelProvidersLoaded = false,
  workerAgentItems,
  notificationAgentItems,
  workspaceTab,
  onWorkspaceTabChange,
  showHubNewBadge,
  taskCount,
  scheduledTaskCount,
  activeTaskBoardView,
  roomCount,
  channels,
  directMessages,
  threadGroups,
  activePane,
  activeThreadRootID,
  currentUserID,
  authBusy,
  authError,
  authPending,
  authStatus,
  usersById,
  collapsedWorkspaceGroups,
  showUpgradeControls,
  onToggleWorkspaceGroup,
  onCreateRoom,
  onCreateAgent,
  onCreateModelProvider,
  onCreateNotificationParticipant,
  onOpenCreateTeam,
  onOpenCreateTask,
  hub,
  onSelectHubSkill,
  onSelectHubTemplate,
  onSelectHub,
  onSelectTask,
  onSelectTaskBoardView,
  onViewTaskDetails,
  onSelectTeam,
  agentsError,
  onSelectConversation,
  onSelectThread,
  onPreviewUser,
  onSelectAgent,
  onSelectModelProvider,
  onSelectHuman,
  onPreviewAgent,
  onSelectComputer,
  appVersion,
  upgradeStatus,
  upgradeBusy,
  upgradePhase,
  upgradeError,
  suppressUpgradeIssue,
  onOpenUpgrade,
  onOpenConfigSettings,
  onLogin,
  onLogout,
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
            showHubNewBadge={showHubNewBadge}
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
            suppressUpgradeIssue={suppressUpgradeIssue}
            showUpgradeControls={showUpgradeControls}
            onOpenUpgrade={onOpenUpgrade}
            onOpenConfigSettings={onOpenConfigSettings}
            authStatus={authStatus}
            authBusy={authBusy}
            authPending={authPending}
            authError={authError}
            onLogin={onLogin}
            onLogout={onLogout}
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
            scheduledTaskCount={scheduledTaskCount}
            activeTaskBoardView={activeTaskBoardView}
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
            onCreateModelProvider={onCreateModelProvider}
            onCreateNotificationParticipant={onCreateNotificationParticipant}
            onOpenCreateTeam={onOpenCreateTeam}
            onOpenCreateTask={onOpenCreateTask}
            hub={hub}
            onSelectHubSkill={onSelectHubSkill}
            onSelectHubTemplate={onSelectHubTemplate}
            onSelectTask={onSelectTask}
            onSelectTaskBoardView={onSelectTaskBoardView}
            onViewTaskDetails={onViewTaskDetails}
            onSelectTeam={onSelectTeam}
            agentsError={agentsError}
            onSelectConversation={onSelectConversation}
            onSelectThread={onSelectThread}
            onPreviewUser={onPreviewUser}
            onSelectHuman={onSelectHuman}
            agentItems={agentItems}
            modelProviders={modelProviders}
            modelProvidersLoaded={modelProvidersLoaded}
            workerAgentItems={workerAgentItems}
            notificationAgentItems={notificationAgentItems}
            onSelectAgent={onSelectAgent}
            onSelectModelProvider={onSelectModelProvider}
            onPreviewAgent={onPreviewAgent}
            onSelectComputer={onSelectComputer}
          />
        </nav>
      </aside>
    </div>
  );
}
