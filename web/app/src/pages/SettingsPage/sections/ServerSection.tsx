import { Field, TextInput, Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

/** Shares configuration with RuntimeSection; saving persists both and restarts the server. */
export function ServerSection() {
  const controller = useWorkspaceControllerContext();
  const config = controller.configSettings;
  const t = controller.t;
  if (!config) {
    return null;
  }
  const disabled = config.loading || config.phase === "saving" || config.phase === "restarting";

  return (
    <SettingsRow title={t("configSettingsServerSection")} description={t("iaSettingsServerDescription")}>
      <div className={styles.formGrid}>
        <Field label={t("configSettingsListenPort")}>
          <TextInput inputMode="numeric" value={config.draft.listen_port} disabled readOnly aria-readonly="true" />
        </Field>
        <Field label={t("configSettingsAdvertiseBaseURL")}>
          <TextInput
            type="url"
            value={config.draft.advertise_base_url}
            disabled={disabled}
            placeholder={config.draft.advertise_base_url_effective || undefined}
            autoComplete="off"
            spellCheck={false}
            onInput={(event) => config.onDraftChange({ advertise_base_url: event.currentTarget.value.trim() })}
          />
        </Field>
        <Field label={t("configSettingsAccessToken")}>
          <TextInput
            type="password"
            placeholder={config.draft.access_token_preview}
            value={config.draft.access_token}
            disabled={disabled}
            autoComplete="off"
            spellCheck={false}
            onInput={(event) => config.onDraftChange({ access_token: event.currentTarget.value })}
          />
        </Field>
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
  );
}
