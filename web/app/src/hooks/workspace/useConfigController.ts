import { useCallback, useEffect, useRef, useState } from "react";
import { fetchServerConfig, fetchServerRestartStatus, restartServer, updateServerConfig } from "@/api/config";
import { errorMessage } from "@/api/client";
import type { ConfigSettingsDraft } from "@/models/configSettings";
import { configDraftToUpdatePayload, configSettingsToDraft, normalizeConfigSettings } from "@/models/configSettings";
import type { ConfigController, UseConfigControllerArgs } from "./types";

export type ConfigPhase = "idle" | "loading" | "saving" | "restarting" | "manual_restart" | "done" | "error";

const emptyDraft = (): ConfigSettingsDraft => ({
  listen_host: "0.0.0.0",
  listen_port: "18080",
  advertise_base_url: "",
  advertise_base_url_effective: "",
  access_token: "",
  access_token_set: false,
  access_token_preview: "",
  show_upgrade: true,
  sandbox_provider: "boxlite",
  hub_local_path: "",
  hub_official_url: "",
  default_manager_template: "",
  default_worker_template: "",
});

export function useConfigController({
  hubTemplates,
  refreshWorkspaceAppVersion,
  t,
}: UseConfigControllerArgs): ConfigController {
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [configDraft, setConfigDraft] = useState<ConfigSettingsDraft>(emptyDraft);
  const [sandboxProviders, setSandboxProviders] = useState<string[]>([]);
  const [configBusy, setConfigBusy] = useState(false);
  const [configError, setConfigError] = useState("");
  const [configPhase, setConfigPhase] = useState<ConfigPhase>("idle");
  const configPollTimerRef = useRef<number | null>(null);

  const stopConfigPoll = useCallback(() => {
    if (configPollTimerRef.current) {
      window.clearInterval(configPollTimerRef.current);
      configPollTimerRef.current = null;
    }
  }, []);

  const loadConfig = useCallback(async () => {
    setConfigBusy(true);
    setConfigError("");
    setConfigPhase("loading");
    try {
      const payload = normalizeConfigSettings(await fetchServerConfig());
      if (!payload) {
        throw new Error(t("configSettingsLoadFailed"));
      }
      setConfigDraft(configSettingsToDraft(payload));
      setSandboxProviders(payload.supported_sandbox_providers || []);
      setConfigPhase("idle");
    } catch (err: unknown) {
      setConfigPhase("error");
      setConfigError(configErrorDetail(err, t("configSettingsLoadFailed")));
    } finally {
      setConfigBusy(false);
    }
  }, [t]);

  const openConfigModal = useCallback(() => {
    setShowConfigModal(true);
    void loadConfig();
  }, [loadConfig]);

  const patchConfigDraft = useCallback((patch: Partial<ConfigSettingsDraft>) => {
    setConfigDraft((current) => ({ ...current, ...patch }));
  }, []);

  const startConfigReconnectPoll = useCallback(() => {
    stopConfigPoll();
    let attempts = 0;
    let sawDisconnect = false;
    const finishManualRestart = () => {
      stopConfigPoll();
      setConfigBusy(false);
      setConfigPhase("manual_restart");
      setConfigError("");
    };

    const poll = async () => {
      attempts += 1;

      try {
        const status = await fetchServerRestartStatus();
        if (status?.manual_restart_required) {
          finishManualRestart();
          return;
        }
        if (status?.last_error) {
          stopConfigPoll();
          setConfigBusy(false);
          setConfigPhase("error");
          setConfigError(`${t("configSettingsRestartFailed")} ${status.last_error}`.trim());
          return;
        }
      } catch (_) {
        // Status endpoint may be unavailable while the server is restarting.
      }

      try {
        await refreshWorkspaceAppVersion({ cacheBust: true });
        if (sawDisconnect) {
          stopConfigPoll();
          setConfigBusy(false);
          setConfigPhase("done");
          setConfigError("");
          return;
        }
      } catch (_) {
        sawDisconnect = true;
      }

      if (attempts >= 60) {
        stopConfigPoll();
        setConfigBusy(false);
        setConfigPhase("error");
        try {
          const status = await fetchServerRestartStatus();
          if (status?.manual_restart_required) {
            finishManualRestart();
            return;
          }
          const detail = status?.last_error ? ` ${status.last_error}` : "";
          setConfigError(`${t("configSettingsRestartFailed")}${detail}`);
        } catch (_) {
          setConfigError(t("configSettingsRestartFailed"));
        }
      }
    };
    poll();
    configPollTimerRef.current = window.setInterval(poll, 2000);
  }, [refreshWorkspaceAppVersion, stopConfigPoll, t]);

  const saveAndRestart = useCallback(async () => {
    if (configBusy) {
      return;
    }
    setConfigBusy(true);
    setConfigError("");
    setConfigPhase("saving");
    try {
      const saved = normalizeConfigSettings(await updateServerConfig(configDraftToUpdatePayload(configDraft)));
      if (saved) {
        setConfigDraft(configSettingsToDraft(saved));
        setSandboxProviders(saved.supported_sandbox_providers || sandboxProviders);
      }
      setConfigPhase("restarting");
      await restartServer();
      startConfigReconnectPoll();
    } catch (err: unknown) {
      setConfigBusy(false);
      setConfigPhase("error");
      setConfigError(configErrorDetail(err, t("configSettingsSaveFailed")));
    }
  }, [configBusy, configDraft, sandboxProviders, startConfigReconnectPoll, t]);

  useEffect(() => {
    return () => {
      stopConfigPoll();
    };
  }, [stopConfigPoll]);

  return {
    openConfigModal,
    configModalProps: showConfigModal
      ? {
          t,
          configDraft,
          hubTemplates: hubTemplates || [],
          sandboxProviders,
          configBusy,
          configError,
          configPhase,
          onDraftChange: patchConfigDraft,
          onClose: () => {
            if (!configBusy) {
              setShowConfigModal(false);
              setConfigPhase("idle");
              setConfigError("");
            }
          },
          onReload: () => window.location.reload(),
          onSaveAndRestart: saveAndRestart,
        }
      : null,
  };
}

function configErrorDetail(err: unknown, fallback: string): string {
  const message = errorMessage(err, "");
  if (!message || message === fallback) {
    return fallback;
  }
  return `${fallback} ${message}`.trim();
}
