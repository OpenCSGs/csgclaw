import { Button } from "@/components/ui";
import { formatSidebarVersionLabel, upgradeStatusLabel } from "@/models/upgradeStatus";

export function UpgradeModal({
  t,
  upgradeStatus,
  appVersion,
  upgradePhase,
  upgradeBusy,
  upgradeError,
  onClose,
  onApply,
}) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card upgrade-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("upgradeTitle")}</div>
            <div className="modal-subtitle">{t("upgradeSubtitle")}</div>
          </div>
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
        <div className="upgrade-summary">
          <div className="upgrade-summary-row">
            <span>{t("upgradeCurrentVersion")}</span>
            <strong>{formatSidebarVersionLabel(upgradeStatus?.current_version || appVersion || "dev")}</strong>
          </div>
          <div className="upgrade-summary-row">
            <span>{t("upgradeLatestVersion")}</span>
            <strong>
              {upgradeStatus?.latest_version
                ? formatSidebarVersionLabel(upgradeStatus.latest_version)
                : t("upgradeNoLatest")}
            </strong>
          </div>
          <div className="upgrade-summary-row">
            <span>{t("upgradeStatus")}</span>
            <strong>{upgradeStatusLabel(upgradePhase, t)}</strong>
          </div>
        </div>
        <div className={`upgrade-status-card ${upgradePhase}`}>
          <span className="upgrade-status-dot" aria-hidden="true"></span>
          <p>
            {upgradePhase === "manual_restart" || upgradeStatus?.manual_restart_required
              ? t("upgradeManualRestartBody")
              : upgradePhase === "done"
                ? t("upgradeDoneBody")
                : upgradePhase === "restarting" ||
                    upgradePhase === "starting" ||
                    upgradeBusy ||
                    upgradeStatus?.upgrading
                  ? t("upgradeContinueUsing")
                  : t("upgradeConfirmBody")}
          </p>
        </div>
        {upgradeError || upgradeStatus?.last_error ? (
          <div className="form-error">{upgradeError || upgradeStatus.last_error}</div>
        ) : null}
        <div className="modal-actions">
          {upgradePhase === "done" ? (
            <Button variant="primary" size="md" onClick={() => window.location.reload()}>
              {t("upgradeRefresh")}
            </Button>
          ) : upgradePhase === "manual_restart" || upgradeStatus?.manual_restart_required ? (
            <Button variant="secondaryGray" size="md" onClick={onClose}>
              {t("close")}
            </Button>
          ) : (
            <>
              <Button variant="secondaryGray" size="md" onClick={onClose}>
                {upgradeBusy || upgradeStatus?.upgrading ? t("close") : t("upgradeLater")}
              </Button>
              <Button
                variant="primary"
                size="md"
                disabled={upgradeBusy || upgradeStatus?.upgrading || !upgradeStatus?.update_available}
                onClick={onApply}
              >
                {upgradeBusy || upgradeStatus?.upgrading ? t("upgradeActionBusy") : t("upgradeConfirm")}
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
