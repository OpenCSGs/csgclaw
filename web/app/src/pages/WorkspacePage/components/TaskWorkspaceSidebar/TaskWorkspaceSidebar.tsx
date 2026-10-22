import { Plus, PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { TaskConversationList } from "@/components/business/TaskConversationList";
import { resolveAgentChannelUserID, agentRuntimeOptions } from "@/models/agents";
import { resolveConversationUser, agentMatchesUser } from "@/models/conversations";
import { pathForPane, WorkspacePaneTypes } from "@/models/routing";
import { Tooltip } from "@/components/ui";
import type { WorkspaceSidebarProps } from "@/hooks/workspace/contracts/sidebar";
import type { TaskEntryController } from "@/hooks/workspace/useTaskEntry";
import type { IMConversation } from "@/models/conversations";
import styles from "./TaskWorkspaceSidebar.module.css";
import { WorkspaceUserMenu } from "../WorkspaceUserMenu";
import { LogoMark, LogoWordmark } from "../WorkspaceSidebar/WorkspaceSidebarBrand";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";

function AgentNavigationIcon({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M15.3332 7.33304V10.333M0.666504 7.33304V10.333M9.99984 8.66634V9.33301M5.99984 8.66634V9.33301M7.99984 4.33301C7.28873 4.33301 6.57762 4.33301 5.8665 4.33301C4.7464 4.33301 4.18635 4.33301 3.75852 4.55099C3.3822 4.74274 3.07624 5.0487 2.88449 5.42503C2.6665 5.85285 2.6665 6.41277 2.6665 7.5326C2.6665 8.50209 2.6665 9.53297 2.6665 10.4674C2.6665 11.5868 2.6665 12.1465 2.88449 12.5743C3.07624 12.9506 3.3822 13.2566 3.75852 13.4484C4.18635 13.6663 4.7464 13.6663 5.86651 13.6663C7.28873 13.6663 8.71095 13.6663 10.1332 13.6663C11.2533 13.6663 11.8133 13.6663 12.2412 13.4484C12.6175 13.2566 12.9234 12.9506 13.1152 12.5743C13.3332 12.1465 13.3332 11.5864 13.3332 10.4663V6.99978C13.3332 6.3797 13.3332 6.06966 13.265 5.81529C13.0801 5.12517 12.541 4.58613 11.8509 4.40118C11.5965 4.33301 11.2865 4.33301 10.6664 4.33301C9.77754 4.33301 8.88869 4.33301 7.99984 4.33301ZM7.99984 4.33301V3.16634V1.99967M7.99984 1.99967L8.0415 1.95801M7.99984 1.99967L7.95817 1.95801M7.99984 1.99967V1.74967M8.1665 1.83301H7.83317M8.1665 1.49967L8.24984 1.58301C8.29586 1.62903 8.29586 1.70365 8.24984 1.74967L8.1665 1.83301M8.1665 1.83301L7.74984 1.74967M8.1665 1.83301L8.12484 1.87467M8.1665 1.49967C8.07446 1.40763 7.92522 1.40763 7.83317 1.49967L7.74984 1.58301C7.70381 1.62903 7.70381 1.70365 7.74984 1.74967L7.83317 1.83301M7.83317 1.83301L8.24984 1.74967M7.83317 1.83301L7.87484 1.87467M7.74984 1.74967L7.83317 1.49967M7.74984 1.74967H7.99984M8.24984 1.74967L8.1665 1.49967M8.24984 1.74967H7.99984M8.1665 1.49967H7.83317M7.9165 1.91634H8.08317M7.9165 1.91634L7.87484 1.87467M7.9165 1.91634L7.95817 1.95801M8.08317 1.91634L8.12484 1.87467M8.08317 1.91634L8.0415 1.95801M7.87484 1.87467H8.12484M8.0415 1.95801H7.95817"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function ScheduledTaskNavigationIcon({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M5.77794 5.77745L8.00016 7.99967M3.28612 3.28563C2.07969 4.49206 1.3335 6.15873 1.3335 7.99967C1.3335 11.6816 4.31826 14.6663 8.00016 14.6663C11.6821 14.6663 14.6668 11.6816 14.6668 7.99967C14.6668 4.31778 11.6821 1.33301 8.00016 1.33301V3.55523"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

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
  function selectDestination(action: () => void) {
    action();
    if (!isSidebarCollapsed && window.innerWidth <= 720) props.onCollapseSidebar();
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
      icon: AgentNavigationIcon,
      active:
        props.activePane.type === WorkspacePaneTypes.agent ||
        props.activePane.type === WorkspacePaneTypes.team ||
        props.activePane.type === WorkspacePaneTypes.notifications,
      action: () => navigate(pathForPane({ type: WorkspacePaneTypes.agent })),
    },
    {
      label: t("scheduledTasksTab"),
      icon: ScheduledTaskNavigationIcon,
      active: props.activePane.type === WorkspacePaneTypes.task && props.activeTaskBoardView === "scheduled",
      action: () => {
        props.onSelectTaskBoardView?.("scheduled");
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
              onClick={() => selectDestination(action)}
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
            activeID={props.activePane.type === WorkspacePaneTypes.conversation ? props.activePane.id : undefined}
            currentUserID={currentUserID}
            usersById={usersById}
            t={t}
            onSelect={(id, match) =>
              selectDestination(() => {
                if (match?.id && props.onSelectSearchMatch) props.onSelectSearchMatch(id, match);
                else props.onSelectConversation(id);
              })
            }
            onArchiveCurrent={() => navigate("/new")}
            onNewWith={(room) => {
              const agent = agentForRoom(room);
              if (agent?.id) {
                const agentID = agent.id;
                selectDestination(() =>
                  navigate(`/new?agent=${encodeURIComponent(agentID)}`, {
                    state: { configuration: room.configuration },
                  }),
                );
              }
            }}
            onEditAgent={(room) => {
              const agent = agentForRoom(room);
              if (agent) selectDestination(() => props.onSelectAgent(agent));
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
          onOpenSettings={() => selectDestination(props.onOpenSettings)}
          onOpenProfile={() => selectDestination(() => openProfile(currentUserID))}
          onOpenCSGLogin={() => selectDestination(() => openSettings("account"))}
          onOpenCSGLogout={() => void props.onLogout()}
        />
      </footer>
    </aside>
  );
}
