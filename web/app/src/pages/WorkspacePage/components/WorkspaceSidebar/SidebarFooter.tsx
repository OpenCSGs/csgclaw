import { formatSidebarVersionLabel } from "@/models/upgradeStatus";

export function SidebarFooter({
  appVersion,
  upgradeStatus,
  upgradeBusy,
  upgradePhase,
  upgradeError,
  onOpenUpgrade,
  t,
}) {
  return (
    <div className="sidebar-footer">
      <div className="sidebar-footer-row">
        <span className="sidebar-version-label">{formatSidebarVersionLabel(appVersion)}</span>
        {upgradeStatus?.update_available ||
        upgradeBusy ||
        upgradeStatus?.upgrading ||
        upgradePhase === "done" ||
        upgradePhase === "error" ? (
          <button
            type="button"
            className={`sidebar-upgrade-button ${upgradeBusy || upgradeStatus?.upgrading ? "is-running" : ""} ${upgradePhase === "done" ? "is-done" : ""}`}
            onClick={onOpenUpgrade}
          >
            <span className="sidebar-upgrade-dot" aria-hidden="true"></span>
            <span>
              {upgradePhase === "done"
                ? t("upgradeRefresh")
                : upgradeBusy || upgradeStatus?.upgrading
                  ? t("upgradeBackground")
                  : t("upgradeAction")}
            </span>
          </button>
        ) : null}
      </div>
      {upgradeError ? <div className="sidebar-footer-error">{upgradeError}</div> : null}
    </div>
  );
}
