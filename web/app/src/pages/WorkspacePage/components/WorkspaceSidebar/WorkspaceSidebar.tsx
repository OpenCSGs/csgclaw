import { useEffect, useMemo, useRef, useState } from "react";
import { PanelLeftOpen, Plus, Search } from "lucide-react";
import {
  SidebarAlertTriangleIcon,
  SidebarBoxIcon,
  SidebarGrid07Icon,
  SidebarLaptopIcon,
  SidebarListUnordered4Icon,
  SidebarMessageIcon,
  SidebarPuzzlePiece02Icon,
  SidebarRobotIcon,
  SidebarTimer2Icon,
  SidebarUserIcon,
  SidebarUsersIcon,
} from "@/components/ui/Icons";
import { SidebarRailControlButton } from "./SidebarRailControlButton";
import { SidebarUserButton } from "./SidebarUserButton";
import { LogoMark, LogoWordmark } from "./WorkspaceSidebarBrand";
import { WorkspacePrimaryNavigation } from "./WorkspacePrimaryNavigation";
import { WorkspaceTabPanels } from "./WorkspaceTabPanels";
import { WorkspaceContextSectionIds } from "./types";
import { WorkspacePaneTypes, WorkspaceTabs, workspaceHasContextSidebar } from "@/models/routing";
import { classNames } from "@/shared/lib/classNames";
import styles from "./WorkspaceSidebar.module.css";
import type { PrimaryNavigationItem, PrimaryNavigationSection } from "./WorkspacePrimaryNavigation";
import type { WorkspaceContextSectionId, WorkspaceSidebarProps } from "./types";
import type { ComponentType } from "react";

type SidebarNavigationIcon = ComponentType<{ size?: number | string }>;

