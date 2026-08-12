import { Button } from "@/components/ui";
import {
  formatSidebarVersionLabel,
  isLocalBuildVersion,
  upgradeErrorMessage,
  upgradeStatusLabel,
} from "@/models/upgradeStatus";
import type { UpgradeChannel, UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import type { TranslateFn } from "@/models/conversations";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import { ModalCloseButton } from "./ModalCloseButton";

export type UpgradeModalProps = {
  appVersion?: string;
  onApply: () => void | Promise<void>;
  onChannelChange: (channel: UpgradeChannel) => void | Promise<void>;
  onClose: () => void;
  t: TranslateFn;
  upgradeBusy?: boolean;
  upgradeChannelBusy?: boolean;
  upgradeChannelLocked?: boolean;
  upgradeError?: string;
  upgradePhase: UpgradePhase;
  upgradeStatus?: UpgradeStatus | null;
};

export function UpgradeModal({
  t,
  upgradeStatus,
  appVersion = "",
  upgradePhase,
  upgradeBusy = false,
  upgradeChannelBusy = false,
  upgradeChannelLocked = false,
  upgradeError = "",
  onClose,
  onApply,
  onChannelChange,
}: UpgradeModalProps) {
  const currentVersion = upgradeStatus?.current_version || appVersion || "dev";
  const channel = upgradeStatus?.channel ?? "release";
  const channelDisabled = Boolean(
    upgradeBusy || upgradeChannelBusy || upgradeChannelLocked || upgradeStatus?.upgrading,
  );
  const manualUpgradeRequired = Boolean(
    upgradeStatus?.update_available && upgradeStatus.auto_upgrade_supported === false,
  );
  const statusLabel = isLocalBuildVersion(currentVersion)
    ? t("upgradeStatusLocal")
    : manualUpgradeRequired
      ? t("upgradeStatusManualUpgrade")
      : upgradeStatusLabel(upgradePhase, t);
  const subtitle = manualUpgradeRequired ? t("upgradeManualUpgradeSubtitle") : t("upgradeSubtitle");
  const statusError = upgradeErrorMessage(upgradeStatus, t);
  const visibleError = upgradeError || statusError;
  const desktopApp = Boolean(getDesktopBridge());
  return (
    <div className="modal-backdrop">
      <div className="modal-card upgrade-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("upgradeTitle")}</div>
            <div className="modal-subtitle">{subtitle}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>
        <div className="upgrade-channel-card">
          <div className="upgrade-channel-copy">
            <strong>{t("upgradeChannel")}</strong>
            <span>{t("upgradeChannelHint")}</span>
          </div>
          <div className="upgrade-channel-switch" role="group" aria-label={t("upgradeChannel")}>
            {(["release", "beta"] as const).map((value) => (
              <button
                key={value}
                type="button"
                className={channel === value ? "active" : ""}
                aria-pressed={channel === value}
                disabled={channelDisabled}
                onClick={() => void onChannelChange(value)}
              >
                {t(value === "release" ? "upgradeChannelRelease" : "upgradeChannelBeta")}
              </button>
            ))}
          </div>
        </div>
        <div className="upgrade-summary">
          <div className="upgrade-summary-row">
            <span>{t("upgradeCurrentVersion")}</span>
            <strong>{formatSidebarVersionLabel(currentVersion)}</strong>
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
            <strong>{statusLabel}</strong>
          </div>
        </div>
        <div className={`upgrade-status-card ${upgradePhase}`}>
          <span className="upgrade-status-dot" aria-hidden="true"></span>
          <p>
            {upgradePhase === "manual_restart" || upgradeStatus?.manual_restart_required
              ? t("upgradeManualRestartBody")
              : manualUpgradeRequired
                ? t("upgradeManualUpgradeBody")
                : upgradePhase === "done"
                  ? t("upgradeDoneBody")
                  : upgradePhase === "restarting" ||
                      upgradePhase === "starting" ||
                      upgradeBusy ||
                      upgradeStatus?.upgrading
                    ? t("upgradeContinueUsing")
                    : desktopApp
                      ? t(upgradeStatus?.downloaded ? "upgradeDesktopDownloadedBody" : "upgradeDesktopConfirmBody")
                      : t("upgradeConfirmBody")}
          </p>
        </div>
        {visibleError ? <div className="form-error">{visibleError}</div> : null}
        <div className="modal-actions">
          {upgradePhase === "done" ? (
            <Button variant="primary" size="md" onClick={() => window.location.reload()}>
              {t("upgradeRefresh")}
            </Button>
          ) : manualUpgradeRequired ? (
            <Button variant="secondaryGray" size="md" onClick={onClose}>
              {t("close")}
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
                {upgradeBusy || upgradeStatus?.upgrading
                  ? t("upgradeActionBusy")
                  : t(desktopApp && !upgradeStatus?.downloaded ? "upgradeDownloadAction" : "upgradeConfirm")}
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
