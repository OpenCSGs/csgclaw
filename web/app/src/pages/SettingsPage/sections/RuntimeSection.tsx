import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { AgentRuntimeSection } from "@/components/business/AgentRuntimeSection";
import { useAgentRuntimes } from "@/components/business/AgentRuntimeSection/useAgentRuntimes";
import { configTemplateOptions, sandboxProviderLabel } from "@/models/configSettings";
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
  const providerOptions = config.providers.length > 0 ? config.providers : ["boxlite", "docker", "csghub"];
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
      <SettingsRow title={t("configSettingsSandboxSection")} description={t("iaSettingsSandboxDescription")}>
        <div className={styles.formGrid}>
          <label className="field">
            <span>{t("configSettingsSandboxProvider")}</span>
            <select
              value={config.draft.sandbox_provider}
              disabled={disabled}
              onChange={(event) => config.onDraftChange({ sandbox_provider: event.currentTarget.value })}
            >
              {providerOptions.map((provider) => (
                <option key={provider} value={provider}>
                  {sandboxProviderLabel(provider, t)}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>{t("configSettingsHubLocalPath")}</span>
            <input
              value={config.draft.hub_local_path}
              disabled={disabled}
              autoComplete="off"
              spellCheck={false}
              onInput={(event) => config.onDraftChange({ hub_local_path: event.currentTarget.value })}
            />
          </label>
          <label className="field">
            <span>{t("configSettingsManagerTemplate")}</span>
            <input value={managerTemplateLabel} disabled readOnly aria-readonly="true" />
          </label>
          <label className="field">
            <span>{t("configSettingsWorkerTemplate")}</span>
            <select
              value={config.draft.default_worker_template}
              disabled={disabled || workerTemplateOptions.length === 0}
              onChange={(event) => config.onDraftChange({ default_worker_template: event.currentTarget.value })}
            >
              {workerTemplateOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
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