const WORKSPACE_NAVIGATION_ICONS = {
  agents: SidebarRobotIcon,
  computers: SidebarLaptopIcon,
  humans: SidebarUserIcon,
  messages: SidebarMessageIcon,
  models: SidebarBoxIcon,
  notifications: SidebarAlertTriangleIcon,
  scheduledTasks: SidebarTimer2Icon,
  skills: SidebarPuzzlePiece02Icon,
  tasks: SidebarListUnordered4Icon,
  teams: SidebarUsersIcon,
  templates: SidebarGrid07Icon,
} as const;

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
  taskCount,
  scheduledTaskCount,
  activeTaskBoardView,
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
  currentWorkspaceLabel,
  showUpgradeControls,
  onToggleWorkspaceGroup,
  onCreateRoom,
  onCreateAgent,
  onCreateModelProvider,
  onCreateNotificationParticipant,
  onOpenCreateTeam,
  onOpenCreateTask,
  onOpenCreateScheduledTask,
  hub,
  onSelectHubSkill,
  onSelectHubTemplate,
  onSelectHub,
  onSelectTask,
  onSelectTaskBoardView,
  onViewTaskDetails,
  onSelectTeam,
  onSelectTeamSection,
  agentsError,
  onSelectConversation,
  onSelectThread,
  onPreviewUser,
  onSelectAgent,
  onSelectNotificationSection,
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
  onOpenSettings,
  onCollapseSidebar,
  onExpandSidebar,
  onLogin,
  onLogout,
  taskItems,
  teams,
  planningTaskID = "",
  startingTaskID = "",
}: WorkspaceSidebarProps) {
  const [contextQuery, setContextQuery] = useState("");
  const [skillUploadOpen, setSkillUploadOpen] = useState(false);
  const contextNavRef = useRef<HTMLElement | null>(null);
  const currentUser = usersById.get(currentUserID);
  const hasContextSidebar = workspaceHasContextSidebar(activePane);
  const firstWorkerAgent = workerAgentItems[0] ?? agentItems[0] ?? null;
  const firstNotificationAgent = notificationAgentItems[0] ?? null;
  const firstTeam = teams[0] ?? null;
  const firstHubTemplate = hub?.templates[0] ?? null;
  const firstHubSkill = hub?.skills[0] ?? null;
  const firstModelProvider = modelProviders?.providers[0] ?? null;
  const notificationAgentIds = useMemo(
    () => new Set(notificationAgentItems.map((item) => item.id).filter((id): id is string => Boolean(id))),
    [notificationAgentItems],
  );
  const routeContextSectionId = useMemo(
    () =>
      contextSectionIdForPane({
        activePane,
        notificationAgentIds,
        workspaceTab,
      }),
    [activePane, notificationAgentIds, workspaceTab],
  );
  const [activeContextSectionId, setActiveContextSectionId] = useState<WorkspaceContextSectionId>(
    () => routeContextSectionId ?? defaultContextSectionIdForTab(workspaceTab),
  );

  useEffect(() => {
    if (routeContextSectionId) {
      setActiveContextSectionId(routeContextSectionId);
    }
  }, [routeContextSectionId]);

  const primaryNavigationSections = useMemo<PrimaryNavigationSection[]>(
    () => [
      {
        id: "messages",
        label: t("messagesTab"),
        items: [
          {
            active:
              activePane.type !== WorkspacePaneTypes.settings &&
              activeContextSectionId === WorkspaceContextSectionIds.messages,
            groupId: WorkspaceContextSectionIds.messages,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.messages),
            id: "messages",
            label: t("messagesTab"),
            onSelect: () => onWorkspaceTabChange(WorkspaceTabs.messages),
          },
        ],
      },
      {
        id: "agents",
        label: t("agentsTab"),
        items: [
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.agents,
            groupId: WorkspaceContextSectionIds.agents,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.agents),
            id: "agents",
            label: t("computerAgentsSection"),
            onSelect: () => {
              if (firstWorkerAgent) {
                onSelectAgent(firstWorkerAgent);
              }
            },
          },
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.humans,
            groupId: WorkspaceContextSectionIds.humans,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.humans),
            id: "humans",
            label: t("humanSection"),
            onSelect: () => {
              if (currentUser) {
                onSelectHuman(currentUser);
              }
            },
          },
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.computers,
            groupId: WorkspaceContextSectionIds.computers,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.computers),
            id: "computers",
            label: t("computersSection"),
            onSelect: onSelectComputer,
          },
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.notifications,
            groupId: WorkspaceContextSectionIds.notifications,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.notifications),
            id: "notifications",
            label: t("notificationsSection"),
            onSelect: () => {
              if (firstNotificationAgent) {
                onSelectAgent(firstNotificationAgent);
                return;
              }
              onSelectNotificationSection();
            },
          },
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.teams,
            groupId: WorkspaceContextSectionIds.teams,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.teams),
            id: "teams",
            label: t("teamsSection"),
            onSelect: () => {
              if (firstTeam) {
                onSelectTeam(firstTeam);
                return;
              }
              onSelectTeamSection();
            },
          },
        ],
      },
      {
        id: "tasks",
        label: t("tasksTab"),
        items: [
          {
            active: activePane.type === WorkspacePaneTypes.task && activeTaskBoardView !== "scheduled",
            groupId: WorkspaceContextSectionIds.tasks,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.tasks),
            id: "tasks",
            label: t("tasksTab"),
            onSelect: () => {
              onSelectTaskBoardView?.("tasks");
              onSelectTask();
            },
          },
          {
            active: activePane.type === WorkspacePaneTypes.task && activeTaskBoardView === "scheduled",
            groupId: WorkspaceContextSectionIds.tasks,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.scheduledTasks),
            id: "scheduled-tasks",
            label: t("scheduledTasksTab"),
            onSelect: () => {
              onSelectTaskBoardView?.("scheduled");
              onSelectTask();
            },
          },
        ],
      },
      {
        id: "resources",
        label: t("resourcesTab"),
        items: [
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.hubTemplates,
            badge: badgeCount(hub?.templates.length),
            groupId: WorkspaceContextSectionIds.hubTemplates,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.templates),
            id: "templates",
            label: t("resourcesTemplatesSection"),
            onSelect: () => {
              if (firstHubTemplate) {
                onSelectHubTemplate(firstHubTemplate);
                return;
              }
              onSelectHub();
            },
          },
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.hubSkills,
            badge: badgeCount(hub?.skills.length),
            groupId: WorkspaceContextSectionIds.hubSkills,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.skills),
            id: "skills",
            label: t("resourcesSkillsLabel"),
            onSelect: () => {
              if (firstHubSkill) {
                onSelectHubSkill(firstHubSkill);
                return;
              }
              onSelectHub();
            },
          },
          {
            active: activeContextSectionId === WorkspaceContextSectionIds.models,
            badge: badgeCount(modelProviders?.providers.length),
            groupId: WorkspaceContextSectionIds.models,
            icon: navigationIcon(WORKSPACE_NAVIGATION_ICONS.models),
            id: "models",
            label: t("resourcesModelProvidersSection"),
            onSelect: () => {
              if (firstModelProvider && onSelectModelProvider) {
                onSelectModelProvider(firstModelProvider);
                return;
              }
              onSelectHub();
            },
          },
        ],
      },
    ],
    [
      activeContextSectionId,
      activePane.type,
      activeTaskBoardView,
      currentUser,
      firstHubSkill,
      firstHubTemplate,
      firstModelProvider,
      firstNotificationAgent,
      firstTeam,
      firstWorkerAgent,
      hub?.templates.length,
      hub?.skills.length,
      modelProviders?.providers.length,
      onSelectAgent,
      onSelectComputer,
      onSelectHub,
      onSelectHubSkill,
      onSelectHubTemplate,
      onSelectHuman,
      onSelectModelProvider,
      onSelectNotificationSection,
      onSelectTask,
      onSelectTaskBoardView,
      onSelectTeam,
      onSelectTeamSection,
      onWorkspaceTabChange,
      t,
    ],
  );
  const contextBadgeCount = contextBadgeCountForSection({
    activeContextSectionId,
    currentUser,
    hub,
    modelProviders,
    notificationAgentItems,
    scheduledTaskCount,
    taskCount,
    teams,
    workerAgentItems,
  });
  const contextCreateAction = contextCreateActionForSection({
    activeContextSectionId,
    onCreateAgent,
    onCreateNotificationParticipant,
    onCreateRoom,
    onCreateModelProvider,
    onOpenCreateTeam,
    onOpenCreateTask,
    onOpenCreateScheduledTask,
    setSkillUploadOpen,
    t,
    activeTaskBoardView,
  });
  const contextTitle = contextTitleForSection(activeContextSectionId, currentWorkspaceLabel, t);

  function activateNavigationItem(item: PrimaryNavigationItem) {
    setActiveContextSectionId(item.groupId);
    item.onSelect();
    if (collapsedWorkspaceGroups[item.groupId]) {
      onToggleWorkspaceGroup(item.groupId);
    }
    scheduleSectionScroll(() => contextNavRef.current, item.groupId);
  }

  return (
    <div className={classNames(styles.slot, !hasContextSidebar && styles.noContextSidebar)}>
      <aside
        className={classNames(styles.primarySidebar, isSidebarCollapsed && styles.primarySidebarCollapsed)}
        aria-label="Sidebar navigation"
      >
        <div className={styles.primaryNavigationWrap}>
          <div className={classNames(styles.primaryHeader, isSidebarCollapsed && styles.primaryHeaderCollapsed)}>
            {isSidebarCollapsed ? (
              <span className={styles.logoMarkSlot}>
                <button
                  type="button"
                  className={styles.logoMarkButton}
                  aria-label={t("expandSidebar")}
                  title={t("expandSidebar")}
                  onClick={onExpandSidebar}
                >
                  <span className={styles.logoMarkExpandIcon} aria-hidden="true">
                    <PanelLeftOpen size={20} strokeWidth={1.75} />
                  </span>
                </button>
                <span className={styles.logoMarkVisual} aria-hidden="true">
                  <LogoMark />
                </span>
              </span>
            ) : (
              <>
                <LogoWordmark />
                <SidebarRailControlButton
                  className={styles.primaryHeaderControl}
                  label={t("collapseSidebar")}
                  markClassName={styles.primaryHeaderControlMark}
                  mode="collapse"
                  onClick={onCollapseSidebar}
                />
              </>
            )}
          </div>
          <WorkspacePrimaryNavigation
            collapsed={isSidebarCollapsed}
            sections={primaryNavigationSections}
            onActivate={activateNavigationItem}
          />
        </div>
        <div className={classNames(styles.primaryFooter, isSidebarCollapsed && styles.primaryFooterCollapsed)}>
          <SidebarUserButton
            active={activePane.type === WorkspacePaneTypes.settings}
            presentation={isSidebarCollapsed ? "icon" : "row"}
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
            onOpenSettings={onOpenSettings}
            authStatus={authStatus}
            authBusy={authBusy}
            authPending={authPending}
            authError={authError}
            onLogin={onLogin}
            onLogout={onLogout}
            t={t}
          />
        </div>
      </aside>
      {hasContextSidebar ? (
        <aside className={styles.mainColumn}>
          <div className={styles.contextHeader}>
            <div className={styles.contextTitleRow}>
              <div className={styles.contextTitleWrap}>
                <strong className={classNames(styles.contextTitle, "truncate")}>{contextTitle}</strong>
                {contextBadgeCount !== null ? <span className={styles.contextBadge}>{contextBadgeCount}</span> : null}
              </div>
              {contextCreateAction ? (
                <button
                  type="button"
                  className={styles.contextAction}
                  aria-label={contextCreateAction.label}
                  data-tooltip={contextCreateAction.label}
                  onClick={contextCreateAction.onClick}
                >
                  <Plus size={16} strokeWidth={2.2} aria-hidden="true" />
                </button>
              ) : null}
            </div>
            <label className={styles.contextSearch}>
              <Search size={20} strokeWidth={1.8} aria-hidden="true" />
              <input
                className={styles.contextSearchInput}
                type="search"
                value={contextQuery}
                placeholder={t("workspaceSearchPlaceholder")}
                aria-label={t("workspaceSearchPlaceholder")}
                onChange={(event) => setContextQuery(event.currentTarget.value)}
              />
            </label>
          </div>
          <nav ref={contextNavRef} className={styles.contextNav} aria-label="Workspace">
            <WorkspaceTabPanels
              contextQuery={contextQuery}
              contextSectionId={activeContextSectionId}
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
              onOpenCreateScheduledTask={onOpenCreateScheduledTask}
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
              skillUploadOpen={skillUploadOpen}
              onSkillUploadOpenChange={setSkillUploadOpen}
            />
          </nav>
        </aside>
      ) : null}
    </div>
  );
}

