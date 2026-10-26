import { AgentModelFields, type AgentModelFieldsProps } from "@/components/business/AgentModelFields";
import { AgentMediaModelFields } from "@/components/business/AgentMediaModelFields";
import { ReasoningControls } from "@/components/business/ProfileControls";
import { modelReasoningEfforts } from "@/models/modelProviders";
import type { AgentDraft } from "@/models/agents";
import styles from "./AgentModelConfiguration.module.css";

type AgentModelConfigurationProps = AgentModelFieldsProps & {
  draft: AgentDraft;
};

export function AgentModelConfiguration({ draft, ...props }: AgentModelConfigurationProps) {
  const { providerOptions, onPatch, busy, t } = props;
  return (
    <div className={`${styles.root} agent-model-config-grid`}>
      <fieldset>
        <legend>{t("profileConversationModelGroup")}</legend>
        <div className={styles.fields}>
          <AgentModelFields
            {...props}
            onPatch={(patch) => {
              const changedModel =
                (patch.model_id !== undefined && patch.model_id !== props.modelID) ||
                (patch.model_provider_id !== undefined && patch.model_provider_id !== props.providerID);
              onPatch(changedModel ? { ...patch, reasoning_effort: "auto" } : patch);
            }}
          />
          <ReasoningControls
            value={draft.reasoning_effort}
            supportedEfforts={modelReasoningEfforts(providerOptions, props.providerID, props.modelID)}
            onChange={(value) => onPatch({ reasoning_effort: value })}
            t={t}
          />
          <div className="field agent-fast-mode-field">
            <span>{t("profileFastMode")}</span>
            <label className="selection-item compact-toggle-row agent-fast-mode-toggle">
              <input
                type="checkbox"
                checked={draft.enable_fast_mode}
                aria-label={t("profileFastMode")}
                onChange={() => onPatch({ enable_fast_mode: !draft.enable_fast_mode })}
              />
              <small className="agent-fast-mode-help">{t("profileFastModeHelp")}</small>
            </label>
          </div>
        </div>
        <p className={styles.hint}>{t("profileConversationModelSettingsHelp")}</p>
      </fieldset>
      <AgentMediaModelFields draft={draft} providerOptions={providerOptions} updateDraft={onPatch} busy={busy} t={t} />
    </div>
  );
}
