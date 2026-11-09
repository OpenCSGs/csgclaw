import { Field, TextInput, Select, Button } from "@/components/ui";
import type { ConfigPhase } from "@/hooks/workspace/useConfigController";
import type { ConfigSettingsDraft } from "@/models/configSettings";
import { configTemplateOptions, configAdvertiseBaseURLPlaceholder } from "@/models/configSettings";
import type { TranslateFn } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import { ModalCloseButton } from "./ModalCloseButton";

type ConfigSettingsModalProps = {
  t: TranslateFn;
  configDraft: ConfigSettingsDraft;
  hubTemplates: readonly HubTemplate[];
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
              <Field label={t("configSettingsListenPort")}>
                <TextInput inputMode="numeric" value={configDraft.listen_port} disabled readOnly aria-readonly="true" />
              </Field>
              <Field label={t("configSettingsAdvertiseBaseURL")}>
                <TextInput
                  type="url"
                  value={configDraft.advertise_base_url}
                  disabled={disabled}
                  placeholder={configAdvertiseBaseURLPlaceholder(configDraft) || undefined}
                  autoComplete="off"
                  spellCheck={false}
                  onInput={(event) => onDraftChange({ advertise_base_url: event.currentTarget.value.trim() })}
                />
              </Field>
            </div>
          </section>
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsHubSection")}</h3>
            <Field label={t("configSettingsHubLocalPath")}>
              <TextInput
                value={configDraft.hub_local_path}
                disabled={disabled}
                autoComplete="off"
                spellCheck={false}
                onInput={(event) => onDraftChange({ hub_local_path: event.currentTarget.value })}
              />
            </Field>
          </section>
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsBootstrapSection")}</h3>
            <div className="config-settings-grid">
              <Field label={t("configSettingsManagerTemplate")}>
                <TextInput value={managerTemplateLabel} disabled readOnly aria-readonly="true" />
              </Field>
              <Field label={t("configSettingsWorkerTemplate")}>
                <Select
                  value={configDraft.default_worker_template}
                  disabled={disabled || workerTemplateOptions.length === 0}
                  options={workerTemplateOptions}
                  onValueChange={(default_worker_template) => onDraftChange({ default_worker_template })}
                />
              </Field>
            </div>
          </section>
          <section className="config-settings-section">
            <h3 className="config-settings-section-title">{t("configSettingsOtherSection")}</h3>
            <Field label={t("configSettingsShowUpgrade")}>
              <TextInput
                value={configDraft.show_upgrade ? t("configSettingsShowUpgradeOn") : t("configSettingsShowUpgradeOff")}
                disabled
                readOnly
                aria-readonly="true"
              />
            </Field>
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
