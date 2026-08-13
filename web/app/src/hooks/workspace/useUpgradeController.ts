import { useCallback, useEffect, useRef, useState } from "react";
import {
  formatClassifiedUpgradeError,
  inferUpgradeChannelFromVersion,
  normalizeUpgradeStatus,
  upgradeErrorMessage,
} from "@/models/upgradeStatus";
import type { UpgradeChannel, UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import {
  applyPlatformUpgrade,
  setPlatformUpgradeChannel,
  subscribePlatformUpgradeStatus,
} from "@/shared/platform/updatePort";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import type { UpgradeController, UseUpgradeControllerArgs } from "./types";

export const UPGRADE_PAGE_RELOAD_DELAY_MS = 600;

export function useUpgradeController({
  appVersion,
  refreshWorkspaceAppVersion,
  refreshWorkspaceUpgradeStatus,
  setAppVersionData,
  setUpgradeStatusData,
  t,
  upgradeStatus,
}: UseUpgradeControllerArgs): UpgradeController {
  const [upgradeBusy, setUpgradeBusy] = useState(false);
  const [upgradeChannelBusy, setUpgradeChannelBusy] = useState(false);
  const [upgradeChannelError, setUpgradeChannelError] = useState("");
  const [upgradeError, setUpgradeError] = useState("");
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradePhase, setUpgradePhase] = useState<UpgradePhase>("idle");
  const upgradePollTimerRef = useRef<number | null>(null);
  const upgradeChannelLocked = Boolean(getDesktopBridge() && upgradeStatus?.update_available);

  const stopUpgradePoll = useCallback(() => {
    if (upgradePollTimerRef.current) {
      window.clearInterval(upgradePollTimerRef.current);
      upgradePollTimerRef.current = null;
    }
  }, []);

  const handleUpgradeStatusChange = useCallback(
    (payload: unknown) => {
      const next = normalizeUpgradeStatus(payload);
      setUpgradeStatusData(next);
      const statusMessage = upgradeErrorMessage(next, t);
      if (statusMessage) {
        setUpgradeBusy(false);
        setUpgradePhase("error");
        setUpgradeError(statusMessage);
        return;
      }
      setUpgradeError("");
      if (next?.manual_restart_required) {
        setUpgradeBusy(false);
        setUpgradePhase("manual_restart");
        setShowUpgradeModal(true);
      } else if (next?.upgrading) {
        setUpgradeBusy(true);
        setUpgradePhase((phase) => (phase === "done" ? phase : "restarting"));
      } else {
        setUpgradeBusy(false);
        setUpgradePhase((phase) => (phase === "done" ? phase : "idle"));
      }
    },
    [setUpgradeStatusData, t],
  );

  const refreshUpgradeStatus = useCallback(async () => {
    try {
      const payload = await refreshWorkspaceUpgradeStatus();
      if (payload) {
        handleUpgradeStatusChange(payload);
        if (!upgradeErrorMessage(payload, t)) {
          setUpgradeChannelError("");
        }
      }
      return payload;
    } catch (_) {
      return null;
    }
  }, [handleUpgradeStatusChange, refreshWorkspaceUpgradeStatus, t]);

  const restoreUpgradeChannel = useCallback(
    async (channel: UpgradeChannel) => {
      let restored: UpgradeStatus | null = null;
      try {
        restored = await setPlatformUpgradeChannel(channel);
      } catch (_) {
        // The desktop updater may already have rolled back before rejecting the switch.
      }
      try {
        restored = (await refreshWorkspaceUpgradeStatus()) ?? restored;
      } catch (_) {
        // Keep the best status returned while restoring the previous channel.
      }
      if (restored) {
        handleUpgradeStatusChange(restored);
      }
      return restored;
    },
    [handleUpgradeStatusChange, refreshWorkspaceUpgradeStatus],
  );

  const startUpgradeReconnectPoll = useCallback(
    (expectedVersion?: string | null) => {
      stopUpgradePoll();
      let attempts = 0;
      const poll = async () => {
        attempts += 1;
        try {
          const version = await refreshWorkspaceAppVersion({ cacheBust: true });
          const expected = (expectedVersion || "").trim();
          if (version && (!expected || version === expected)) {
            stopUpgradePoll();
            setAppVersionData(version);
            setUpgradeBusy(false);
            setUpgradePhase("done");
            setUpgradeStatusData((current) => ({
              auto_upgrade_supported: current?.auto_upgrade_supported ?? true,
              auto_upgrade_unsupported_reason: current?.auto_upgrade_unsupported_reason ?? "",
              channel: current?.channel ?? "release",
              current_version: version,
              downloaded: false,
              latest_version: version,
              last_checked_at: current?.last_checked_at ?? "",
              update_available: false,
              checking: false,
              manual_restart_required: false,
              upgrading: false,
              last_error: "",
              last_error_kind: "",
              last_error_log_path: "",
            }));
            scheduleUpgradePageReload();
            return;
          }
          const latest = await refreshUpgradeStatus();
          if (latest?.manual_restart_required) {
            stopUpgradePoll();
            setUpgradeBusy(false);
            setUpgradePhase("manual_restart");
            setShowUpgradeModal(true);
            return;
          }
          const statusMessage = upgradeErrorMessage(latest, t);
          if (statusMessage) {
            stopUpgradePoll();
            setUpgradeBusy(false);
            setUpgradePhase("error");
            setShowUpgradeModal(true);
            setUpgradeError(statusMessage);
            return;
          }
        } catch (_) {
          // The daemon is expected to be unavailable while the upgrade helper restarts it.
        }
        if (attempts >= 60) {
          stopUpgradePoll();
          setUpgradeBusy(false);
          setUpgradePhase("error");
          setShowUpgradeModal(true);
          const latest = await refreshUpgradeStatus();
          setUpgradeError(upgradeErrorMessage(latest, t) || t("upgradeApplyFailed"));
        }
      };
      poll();
      upgradePollTimerRef.current = window.setInterval(poll, 2000);
    },
    [refreshUpgradeStatus, refreshWorkspaceAppVersion, setAppVersionData, setUpgradeStatusData, stopUpgradePoll, t],
  );

  const applyUpgrade = useCallback(async () => {
    if (upgradeBusy || upgradeStatus?.upgrading) {
      return;
    }
    if (upgradeStatus?.update_available && upgradeStatus.auto_upgrade_supported === false) {
      setUpgradeBusy(false);
      setUpgradeError("");
      setUpgradePhase("idle");
      setShowUpgradeModal(true);
      return;
    }

    setUpgradeBusy(true);
    setUpgradeError("");
    setUpgradePhase("starting");
    setShowUpgradeModal(true);
    try {
      const applyMode = await applyPlatformUpgrade();
      setUpgradePhase("restarting");
      setUpgradeStatusData((current) => ({
        auto_upgrade_supported: current?.auto_upgrade_supported ?? upgradeStatus?.auto_upgrade_supported ?? true,
        auto_upgrade_unsupported_reason:
          current?.auto_upgrade_unsupported_reason ?? upgradeStatus?.auto_upgrade_unsupported_reason ?? "",
        channel: current?.channel ?? upgradeStatus?.channel ?? "release",
        current_version: current?.current_version || appVersion,
        downloaded: current?.downloaded ?? upgradeStatus?.downloaded ?? false,
        latest_version: current?.latest_version || upgradeStatus?.latest_version || "",
        update_available: current?.update_available ?? Boolean(upgradeStatus?.update_available),
        checking: current?.checking ?? false,
        last_checked_at: current?.last_checked_at ?? "",
        manual_restart_required: false,
        upgrading: true,
        last_error: "",
        last_error_kind: "",
        last_error_log_path: "",
      }));
      if (applyMode === "browser-daemon") {
        startUpgradeReconnectPoll(upgradeStatus?.latest_version);
      }
      setShowUpgradeModal(false);
    } catch (err: unknown) {
      setUpgradeBusy(false);
      setUpgradePhase("error");
      const latest = await refreshUpgradeStatus().catch(() => null);
      const statusMessage = upgradeErrorMessage(latest, t);
      if (statusMessage) {
        setUpgradeError(statusMessage);
        return;
      }
      const detail = upgradeErrorDetail(err);
      setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
    }
  }, [
    appVersion,
    refreshUpgradeStatus,
    setUpgradeStatusData,
    startUpgradeReconnectPoll,
    t,
    upgradeBusy,
    upgradeStatus,
  ]);

  const changeUpgradeChannel = useCallback(
    async (channel: UpgradeChannel) => {
      const currentVersion = upgradeStatus?.current_version || appVersion;
      const inferredChannel = inferUpgradeChannelFromVersion(currentVersion);
      const retryCurrentChannel = Boolean(
        channel === inferredChannel && (upgradeChannelError || upgradeError || upgradeErrorMessage(upgradeStatus, t)),
      );
      if (upgradeChannelBusy || upgradeBusy || upgradeStatus?.checking || upgradeStatus?.upgrading) {
        return false;
      }
      if (channel === inferredChannel && !retryCurrentChannel) {
        return true;
      }
      setUpgradeChannelBusy(true);
      setUpgradeChannelError("");
      try {
        const next = await setPlatformUpgradeChannel(channel);
        handleUpgradeStatusChange(next);
        const statusMessage = upgradeErrorMessage(next, t);
        if (statusMessage) {
          const restored = await restoreUpgradeChannel(inferredChannel);
          if (retryCurrentChannel && restored && !upgradeErrorMessage(restored, t)) {
            return true;
          }
          setUpgradeChannelError(`${t("upgradeChannelSwitchFailed")} ${statusMessage}`.trim());
          return false;
        }
        return true;
      } catch (error) {
        const detail = upgradeErrorDetail(error).trim();
        const classified = formatClassifiedUpgradeError(detail, t);
        const restored = await restoreUpgradeChannel(inferredChannel);
        if (retryCurrentChannel && restored && !upgradeErrorMessage(restored, t)) {
          return true;
        }
        setUpgradeChannelError(`${t("upgradeChannelSwitchFailed")} ${classified}`.trim());
        return false;
      } finally {
        setUpgradeChannelBusy(false);
      }
    },
    [
      appVersion,
      handleUpgradeStatusChange,
      restoreUpgradeChannel,
      t,
      upgradeBusy,
      upgradeChannelBusy,
      upgradeChannelError,
      upgradeError,
      upgradeStatus,
    ],
  );

  const openUpgradeModal = useCallback(() => {
    if (upgradePhase !== "error") {
      setUpgradeError("");
    }
    setUpgradePhase((phase) => {
      if (phase === "done" || phase === "error" || phase === "manual_restart") {
        return phase;
      }
      if (upgradeStatus?.manual_restart_required) {
        return "manual_restart";
      }
      return upgradeBusy || upgradeStatus?.upgrading ? "restarting" : "idle";
    });
    setShowUpgradeModal(true);
  }, [upgradeBusy, upgradePhase, upgradeStatus?.manual_restart_required, upgradeStatus?.upgrading]);

  useEffect(() => {
    return subscribePlatformUpgradeStatus(handleUpgradeStatusChange);
  }, [handleUpgradeStatusChange]);

  useEffect(() => {
    return () => {
      stopUpgradePoll();
    };
  }, [stopUpgradePoll]);

  return {
    changeUpgradeChannel,
    upgradeBusy,
    upgradeChannelBusy,
    upgradeChannelError,
    upgradeChannelLocked,
    upgradeError,
    upgradePhase,
    showUpgradeModal,
    handleUpgradeStatusChange,
    openUpgradeModal,
    refreshUpgradeStatus,
    upgradeModalProps: showUpgradeModal
      ? {
          t,
          upgradeStatus,
          appVersion,
          upgradePhase,
          upgradeBusy,
          upgradeError,
          onClose: () => setShowUpgradeModal(false),
          onApply: applyUpgrade,
        }
      : null,
  };
}

function upgradeErrorDetail(err: unknown): string {
  const message = err instanceof Error ? err.message : "";
  return message && message !== "upgrade apply failed" ? ` ${message}` : "";
}

export function scheduleUpgradePageReload(reloadPage?: () => void): ReturnType<typeof setTimeout> {
  return setTimeout(() => {
    if (reloadPage) {
      reloadPage();
      return;
    }
    if (typeof window !== "undefined") {
      window.location.reload();
    }
  }, UPGRADE_PAGE_RELOAD_DELAY_MS);
}