function navigationIcon(Icon: SidebarNavigationIcon) {
  return <Icon size={20} />;
}

function badgeCount(count: number | null | undefined) {
  return typeof count === "number" ? count : undefined;
}

function contextSectionIdForPane({
  activePane,
  notificationAgentIds,
  workspaceTab,
}: {
  activePane: WorkspaceSidebarProps["activePane"];
  notificationAgentIds: ReadonlySet<string>;
  workspaceTab: WorkspaceSidebarProps["workspaceTab"];
}): WorkspaceContextSectionId | null {
  if (activePane.type === WorkspacePaneTypes.settings) {
    return null;
  }
  if (activePane.type === WorkspacePaneTypes.task) {
    return WorkspaceContextSectionIds.tasks;
  }
  if (activePane.type === WorkspacePaneTypes.modelProvider) {
    return WorkspaceContextSectionIds.models;
  }
  if (activePane.type === WorkspacePaneTypes.hub) {
    if (activePane.resourceType === "skill") {
      return WorkspaceContextSectionIds.hubSkills;
    }
    if (activePane.resourceType === "template") {
      return WorkspaceContextSectionIds.hubTemplates;
    }
    return null;
  }
  if (activePane.type === WorkspacePaneTypes.agent) {
    return notificationAgentIds.has(activePane.id || "")
      ? WorkspaceContextSectionIds.notifications
      : WorkspaceContextSectionIds.agents;
  }
  if (activePane.type === WorkspacePaneTypes.notifications) {
    return WorkspaceContextSectionIds.notifications;
  }
  if (activePane.type === WorkspacePaneTypes.human) {
    return WorkspaceContextSectionIds.humans;
  }
  if (activePane.type === WorkspacePaneTypes.team) {
    return WorkspaceContextSectionIds.teams;
  }
  if (activePane.type === WorkspacePaneTypes.computer) {
    return WorkspaceContextSectionIds.computers;
  }
  if (workspaceTab === WorkspaceTabs.threads || workspaceTab === WorkspaceTabs.messages) {
    return WorkspaceContextSectionIds.messages;
  }
  return defaultContextSectionIdForTab(workspaceTab);
}

