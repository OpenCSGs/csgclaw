import { Button } from "@/components/ui";
import { HubIcon, RoomsIcon, UsersIcon } from "@/components/ui/Icons";
import { WorkspaceTabs } from "@/models/routing";

export function WorkspaceTabBar({
  workspaceTab,
  onWorkspaceTabChange,
  roomCount,
  threadCount,
  agentCount,
  onSelectHub,
  t,
  variant = "default",
}) {
  const rail = variant === "rail";
  return (
    <div
      className={`workspace-tabbar ${rail ? "workspace-tabbar-rail" : ""}`}
      role="tablist"
      aria-label="Workspace sections"
      aria-orientation={rail ? "vertical" : undefined}
    >
      <Button
        className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
        active={workspaceTab === WorkspaceTabs.messages}
        role="tab"
        aria-selected={workspaceTab === WorkspaceTabs.messages}
        aria-label={t("messagesTab")}
        title={t("messagesTab")}
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
      <Button
        className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
        active={workspaceTab === WorkspaceTabs.threads}
        role="tab"
        aria-selected={workspaceTab === WorkspaceTabs.threads}
        aria-label={t("threadsTab")}
        title={t("threadsTab")}
        onClick={() => onWorkspaceTabChange(WorkspaceTabs.threads)}
      >
        <span className="workspace-tab-icon" aria-hidden="true">
          <RoomsIcon />
        </span>
        {!rail ? (
          <span className="workspace-tab-copy">
            <strong>{t("threadsTab")}</strong>
            <small>{threadCount}</small>
          </span>
        ) : null}
      </Button>
      <Button
        className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
        active={workspaceTab === WorkspaceTabs.agents}
        role="tab"
        aria-selected={workspaceTab === WorkspaceTabs.agents}
        aria-label={t("agentsTab")}
        title={t("agentsTab")}
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
      <Button
        className={`workspace-tab ${rail ? "workspace-tab-rail" : ""}`}
        active={workspaceTab === WorkspaceTabs.hub}
        role="tab"
        aria-selected={workspaceTab === WorkspaceTabs.hub}
        aria-label={t("hubTab")}
        title={t("hubTab")}
        onClick={() => onSelectHub()}
      >
        <span className="workspace-tab-icon" aria-hidden="true">
          <HubIcon />
        </span>
        {rail ? (
          <span className="workspace-tab-rail-new" aria-hidden="true"></span>
        ) : (
          <span className="workspace-tab-copy">
            <strong>{t("hubTab")}</strong>
            <span className="workspace-tab-badge">{t("newBadge")}</span>
          </span>
        )}
      </Button>
    </div>
  );
}
