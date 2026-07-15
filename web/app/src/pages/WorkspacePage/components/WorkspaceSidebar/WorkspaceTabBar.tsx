import { Button, Tooltip } from "@/components/ui";
import { HubIcon, RoomsIcon, TaskIcon, UsersIcon } from "@/components/ui/Icons";
import { WorkspaceTabs } from "@/models/routing";
import type { WorkspaceSidebarProps } from "./types";

type WorkspaceTabBarProps = Pick<
  WorkspaceSidebarProps,
  "onSelectHub" | "onWorkspaceTabChange" | "roomCount" | "showHubNewBadge" | "t" | "taskCount" | "workspaceTab"
> & {
  agentCount: number;
  variant?: "default" | "rail";
};

export function WorkspaceTabBar({
  workspaceTab,
  onWorkspaceTabChange,
  taskCount = 0,
  roomCount,
  agentCount,
  onSelectHub,
  showHubNewBadge,
  t,
  variant = "default",
}: WorkspaceTabBarProps) {
  const rail = variant === "rail";
  return (
    <div
      className={`workspace-tabbar ${rail ? "workspace-tabbar-rail" : ""}`}
      role="tablist"
      aria-label="Workspace sections"
      aria-orientation={rail ? "vertical" : undefined}
    >
      <Tooltip content={t("messagesTab")}>
        <Button
          className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
          active={workspaceTab === WorkspaceTabs.messages}
          role="tab"
          aria-selected={workspaceTab === WorkspaceTabs.messages}
          aria-label={t("messagesTab")}
          onClick={() => onWorkspaceTabChange(WorkspaceTabs.messages)}
        >
          <span className="workspace-tab-icon" aria-hidden="true">
            <RoomsIcon />
          </span>
          {!rail ? (
            <span className="workspace-tab-copy">
              <strong>{t("messagesTab")}</strong>
              <small>{roomCount}</small>
            </span>
          ) : null}
        </Button>
      </Tooltip>
      <Tooltip content={t("agentsTab")}>
        <Button
          className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
          active={workspaceTab === WorkspaceTabs.agents}
          role="tab"
          aria-selected={workspaceTab === WorkspaceTabs.agents}
          aria-label={t("agentsTab")}
          onClick={() => onWorkspaceTabChange(WorkspaceTabs.agents)}
        >
          <span className="workspace-tab-icon" aria-hidden="true">
            <UsersIcon />
          </span>
          {!rail ? (
            <span className="workspace-tab-copy">
              <strong>{t("agentsTab")}</strong>
              <small>{agentCount}</small>
            </span>
          ) : null}
        </Button>
      </Tooltip>
      <Tooltip content={t("tasksTab")}>
        <Button
          className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
          active={workspaceTab === WorkspaceTabs.tasks}
          role="tab"
          aria-selected={workspaceTab === WorkspaceTabs.tasks}
          aria-label={t("tasksTab")}
          onClick={() => onWorkspaceTabChange(WorkspaceTabs.tasks)}
        >
          <span className="workspace-tab-icon" aria-hidden="true">
            <TaskIcon />
          </span>
          {!rail ? (
            <span className="workspace-tab-copy">
              <strong>{t("tasksTab")}</strong>
              <small>{taskCount}</small>
            </span>
          ) : null}
        </Button>
      </Tooltip>
      <Tooltip content={t("resourcesTab")}>
        <Button
          className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
          active={workspaceTab === WorkspaceTabs.hub}
          role="tab"
          aria-selected={workspaceTab === WorkspaceTabs.hub}
          aria-label={t("resourcesTab")}
          onClick={() => onSelectHub()}
        >
          <span className="workspace-tab-icon" aria-hidden="true">
            <HubIcon />
          </span>
          {rail && showHubNewBadge ? (
            <span className="workspace-tab-rail-new" aria-hidden="true"></span>
          ) : !rail ? (
            <span className="workspace-tab-copy">
              <strong>{t("resourcesTab")}</strong>
              {showHubNewBadge ? <span className="workspace-tab-badge">{t("newBadge")}</span> : null}
            </span>
          ) : null}
        </Button>
      </Tooltip>
    </div>
  );
}
