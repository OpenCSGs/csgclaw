import { Button } from "@/components/ui";
import type { ConfigPhase } from "@/hooks/workspace/useConfigController";
import type { ConfigSettingsDraft } from "@/models/configSettings";
import {
  configTemplateOptions,
  configAdvertiseBaseURLPlaceholder,
  sandboxProviderLabel,
} from "@/models/configSettings";
import type { TranslateFn } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import { ModalCloseButton } from "./ModalCloseButton";

type ConfigSettingsModalProps = {
  t: TranslateFn;
  configDraft: ConfigSettingsDraft;
  hubTemplates: readonly HubTemplate[];
  sandboxProviders: string[];
  configBusy: boolean;
  configError: string;
  configPhase: ConfigPhase;
  onDraftChange: (patch: Partial<ConfigSettingsDraft>) => void;
  onClose: () => void;
  onReload: () => void;
  onSaveAndRestart: () => Promise<void>;
};

export function ConfigSettingsModal({
  t,
  configDraft,
  hubTemplates,
  sandboxProviders,
  configBusy,
  configError,
  configPhase,
  onDraftChange,
  onClose,
  onReload,
  onSaveAndRestart,
}: ConfigSettingsModalProps) {
  const restarting = configPhase === "restarting" || configPhase === "saving";
  const disabled = configBusy || configPhase === "loading";
  const statusBody =
    configPhase === "manual_restart"
      ? t("configSettingsManualRestartBody")
      : configPhase === "done"
        ? t("configSettingsDoneBody")
        : restarting
          ? t("configSettingsRestartingBody")
          : t("configSettingsHint");
  const providerOptions = sandboxProviders.length > 0 ? sandboxProviders : ["boxlite", "docker", "csghub"];
  const managerTemplateOptions = configTemplateOptions(hubTemplates, "manager", configDraft.default_manager_template);
  const managerTemplateLabel =
    managerTemplateOptions.find((option) => option.value === configDraft.default_manager_template)?.label ||
    configDraft.default_manager_template;
  const workerTemplateOptions = configTemplateOptions(hubTemplates, "worker", configDraft.default_worker_template);

  return (
    <div className="modal-backdrop">
      <div className="modal-card config-settings-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("configSettingsTitle")}</div>
            <div className="modal-subtitle">{t("configSettingsSubtitle")}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} disabled={configBusy} />
        </div>
        <div className={`upgrade-status-card ${configPhase}`}>
          <span className="upgrade-status-dot" aria-hidden="true"></span>
          <p>{statusBody}</p>
        </div>
        <div className="config-settings-form">
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsServerSection")}</h3>
            <div className="config-settings-stack">
              <label className="field">
                <span>{t("configSettingsListenPort")}</span>
                <input inputMode="numeric" value={configDraft.listen_port} disabled readOnly aria-readonly="true" />
              </label>
              <label className="field">
                <span>{t("configSettingsAdvertiseBaseURL")}</span>
                <input
                  type="url"
                  value={configDraft.advertise_base_url}
                  disabled={disabled}
                  placeholder={configAdvertiseBaseURLPlaceholder(configDraft) || undefined}
                  autoComplete="off"
                  spellCheck={false}
                  onInput={(event) => onDraftChange({ advertise_base_url: event.currentTarget.value.trim() })}
                />
              </label>
            </div>
          </section>
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsSandboxSection")}</h3>
            <label className="field">
              <span>{t("configSettingsSandboxProvider")}</span>
              <select
                value={configDraft.sandbox_provider}
                disabled={disabled}
                onChange={(event) => onDraftChange({ sandbox_provider: event.currentTarget.value })}
              >
                {providerOptions.map((provider) => (
                  <option key={provider} value={provider}>
                    {sandboxProviderLabel(provider, t)}
                  </option>
                ))}
              </select>
            </label>
          </section>
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsBootstrapSection")}</h3>
            <div className="config-settings-grid">
              <label className="field">
                <span>{t("configSettingsManagerTemplate")}</span>
                <input value={managerTemplateLabel} disabled readOnly aria-readonly="true" />
              </label>
              <label className="field">
                <span>{t("configSettingsWorkerTemplate")}</span>
                <select
                  value={configDraft.default_worker_template}
                  disabled={disabled || workerTemplateOptions.length === 0}
                  onChange={(event) => onDraftChange({ default_worker_template: event.currentTarget.value })}
                >
                  {workerTemplateOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          </section>
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsOtherSection")}</h3>
            <label className="field">
              <span>{t("configSettingsShowUpgrade")}</span>
              <input
                value={configDraft.show_upgrade ? t("configSettingsShowUpgradeOn") : t("configSettingsShowUpgradeOff")}
                disabled
                readOnly
                aria-readonly="true"
              />
            </label>
          </section>
        </div>
        {configError ? <div className="form-error">{configError}</div> : null}
        <div className="modal-actions">
          {configPhase === "done" ? (
            <Button variant="primary" size="md" onClick={onReload}>
              {t("upgradeRefresh")}
            </Button>
          ) : configPhase === "manual_restart" ? (
            <Button variant="secondaryGray" size="md" onClick={onClose}>
              {t("close")}
            </Button>
          ) : (
            <>
              <Button variant="secondaryGray" size="md" onClick={onClose} disabled={configBusy}>
                {configBusy ? t("close") : t("upgradeLater")}
              </Button>
              <Button variant="primary" size="md" disabled={disabled} onClick={() => void onSaveAndRestart()}>
                {configBusy ? t("configSettingsSaving") : t("configSettingsSaveRestart")}
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