function defaultContextSectionIdForTab(workspaceTab: WorkspaceSidebarProps["workspaceTab"]): WorkspaceContextSectionId {
  if (workspaceTab === WorkspaceTabs.agents) {
    return WorkspaceContextSectionIds.agents;
  }
  if (workspaceTab === WorkspaceTabs.hub) {
    return WorkspaceContextSectionIds.hubTemplates;
  }
  if (workspaceTab === WorkspaceTabs.tasks) {
    return WorkspaceContextSectionIds.tasks;
  }
  return WorkspaceContextSectionIds.messages;
}

function contextTitleForSection(sectionId: WorkspaceContextSectionId, fallback: string, t: WorkspaceSidebarProps["t"]) {
  if (sectionId === WorkspaceContextSectionIds.messages) {
    return t("messagesTab");
  }
  if (sectionId === WorkspaceContextSectionIds.agents) {
    return t("agentOverview");
  }
  if (sectionId === WorkspaceContextSectionIds.humans) {
    return t("humanDetailTitle");
  }
  if (sectionId === WorkspaceContextSectionIds.computers) {
    return t("computerOverview");
  }
  if (sectionId === WorkspaceContextSectionIds.notifications) {
    return t("notificationsSection");
  }
  if (sectionId === WorkspaceContextSectionIds.teams) {
    return t("teamsSection");
  }
  if (sectionId === WorkspaceContextSectionIds.hubTemplates) {
    return t("resourcesTemplatesSection");
  }
  if (sectionId === WorkspaceContextSectionIds.hubSkills) {
    return t("resourcesSkillsLabel");
  }
  if (sectionId === WorkspaceContextSectionIds.models) {
    return t("resourcesModelProvidersSection");
  }
  if (sectionId === WorkspaceContextSectionIds.tasks) {
    return t("tasksOverview");
  }
  return fallback;
}

