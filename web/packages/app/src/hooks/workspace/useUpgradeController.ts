import { useCallback, useEffect, useRef, useState } from "react";
import { apiErrorCode, errorMessage as apiErrorMessage } from "@/api/client";
import {
  classifyDesktopUpdateErrorKind,
  formatSidebarVersionLabel,
  hasUpgradeStatusError,
  inferUpgradeChannelFromVersion,
  isLocalBuildUpgradeStatus,
  normalizeUpgradeStatus,
  upgradeErrorKey,
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
const UPGRADE_POLL_INTERVAL_MS = 2000;
const UPGRADE_POLL_TIMEOUT_MS = 120_000;

type UpgradeFailure = UpgradeStatus | string | null;

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
  const [channelFailure, setChannelFailure] = useState<UpgradeFailure>(null);
  const [upgradeFailure, setUpgradeFailure] = useState<UpgradeFailure>(null);
  const upgradeChannelError = formatUpgradeFailure(channelFailure, t);
  const upgradeError = formatUpgradeFailure(upgradeFailure, t);
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
      if (next && hasUpgradeStatusError(next)) {
        setUpgradeBusy(false);
        setUpgradePhase("error");
        setUpgradeFailure(next);
        return;
      }
      setUpgradeFailure(null);
      if (next?.manual_restart_required) {
        setUpgradeBusy(false);
        setUpgradePhase("manual_restart");
        setShowUpgradeModal(true);
      } else if (next?.upgrading) {
        setUpgradeBusy(true);
        setUpgradePhase((phase) => (phase === "done" ? phase : "restarting"));
      } else if (upgradePollTimerRef.current === null) {
        setUpgradeBusy(false);
        setUpgradePhase((phase) => (phase === "done" || phase === "unconfirmed" ? phase : "idle"));
      }
    },
    [setUpgradeStatusData],
  );

  const refreshUpgradeStatus = useCallback(async () => {
    try {
      const payload = await refreshWorkspaceUpgradeStatus();
      if (payload) {
        handleUpgradeStatusChange(payload);
        if (!hasUpgradeStatusError(payload)) {
          setChannelFailure(null);
        }
      }
      return payload;
    } catch (_) {
      return null;
    }
  }, [handleUpgradeStatusChange, refreshWorkspaceUpgradeStatus]);

  const startUpgradeReconnectPoll = useCallback(
    (expectedVersion?: string | null) => {
      stopUpgradePoll();
      const expected = expectedVersion?.trim();
      const deadline = Date.now() + UPGRADE_POLL_TIMEOUT_MS;
      let inFlight = false;
      const poll = async () => {
        if (Date.now() >= deadline) {
          stopUpgradePoll();
          setUpgradeBusy(false);
          setUpgradePhase("unconfirmed");
          setShowUpgradeModal(true);
          return;
        }
        if (inFlight) return;
        inFlight = true;
        try {
          const version = await refreshWorkspaceAppVersion({ cacheBust: true }).catch(() => "");
          if (upgradePollTimerRef.current !== timer) return;
          if (version && expected && formatSidebarVersionLabel(version) === formatSidebarVersionLabel(expected)) {
            stopUpgradePoll();
            setAppVersionData(version);
            setUpgradeBusy(false);
            setUpgradePhase("done");
            setUpgradeStatusData((current) => ({
              auto_upgrade_supported: current?.auto_upgrade_supported ?? true,
              auto_upgrade_unsupported_reason: current?.auto_upgrade_unsupported_reason ?? "",
              channel: inferUpgradeChannelFromVersion(version),
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
          const latest = await refreshWorkspaceUpgradeStatus();
          if (upgradePollTimerRef.current !== timer) return;
          if (latest?.manual_restart_required || hasUpgradeStatusError(latest)) {
            stopUpgradePoll();
            handleUpgradeStatusChange(latest);
            setShowUpgradeModal(true);
          }
        } catch {
          // A request can fail while the local service restarts.
        } finally {
          inFlight = false;
        }
      };
      const timer = window.setInterval(() => void poll(), UPGRADE_POLL_INTERVAL_MS);
      upgradePollTimerRef.current = timer;
      void poll();
    },
    [
      handleUpgradeStatusChange,
      refreshWorkspaceAppVersion,
      refreshWorkspaceUpgradeStatus,
      setAppVersionData,
      setUpgradeStatusData,
      stopUpgradePoll,
    ],
  );

  const applyUpgrade = useCallback(async () => {
    if (upgradeBusy || upgradeChannelBusy || upgradeStatus?.checking || upgradeStatus?.upgrading) {
      return;
    }
    if (upgradeStatus?.update_available && upgradeStatus.auto_upgrade_supported === false) {
      setUpgradeBusy(false);
      setUpgradeFailure(null);
      setUpgradePhase("idle");
      setShowUpgradeModal(true);
      return;
    }

    setUpgradeBusy(true);
    setUpgradeFailure(null);
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
      const latest = await refreshUpgradeStatus();
      if (latest?.upgrading || latest?.manual_restart_required) {
        if (latest.upgrading && !getDesktopBridge()) {
          startUpgradeReconnectPoll(latest.latest_version);
        }
        return;
      }
      setUpgradeBusy(false);
      setUpgradePhase("error");
      setUpgradeFailure(
        latest && hasUpgradeStatusError(latest) ? latest : requestUpgradeErrorKey(err, "upgradeApplyFailed"),
      );
    }
  }, [
    appVersion,
    refreshUpgradeStatus,
    setUpgradeStatusData,
    startUpgradeReconnectPoll,
    upgradeBusy,
    upgradeChannelBusy,
    upgradeStatus,
  ]);

  const changeUpgradeChannel = useCallback(
    async (channel: UpgradeChannel) => {
      const currentVersion = upgradeStatus?.current_version || appVersion;
      const inferredChannel = inferUpgradeChannelFromVersion(currentVersion);
      if (isLocalBuildUpgradeStatus(upgradeStatus, currentVersion)) {
        setChannelFailure("upgradeLocalBuildUnsupported");
        return false;
      }
      const retryCurrentChannel = Boolean(
        channel === inferredChannel && (channelFailure || upgradeFailure || hasUpgradeStatusError(upgradeStatus)),
      );
      if (upgradeChannelBusy || upgradeBusy || upgradeStatus?.checking || upgradeStatus?.upgrading) {
        return false;
      }
      if (channel === inferredChannel && !retryCurrentChannel) {
        return true;
      }
      setUpgradeChannelBusy(true);
      setChannelFailure(null);
      try {
        const next = await setPlatformUpgradeChannel(channel);
        handleUpgradeStatusChange(next);
        if (hasUpgradeStatusError(next)) {
          setChannelFailure(next);
          return false;
        }
        if (!getDesktopBridge() && channel !== inferredChannel && next.upgrading) {
          startUpgradeReconnectPoll(next.latest_version);
        }
        return true;
      } catch (error) {
        setChannelFailure(requestUpgradeErrorKey(error, "upgradeChannelSwitchFailed"));
        return false;
      } finally {
        setUpgradeChannelBusy(false);
      }
    },
    [
      appVersion,
      handleUpgradeStatusChange,
      startUpgradeReconnectPoll,
      channelFailure,
      upgradeBusy,
      upgradeChannelBusy,
      upgradeFailure,
      upgradeStatus,
    ],
  );

  const openUpgradeModal = useCallback(() => {
    if (upgradePhase !== "error" && upgradePhase !== "unconfirmed") {
      setUpgradeFailure(null);
    }
    setUpgradePhase((phase) => {
      if (phase === "done" || phase === "error" || phase === "manual_restart" || phase === "unconfirmed") {
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

function formatUpgradeFailure(failure: UpgradeFailure, t: UseUpgradeControllerArgs["t"]): string {
  return typeof failure === "string" ? t(failure) : upgradeErrorMessage(failure, t);
}

function requestUpgradeErrorKey(error: unknown, fallback: string): string {
  const classified = classifyDesktopUpdateErrorKind(apiErrorMessage(error));
  return upgradeErrorKey(apiErrorCode(error), upgradeErrorKey(classified, fallback));
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
