// @ts-nocheck
import { Button } from "@/components/ui";
import { upgradeStatusLabel } from "@/models/upgradeStatus";

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
          <Button className="modal-close" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
        <div className="upgrade-summary">
          <div className="upgrade-summary-row">
            <span>{t("upgradeCurrentVersion")}</span>
            <strong>{upgradeStatus?.current_version || appVersion || "dev"}</strong>
          </div>
          <div className="upgrade-summary-row">
            <span>{t("upgradeLatestVersion")}</span>
            <strong>{upgradeStatus?.latest_version || t("upgradeNoLatest")}</strong>
          </div>
          <div className="upgrade-summary-row">
            <span>{t("upgradeStatus")}</span>
            <strong>{upgradeStatusLabel(upgradePhase, t)}</strong>
          </div>
        </div>
        <div className={`upgrade-status-card ${upgradePhase}`}>
          <span className="upgrade-status-dot" aria-hidden="true"></span>
          <p>
            {upgradePhase === "done"
              ? t("upgradeDoneBody")
              : upgradePhase === "restarting" || upgradePhase === "starting" || upgradeBusy || upgradeStatus?.upgrading
                ? t("upgradeContinueUsing")
                : t("upgradeConfirmBody")}
          </p>
        </div>
        {upgradeError || upgradeStatus?.last_error ? (
          <div className="form-error">{upgradeError || upgradeStatus.last_error}</div>
        ) : null}
        <div className="modal-actions">
          {upgradePhase === "done" ? (
            <Button variant="primary" className="send-button" onClick={() => window.location.reload()}>
              {t("upgradeRefresh")}
            </Button>
          ) : (
            <>
              <Button className="secondary-button" onClick={onClose}>
                {upgradeBusy || upgradeStatus?.upgrading ? t("close") : t("upgradeLater")}
              </Button>
              <Button
                variant="primary"
                className="send-button"
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
