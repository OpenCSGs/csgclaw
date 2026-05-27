export function SidebarHeader({ t, currentWorkspaceLabel, runningAgentCount, agentCount }) {
  return (
    <div className="sidebar-header workspace-header">
      <div className="workspace-presence-panel" aria-label={currentWorkspaceLabel}>
        <span className="workspace-presence-dot" aria-hidden="true"></span>
        <div className="workspace-presence-copy">
          <span>{t("localAgentConsole")}</span>
          <strong>
            {runningAgentCount}/{agentCount} {t("activeNow")}
          </strong>
        </div>
      </div>
    </div>
  );
}
