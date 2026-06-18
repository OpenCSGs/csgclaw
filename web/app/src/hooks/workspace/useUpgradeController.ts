import { useCallback, useEffect, useRef, useState } from "react";
import { applyUpgradeRequest } from "@/api/upgrade";
import { normalizeUpgradeStatus } from "@/models/upgradeStatus";
import type { UpgradePhase } from "@/models/upgradeStatus";
import type { UpgradeController, UseUpgradeControllerArgs } from "./types";

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
  const [upgradeError, setUpgradeError] = useState("");
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradePhase, setUpgradePhase] = useState<UpgradePhase>("idle");
  const upgradePollTimerRef = useRef<number | null>(null);

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
      if (next?.manual_restart_required) {
        setUpgradeBusy(false);
        setUpgradePhase("manual_restart");
        setShowUpgradeModal(true);
      } else if (next?.upgrading) {
        setUpgradeBusy(true);
        setUpgradePhase((phase) => (phase === "done" ? phase : "restarting"));
      } else if (!next?.update_available) {
        setUpgradeBusy(false);
      }
    },
    [setUpgradeStatusData],
  );

  const refreshUpgradeStatus = useCallback(async () => {
    const payload = await refreshWorkspaceUpgradeStatus();
    if (payload?.manual_restart_required) {
      setUpgradeBusy(false);
      setUpgradePhase("manual_restart");
      setShowUpgradeModal(true);
    } else if (payload?.upgrading) {
      setUpgradeBusy(true);
      setUpgradePhase((phase) => (phase === "done" ? phase : "restarting"));
    } else if (!payload?.update_available) {
      setUpgradeBusy(false);
    }
    return payload;
  }, [refreshWorkspaceUpgradeStatus]);

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
              current_version: version,
              latest_version: version,
              last_checked_at: current?.last_checked_at ?? "",
              update_available: false,
              checking: false,
              manual_restart_required: false,
              upgrading: false,
              last_error: "",
            }));
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
          if (latest?.last_error) {
            stopUpgradePoll();
            setUpgradeBusy(false);
            setUpgradePhase("error");
            setShowUpgradeModal(true);
            setUpgradeError(`${t("upgradeApplyFailed")} ${latest.last_error}`.trim());
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
          const detail = latest?.last_error ? ` ${latest.last_error}` : "";
          setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
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
      await applyUpgradeRequest();
      setUpgradePhase("restarting");
      setUpgradeStatusData((current) => ({
        auto_upgrade_supported: current?.auto_upgrade_supported ?? upgradeStatus?.auto_upgrade_supported ?? true,
        auto_upgrade_unsupported_reason:
          current?.auto_upgrade_unsupported_reason ?? upgradeStatus?.auto_upgrade_unsupported_reason ?? "",
        current_version: current?.current_version || appVersion,
        latest_version: current?.latest_version || upgradeStatus?.latest_version || "",
        update_available: current?.update_available ?? Boolean(upgradeStatus?.update_available),
        checking: current?.checking ?? false,
        last_checked_at: current?.last_checked_at ?? "",
        manual_restart_required: false,
        upgrading: true,
        last_error: "",
      }));
      startUpgradeReconnectPoll(upgradeStatus?.latest_version);
      setShowUpgradeModal(false);
    } catch (err: unknown) {
      setUpgradeBusy(false);
      setUpgradePhase("error");
      const detail = upgradeErrorDetail(err);
      setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
    }
  }, [appVersion, setUpgradeStatusData, startUpgradeReconnectPoll, t, upgradeBusy, upgradeStatus]);

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
    return () => {
      stopUpgradePoll();
    };
  }, [stopUpgradePoll]);

  return {
    upgradeBusy,
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
