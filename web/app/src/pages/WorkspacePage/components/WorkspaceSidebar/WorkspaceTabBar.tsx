// @ts-nocheck
import { WORKSPACE_TAB_AGENTS, WORKSPACE_TAB_HUB, WORKSPACE_TAB_MESSAGES } from "@/bootstrap/constants";
import { Button } from "@/components/ui";
import { HubIcon, RoomsIcon, UsersIcon } from "@/components/ui/Icons";

export function WorkspaceTabBar({ workspaceTab, onWorkspaceTabChange, roomCount, agentCount, onSelectHub, t }) {
  return (
    <div className="workspace-tabbar" role="tablist" aria-label="Workspace sections">
      <Button
        className="workspace-tab"
        active={workspaceTab === WORKSPACE_TAB_MESSAGES}
        role="tab"
        aria-selected={workspaceTab === WORKSPACE_TAB_MESSAGES}
        aria-label={t("messagesTab")}
        title={t("messagesTab")}
        onClick={() => onWorkspaceTabChange(WORKSPACE_TAB_MESSAGES)}
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
        active={workspaceTab === WORKSPACE_TAB_AGENTS}
        role="tab"
        aria-selected={workspaceTab === WORKSPACE_TAB_AGENTS}
        aria-label={t("agentsTab")}
        title={t("agentsTab")}
        onClick={() => onWorkspaceTabChange(WORKSPACE_TAB_AGENTS)}
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
        active={workspaceTab === WORKSPACE_TAB_HUB}
        role="tab"
        aria-selected={workspaceTab === WORKSPACE_TAB_HUB}
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
