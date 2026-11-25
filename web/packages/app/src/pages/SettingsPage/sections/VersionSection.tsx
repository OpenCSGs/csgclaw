import { useEffect, useId, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import {
  formatSidebarVersionLabel,
  hasUpgradeAttention,
  inferUpgradeChannelFromVersion,
  isLocalBuildUpgradeStatus,
  upgradeErrorMessage,
} from "@/models/upgradeStatus";
import { SwitchVersionDialog } from "../components/SwitchVersionDialog";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
import versionSectionStyles from "./VersionSection.module.css";
import accountSectionStyles from "./AccountSection.module.css";

export function VersionSection({ highlighted = false }: { highlighted?: boolean }) {
  const restartNoticeId = useId();
  const rowRef = useRef<HTMLElement>(null);
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  const [switchVersionOpen, setSwitchVersionOpen] = useState(false);

  const upgradeStatus = sidebar?.upgradeStatus;
  const refreshUpgradeStatus = sidebar?.onRefreshUpgradeStatus;

  useEffect(() => {
    if (!controller.ready || !sidebar?.showUpgradeControls || !refreshUpgradeStatus) {
      return;
    }
    void refreshUpgradeStatus();
  }, [controller.ready, refreshUpgradeStatus, sidebar?.showUpgradeControls]);

  useEffect(() => {
    if (highlighted) rowRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [highlighted]);

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
    sidebar.upgradeChannelBusy ||
    sidebar.upgradeBusy ||
    sidebar.upgradePhase === "unconfirmed" ||
    upgradeStatus?.checking ||
    upgradeStatus?.upgrading,
  );
  const updateInProgress = sidebar.upgradeBusy || upgradeStatus?.upgrading;
  const pendingStatus =
    sidebar.upgradePhase === "unconfirmed"
      ? t("upgradeUnconfirmedBody")
      : updateInProgress
        ? t(sidebar.upgradePhase === "starting" ? "upgradeStatusStarting" : "upgradeActionBusy")
        : sidebar.upgradeChannelBusy
          ? t("upgradeProgressLabel")
          : upgradeStatus?.checking
            ? t("upgradeStatusChecking")
            : "";
  const versionError = sidebar.upgradeError || upgradeErrorMessage(upgradeStatus, t);
  const showNewVersionBadge = Boolean(
    sidebar.showUpgradeControls &&
    (mockUpgradeAvailable || (!localBuild && upgradeStatus?.update_available)) &&
    sidebar.upgradePhase !== "done",
  );

  const latestVersion = showNewVersionBadge ? upgradeStatus?.latest_version?.trim() : "";

  return (
    <>
      <SettingsRow
        ref={rowRef}
        className={highlighted ? versionSectionStyles.highlightedVersion : undefined}
        title={
          <span className={versionSectionStyles.versionTitle}>
            <span>{t("versionInfo")}</span>
            {showNewVersionBadge ? (
              <span className={versionSectionStyles.versionBadge} aria-label={t("upgradeNewVersionBadge")}>
                <span aria-hidden="true"></span>
                {t("upgradeNewVersionBadge")}
              </span>
            ) : null}
          </span>
        }
        description={t("settingsVersionDescription")}
      >
        <div className={styles.formGrid}>
          <div className={`${versionSectionStyles.versionFields} ${styles.spanAll}`}>
            <div className={styles.formItem}>
              <span>{t("settingsCurrentVersion")}</span>
              <strong className={accountSectionStyles.fieldValue}>{version}</strong>
            </div>
            {latestVersion ? (
              <div className={styles.formItem}>
                <span>{t("upgradeLatestVersion")}</span>
                <strong className={accountSectionStyles.fieldValue}>{formatSidebarVersionLabel(latestVersion)}</strong>
              </div>
            ) : null}
            <div className={styles.formItem}>
              <span>{t("upgradeChannelLabel")}</span>
              {showUpgradeChannel ? (
                <Button
                  variant="linkGray"
                  size="sm"
                  className={versionSectionStyles.versionChannelLink}
                  disabled={upgradeChannelDisabled}
                  onClick={() => setSwitchVersionOpen(true)}
                >
                  {t("upgradeChannelSwitch")}
                </Button>
              ) : (
                <strong className={accountSectionStyles.fieldValue}>{inferredUpgradeChannel || "-"}</strong>
              )}
            </div>
          </div>
          {showUpgradeChannel && pendingStatus ? (
            <p className={`${versionSectionStyles.restartNotice} ${styles.spanAll}`} role="status">
              {pendingStatus}
            </p>
          ) : null}
          {showNewVersionBadge && upgradeStatus?.auto_upgrade_unsupported_reason === "not_official_bundle" ? (
            <p className={`${versionSectionStyles.restartNotice} ${styles.spanAll}`}>{t("upgradeManualUpgradeBody")}</p>
          ) : null}
          {versionError ? (
            <p className={accountSectionStyles.inlineError} role="alert">
              {versionError}
            </p>
          ) : null}
          {showUpgradeAction ? (
            <div className={`${styles.formActions} ${styles.spanAll}`}>
              <Button
                variant="primary"
                size="md"
                disabled={upgradeChannelDisabled}
                aria-describedby={latestVersion ? restartNoticeId : undefined}
                onClick={sidebar.onOpenUpgrade}
              >
                {t("upgradeDownloadAction")}
              </Button>
              {latestVersion ? (
                <span id={restartNoticeId} className={versionSectionStyles.restartNotice}>
                  {t("upgradeRestartNotice")}
                </span>
              ) : null}
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