function contextBadgeCountForSection({
  activeContextSectionId,
  currentUser,
  hub,
  modelProviders,
  notificationAgentItems,
  scheduledTaskCount = 0,
  taskCount,
  teams,
  workerAgentItems,
}: {
  activeContextSectionId: WorkspaceContextSectionId;
  currentUser: unknown;
  hub: WorkspaceSidebarProps["hub"];
  modelProviders: WorkspaceSidebarProps["modelProviders"];
  notificationAgentItems: WorkspaceSidebarProps["notificationAgentItems"];
  scheduledTaskCount?: number;
  taskCount: number;
  teams: WorkspaceSidebarProps["teams"];
  workerAgentItems: WorkspaceSidebarProps["workerAgentItems"];
}): number | null {
  if (activeContextSectionId === WorkspaceContextSectionIds.agents) {
    return workerAgentItems.length;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.humans) {
    return currentUser ? 1 : 0;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.computers) {
    return 1;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.notifications) {
    return notificationAgentItems.length;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.teams) {
    return teams.length;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.hubTemplates) {
    return hub?.templates.length ?? 0;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.hubSkills) {
    return hub?.skills.length ?? 0;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.models) {
    return modelProviders?.providers.length ?? 0;
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.tasks) {
    return taskCount + scheduledTaskCount;
  }
  return null;
}

function contextCreateActionForSection({
  activeTaskBoardView,
  activeContextSectionId,
  onCreateAgent,
  onCreateNotificationParticipant,
  onCreateRoom,
  onCreateModelProvider,
  onOpenCreateTeam,
  onOpenCreateTask,
  onOpenCreateScheduledTask,
  setSkillUploadOpen,
  t,
}: Pick<
  WorkspaceSidebarProps,
  | "onCreateAgent"
  | "onCreateModelProvider"
  | "onCreateNotificationParticipant"
  | "onCreateRoom"
  | "onOpenCreateScheduledTask"
  | "onOpenCreateTask"
  | "onOpenCreateTeam"
  | "t"
> & {
  activeTaskBoardView?: "tasks" | "scheduled";
  activeContextSectionId: WorkspaceContextSectionId;
  setSkillUploadOpen: (open: boolean) => void;
}) {
  if (activeContextSectionId === WorkspaceContextSectionIds.messages) {
    return {
      label: t("createRoom"),
      onClick: onCreateRoom,
    };
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.agents) {
    return {
      label: t("createAgent"),
      onClick: onCreateAgent,
    };
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.notifications) {
    return {
      label: t("createNotificationBot"),
      onClick: onCreateNotificationParticipant,
    };
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.teams) {
    return {
      label: t("teamCreate"),
      onClick: onOpenCreateTeam,
    };
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.tasks) {
    if (activeTaskBoardView === "scheduled" && onOpenCreateScheduledTask) {
      return {
        label: t("scheduledTaskCreate"),
        onClick: onOpenCreateScheduledTask,
      };
    }
    return {
      label: t("taskCreate"),
      onClick: onOpenCreateTask,
    };
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.hubSkills) {
    return {
      label: t("resourcesSkillUpload"),
      onClick: () => setSkillUploadOpen(true),
    };
  }
  if (activeContextSectionId === WorkspaceContextSectionIds.models && onCreateModelProvider) {
    return {
      label: t("modelProviderAdd"),
      onClick: onCreateModelProvider,
    };
  }
  return null;
}

function scheduleSectionScroll(getContainer: () => HTMLElement | null, groupId: string) {
  const scroll = () => scrollWorkspaceSection(getContainer(), groupId);
  if (typeof window.requestAnimationFrame === "function") {
    window.requestAnimationFrame(scroll);
    return;
  }
  window.setTimeout(scroll, 0);
}

function scrollWorkspaceSection(container: HTMLElement | null, groupId: string) {
  const groups = container?.querySelectorAll<HTMLElement>("[data-workspace-section]") ?? [];
  const target = Array.from(groups).find((group) => group.dataset.workspaceSection === groupId);
  target?.scrollIntoView({ block: "nearest" });
}
