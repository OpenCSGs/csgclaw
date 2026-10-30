import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import {
  formatSidebarVersionLabel,
  hasUpgradeAttention,
  inferUpgradeChannelFromVersion,
  isLocalBuildUpgradeStatus,
  upgradeErrorMessage,
} from "@/models/upgradeStatus";
import { nextSimulatedUpgradeProgress } from "@/models/upgradeProgress";
import { SwitchVersionDialog } from "../components/SwitchVersionDialog";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function VersionSection() {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  const [switchVersionOpen, setSwitchVersionOpen] = useState(false);

  const upgradeStatus = sidebar?.upgradeStatus;
  const progressActive = Boolean(
    sidebar && (sidebar.upgradeChannelBusy || upgradeStatus?.checking || upgradeStatus?.upgrading),
  );
  const upgradeProgress = useSimulatedUpgradeProgress(progressActive, Boolean(upgradeStatus?.downloaded));
  const refreshUpgradeStatus = sidebar?.onRefreshUpgradeStatus;

  useEffect(() => {
    if (!controller.ready || !sidebar?.showUpgradeControls || !refreshUpgradeStatus) {
      return;
    }
    void refreshUpgradeStatus();
  }, [controller.ready, refreshUpgradeStatus, sidebar?.showUpgradeControls]);

  if (!sidebar) {
    return null;
  }
  const t = sidebar.t;

  const currentVersion = upgradeStatus?.current_version || sidebar.appVersion;
  const version = formatSidebarVersionLabel(currentVersion);
  const localBuild = isLocalBuildUpgradeStatus(upgradeStatus, currentVersion);
  const mockUpgradeAvailable = import.meta.env.DEV && isMockUpgradePreviewEnabled();
  const showUpgradeAction =
    sidebar.showUpgradeControls &&
    (mockUpgradeAvailable ||
      (!localBuild &&
        upgradeStatus?.auto_upgrade_supported !== false &&
        hasUpgradeAttention(upgradeStatus, sidebar.upgradePhase, sidebar.upgradeBusy)));
  const showUpgradeChannel = sidebar.showUpgradeControls;
  const inferredUpgradeChannel = inferUpgradeChannelFromVersion(currentVersion);
  const upgradeChannelDisabled = Boolean(
    sidebar.upgradeChannelBusy || sidebar.upgradeBusy || upgradeStatus?.checking || upgradeStatus?.upgrading,
  );
  const showUpgradeProgress = upgradeProgress.visible;
  const versionError = sidebar.upgradeError || upgradeErrorMessage(upgradeStatus, t);
  const showNewVersionBadge = Boolean(
    sidebar.showUpgradeControls &&
    (mockUpgradeAvailable || (!localBuild && upgradeStatus?.update_available)) &&
    sidebar.upgradePhase !== "done",
  );

  return (
    <>
      <SettingsRow
        title={
          <span className={styles.versionTitle}>
            <span>{t("versionInfo")}</span>
            {showNewVersionBadge ? (
              <span className={styles.versionBadge} aria-label={t("upgradeNewVersionBadge")}>
                <span aria-hidden="true"></span>
                {t("upgradeNewVersionBadge")}
              </span>
            ) : null}
          </span>
        }
        description={t("settingsVersionDescription")}
      >
        <div className={styles.formGrid}>
          <div className="field">
            <span>{t("settingsCurrentVersion")}</span>
            <strong className={styles.fieldValue}>{version}</strong>
          </div>
          <div className="field">
            <span>{t("upgradeChannelLabel")}</span>
            {showUpgradeChannel ? (
              <Button
                variant="linkGray"
                size="sm"
                className={styles.versionChannelLink}
                disabled={upgradeChannelDisabled}
                onClick={() => setSwitchVersionOpen(true)}
              >
                {t("upgradeChannelSwitch")}
              </Button>
            ) : (
              <strong className={styles.fieldValue}>{inferredUpgradeChannel || "-"}</strong>
            )}
          </div>
          {showUpgradeChannel && showUpgradeProgress ? (
            <div
              className={`${styles.upgradeProgressRow} ${styles.spanAll}`}
              role="progressbar"
              aria-label={t("upgradeProgressLabel")}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={upgradeProgress.percent}
            >
              <div className={styles.upgradeProgressTrack} aria-hidden="true">
                <span style={{ width: `${upgradeProgress.percent}%` }}></span>
              </div>
              <span className={styles.upgradeProgressValue}>{upgradeProgress.percent}%</span>
            </div>
          ) : null}
          {versionError ? (
            <p className={styles.inlineError} role="alert">
              {versionError}
            </p>
          ) : null}
          {showUpgradeAction ? (
            <div className={`${styles.formActions} ${styles.spanAll}`}>
              <Button
                variant="primary"
                size="md"
                disabled={sidebar.upgradeBusy || upgradeStatus?.upgrading}
                onClick={sidebar.onOpenUpgrade}
              >
                {t(upgradeStatus?.downloaded ? "upgradeAction" : "upgradeDownloadAction")}
              </Button>
            </div>
          ) : null}
        </div>
      </SettingsRow>

      <SwitchVersionDialog
        busy={upgradeChannelDisabled}
        currentChannel={inferredUpgradeChannel}
        error={sidebar.upgradeChannelError}
        open={switchVersionOpen}
        t={t}
        onConfirm={sidebar.onUpgradeChannelChange}
        onOpenChange={setSwitchVersionOpen}
      />
    </>
  );
}

function isMockUpgradePreviewEnabled(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  if (new URLSearchParams(window.location.search).get("mockUpgrade") === "1") {
    return true;
  }
  const hashQuery = window.location.hash.split("?")[1] || "";
  return new URLSearchParams(hashQuery).get("mockUpgrade") === "1";
}

/** Simulated display progress; does not measure download progress. */
function useSimulatedUpgradeProgress(active: boolean, completed: boolean) {
  const [percent, setPercent] = useState(0);
  const [visible, setVisible] = useState(false);
  const activeRunRef = useRef(false);

  useEffect(() => {
    if (active) {
      activeRunRef.current = true;
      setVisible(true);
      setPercent((current) => (current > 0 && current < 100 ? current : 3));
      const timer = window.setInterval(() => {
        setPercent((current) => nextSimulatedUpgradeProgress(current));
      }, 1000);
      return () => window.clearInterval(timer);
    }
    if (completed && activeRunRef.current) {
      setPercent(100);
      setVisible(true);
      const timer = window.setTimeout(() => {
        activeRunRef.current = false;
        setVisible(false);
      }, 600);
      return () => window.clearTimeout(timer);
    }
    activeRunRef.current = false;
    setPercent(0);
    setVisible(false);
    return undefined;
  }, [active, completed]);

  return { percent, visible };
}
