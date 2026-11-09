import { TextInput, Field, Button, Select } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { AgentRuntimeSection } from "@/components/business/AgentRuntimeSection";
import { useAgentRuntimes } from "@/components/business/AgentRuntimeSection/useAgentRuntimes";
import { configTemplateOptions } from "@/models/configSettings";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function RuntimeSection() {
  const controller = useWorkspaceControllerContext();
  const config = controller.configSettings;
  const t = controller.t;
  const agentRuntimes = useAgentRuntimes(t);
  if (!config) {
    return null;
  }

  const disabled = config.loading || config.phase === "saving" || config.phase === "restarting";
  const managerTemplateOptions = configTemplateOptions(
    config.hubTemplates,
    "manager",
    config.draft.default_manager_template,
  );
  const managerTemplateLabel =
    managerTemplateOptions.find((option) => option.value === config.draft.default_manager_template)?.label ||
    config.draft.default_manager_template;
  const workerTemplateOptions = configTemplateOptions(
    config.hubTemplates,
    "worker",
    config.draft.default_worker_template,
  );

  return (
    <>
      <SettingsRow title={t("profileRuntimeSection")} description={t("iaSettingsRuntimeDescription")}>
        <div className={styles.formGrid}>
          <label className={`field ${styles.spanAll}`}>
            <span>{t("configSettingsHubLocalPath")}</span>
            <TextInput
              value={config.draft.hub_local_path}
              disabled={disabled}
              autoComplete="off"
              spellCheck={false}
              onInput={(event) => config.onDraftChange({ hub_local_path: event.currentTarget.value })}
            />
          </label>
          <Field label={t("configSettingsManagerTemplate")}>
            <TextInput value={managerTemplateLabel} disabled readOnly aria-readonly="true" />
          </Field>
          <label className="field">
            <span>{t("configSettingsWorkerTemplate")}</span>
            <Select
              options={workerTemplateOptions}
              triggerProps={{
                "aria-label": t("configSettingsWorkerTemplate"),
                disabled: disabled || workerTemplateOptions.length === 0,
              }}
              value={config.draft.default_worker_template}
              onValueChange={(value) => config.onDraftChange({ default_worker_template: value })}
            />
          </label>
          {config.error ? <p className="form-error">{config.error}</p> : null}
          <div className={`${styles.formActions} ${styles.spanAll}`}>
            {config.phase === "done" ? (
              <Button variant="primary" size="md" onClick={config.onReload}>
                {t("upgradeRefresh")}
              </Button>
            ) : (
              <Button
                variant="primary"
                size="md"
                disabled={disabled}
                loading={config.loading}
                loadingLabel={t("configSettingsSaving")}
                onClick={() => void config.onSaveAndRestart()}
              >
                {t("configSettingsSaveRestart")}
              </Button>
            )}
          </div>
        </div>
      </SettingsRow>

      <SettingsRow title={t("iaSettingsRuntimeSectionTitle")} description={t("iaSettingsRuntimeSectionHint")}>
        <AgentRuntimeSection
          error={agentRuntimes.error}
          installError={agentRuntimes.installError}
          installProgress={agentRuntimes.installProgress}
          installingRuntime={agentRuntimes.installingRuntime}
          loading={agentRuntimes.loading}
          onInstallRuntime={agentRuntimes.install}
          onRetryLoad={agentRuntimes.refresh}
          refreshing={agentRuntimes.refreshing}
          runtimes={agentRuntimes.runtimes}
          t={t}
        />
      </SettingsRow>
    </>
  );
}
