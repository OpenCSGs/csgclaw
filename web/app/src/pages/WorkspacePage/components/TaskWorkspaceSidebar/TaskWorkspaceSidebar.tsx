import { Plus, Bot, CalendarClock, PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { TaskConversationList } from "@/components/business/TaskConversationList";
import { resolveAgentChannelUserID, agentRuntimeOptions } from "@/models/agents";
import { resolveConversationUser, agentMatchesUser } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";
import { Tooltip } from "@/components/ui";
import type { WorkspaceSidebarProps } from "@/hooks/workspace/contracts/sidebar";
import type { TaskEntryController } from "@/hooks/workspace/useTaskEntry";
import type { IMConversation } from "@/models/conversations";
import styles from "./TaskWorkspaceSidebar.module.css";
import { WorkspaceUserMenu } from "../WorkspaceUserMenu";
import { LogoMark, LogoWordmark } from "../WorkspaceSidebar/WorkspaceSidebarBrand";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";

export function TaskWorkspaceSidebar(props: WorkspaceSidebarProps & { entry: TaskEntryController }) {
  const navigate = useNavigate();
  const { openSettings, openProfile } = useWorkspaceDialogs();
  const { t, entry, isSidebarCollapsed, currentUserID, usersById } = props;
  const directories = props.workerAgentItems.reduce<Array<{ path: string; agentIDs: string[] }>>((items, agent) => {
    const path = agentRuntimeOptions(agent).local_workspace_dir;
    if (typeof path !== "string" || !path) return items;
    let item = items.find((value) => value.path === path);
    if (!item) {
      item = { path, agentIDs: [] };
      items.push(item);
    }
    item.agentIDs.push(resolveAgentChannelUserID(agent), agent.id || "");
    return items;
  }, []);
  for (const room of props.directMessages) {
    const path = room.configuration?.workspace_dir;
    if (path && !directories.some((item) => item.path === path)) directories.push({ path, agentIDs: [] });
  }
  function agentForRoom(room: IMConversation) {
    const other = resolveConversationUser(room, currentUserID, usersById);
    return props.workerAgentItems.find(
      (agent) => agentMatchesUser(agent, other) || room.members.includes(resolveAgentChannelUserID(agent)),
    );
  }
  const links = [
    {
      label: t("taskEntryNew"),
      icon: Plus,
      active: props.activePane.type === WorkspacePaneTypes.conversation && !props.activePane.id,
      action: () => navigate("/new"),
    },
    {
      label: t("agentsTab"),
      icon: Bot,
      active: props.activePane.type === WorkspacePaneTypes.agent || props.activePane.type === WorkspacePaneTypes.team,
      action: () => navigate("/agents"),
    },
    {
      label: t("scheduledTasksTab"),
      icon: CalendarClock,
      active: props.activePane.type === WorkspacePaneTypes.task,
      action: () => {
        props.onSelectTaskBoardView?.("scheduled");
        props.onSelectTask();
      },
    },
  ];
  return (
    <aside
      className={`${styles.sidebar} ${isSidebarCollapsed ? styles.collapsed : ""}`}
      aria-label={t("taskEntryNavigation")}
    >
      <header>
        {!isSidebarCollapsed ? (
          <span className={styles.brand}>
            <LogoWordmark />
          </span>
        ) : null}
        <Tooltip content={t(isSidebarCollapsed ? "expandSidebar" : "collapseSidebar")}>
          <button
            type="button"
            className={styles.headerControl}
            aria-label={t(isSidebarCollapsed ? "expandSidebar" : "collapseSidebar")}
            onClick={isSidebarCollapsed ? props.onExpandSidebar : props.onCollapseSidebar}
          >
            {isSidebarCollapsed ? (
              <>
                <span className={styles.logoMark} aria-hidden="true">
                  <LogoMark />
                </span>
                <PanelLeftOpen className={styles.expandIcon} size={20} aria-hidden="true" />
              </>
            ) : (
              <PanelLeftClose size={20} aria-hidden="true" />
            )}
          </button>
        </Tooltip>
      </header>
      <nav>
        {links.map(({ label, icon: Icon, active, action }, index) => (
          <Tooltip key={label} content={isSidebarCollapsed ? label : null}>
            <button
              type="button"
              aria-label={label}
              aria-current={active ? "page" : undefined}
              className={`${index === 0 ? styles.newTask : ""} ${active ? styles.active : ""}`}
              onClick={action}
            >
              <Icon size={20} aria-hidden="true" />
              {isSidebarCollapsed ? null : (
                <>
                  <span>{label}</span>
                  {index > 0 ? (
                    <small className={styles.badge}>
                      {index === 1 ? props.workerAgentItems.length : props.scheduledTaskCount || 0}
                    </small>
                  ) : null}
                </>
              )}
            </button>
          </Tooltip>
        ))}
      </nav>
      {!isSidebarCollapsed ? (
        <div className={styles.history}>
          <TaskConversationList
            compact
            directories={directories}
            entry={entry}
            activeID={props.activePane.id}
            currentUserID={currentUserID}
            usersById={usersById}
            t={t}
            onSelect={props.onSelectConversation}
            onArchiveCurrent={() => navigate("/new")}
            onNewWith={(room) => {
              const agent = agentForRoom(room);
              if (agent?.id)
                navigate(`/new?agent=${encodeURIComponent(agent.id)}`, {
                  state: { configuration: room.configuration },
                });
            }}
            onEditAgent={(room) => {
              const agent = agentForRoom(room);
              if (agent) props.onSelectAgent(agent);
            }}
          />
        </div>
      ) : null}
      <footer>
        <WorkspaceUserMenu
          currentUser={usersById.get(currentUserID)}
          openCSG={{
            authenticated: props.authStatus.authenticated === true,
            expired: props.authExpired,
            baseURL: props.authStatus.base_url,
          }}
          theme={props.theme}
          locale={props.locale}
          t={t}
          presentation={isSidebarCollapsed ? "icon" : "row"}
          onThemeChange={props.onThemeChange}
          onLocaleChange={props.onLocaleChange}
          onOpenSettings={props.onOpenSettings}
          onOpenProfile={() => openProfile(currentUserID)}
          onOpenCSGLogin={() => openSettings("account")}
          onOpenCSGLogout={() => void props.onLogout()}
        />
      </footer>
    </aside>
  );
}
