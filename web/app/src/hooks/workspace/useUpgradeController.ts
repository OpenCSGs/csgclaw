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
      if (next?.upgrading) {
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
    if (payload?.upgrading) {
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
              current_version: version,
              latest_version: version,
              last_checked_at: current?.last_checked_at ?? "",
              update_available: false,
              checking: false,
              upgrading: false,
              last_error: "",
            }));
            return;
          }
        } catch (_) {
          // The daemon is expected to be unavailable while the upgrade helper restarts it.
        }
        if (attempts >= 60) {
          stopUpgradePoll();
          setUpgradeBusy(false);
          setUpgradePhase("error");
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

    setUpgradeBusy(true);
    setUpgradeError("");
    setUpgradePhase("starting");
    setShowUpgradeModal(true);
    try {
      await applyUpgradeRequest();
      setUpgradePhase("restarting");
      setUpgradeStatusData((current) => ({
        current_version: current?.current_version || appVersion,
        latest_version: current?.latest_version || upgradeStatus?.latest_version || "",
        update_available: current?.update_available ?? Boolean(upgradeStatus?.update_available),
        checking: current?.checking ?? false,
        last_checked_at: current?.last_checked_at ?? "",
        upgrading: true,
        last_error: "",
      }));
      startUpgradeReconnectPoll(upgradeStatus?.latest_version);
      setShowUpgradeModal(false);
    } catch (err) {
      setUpgradeBusy(false);
      setUpgradePhase("error");
      const detail = err?.message && err.message !== "upgrade apply failed" ? ` ${err.message}` : "";
      setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
    }
  }, [appVersion, setUpgradeStatusData, startUpgradeReconnectPoll, t, upgradeBusy, upgradeStatus]);

  const openUpgradeModal = useCallback(() => {
    setUpgradeError("");
    setUpgradePhase(upgradeBusy || upgradeStatus?.upgrading ? "restarting" : "idle");
    setShowUpgradeModal(true);
  }, [upgradeBusy, upgradeStatus?.upgrading]);

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
