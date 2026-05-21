export function SidebarHeader({ t, currentWorkspaceLabel, runningAgentCount, agentCount }) {
  return (
    <div className="sidebar-header workspace-header">
      <div className="workspace-signal-panel" aria-label={currentWorkspaceLabel}>
        <div className="workspace-signal-copy">
          <span>{currentWorkspaceLabel}</span>
          <strong>
            {runningAgentCount}/{agentCount} {t("activeNow")}
          </strong>
        </div>
        <div className="workspace-signal-meter" aria-hidden="true">
          <span></span>
          <span></span>
          <span></span>
        </div>
      </div>
    </div>
  );
}
