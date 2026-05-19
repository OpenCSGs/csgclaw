import { Button } from "@/components/ui";
import { HubIcon, RoomsIcon, UsersIcon } from "@/components/ui/Icons";
import { WorkspaceTabs } from "@/models/routing";

export function WorkspaceTabBar({ workspaceTab, onWorkspaceTabChange, roomCount, agentCount, onSelectHub, t }) {
  return (
    <div className="workspace-tabbar" role="tablist" aria-label="Workspace sections">
      <Button
        className="workspace-tab"
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
        <span className="workspace-tab-copy">
          <strong>{t("messagesTab")}</strong>
          <small>{roomCount}</small>
        </span>
      </Button>
      <Button
        className="workspace-tab"
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
        <span className="workspace-tab-copy">
          <strong>{t("agentsTab")}</strong>
          <small>{agentCount}</small>
        </span>
      </Button>
      <Button
        className="workspace-tab"
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
        <span className="workspace-tab-copy">
          <strong>{t("hubTab")}</strong>
          <span className="workspace-tab-badge">{t("newBadge")}</span>
        </span>
      </Button>
    </div>
  );
}
