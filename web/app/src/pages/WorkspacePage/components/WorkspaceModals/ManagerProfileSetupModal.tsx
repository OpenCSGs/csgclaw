import { GATEWAY_RUNTIME_KIND_OPTIONS } from "@/shared/constants/agents";
import {
  APIKeyField,
  CLIProxyAuthControl,
  EnvKeyValueEditor,
  profileBaseURLMissing,
  requiredFieldLabel,
} from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import {
  formatProviderLabel,
  formatRuntimeKindLabel,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
} from "@/models/agents";
import type { AgentDraft, AgentProfileLike, RuntimeBootstrapConfig } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";

type ManagerProfileDetectionResult = {
  error?: string | null;
  model_id?: string | null;
  provider?: string | null;
  status?: string | null;
};

type ManagerProfileLike = AgentProfileLike & {
  detection_results?: ManagerProfileDetectionResult[] | null;
};

type VoidOrPromise = void | Promise<void>;

export type ManagerProfileSetupModalProps = {
  authBusyProvider?: string;
  authStatuses?: CLIProxyAuthStatusMap;
  bootstrapConfig?: RuntimeBootstrapConfig | null;
  managerProfile?: ManagerProfileLike | null;
  onProfileDraftChange: (draft: AgentDraft) => void;
  onProfileModelsReset: () => void;
  onProviderLogin?: (provider: string) => VoidOrPromise;
  onSave: () => VoidOrPromise;
  profileBusy?: boolean;
  profileDraft: AgentDraft;
  profileError?: string;
  profileModelBusy?: boolean;
  profileModels?: string[];
  t: TranslateFn;
};

export function ManagerProfileSetupModal({
  t,
  managerProfile,
  profileDraft,
  onProfileDraftChange,
  onProfileModelsReset,
  bootstrapConfig = null,
  profileModels = [],
  profileModelBusy = false,
  authStatuses = {},
  authBusyProvider = "",
  onProviderLogin,
  profileError = "",
  profileBusy = false,
  onSave,
}: ManagerProfileSetupModalProps) {
  return (
    <div className="modal-backdrop profile-backdrop nonblocking">
      <div className="modal-card profile-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("profileSetupTitle")}</div>
            <div className="modal-subtitle">{t("profileSetupSubtitle")}</div>
          </div>
        </div>
        {managerProfile?.detection_results?.length ? (
          <div className="detection-list">
            <div className="section-label">{t("detectionResults")}</div>
            {managerProfile.detection_results.map((item) => (
              <div key={item.provider} className={`detection-row ${item.status === "ok" ? "ok" : "failed"}`}>
                <span>{formatProviderLabel(item.provider)}</span>
                <small>{item.status === "ok" ? item.model_id : item.error}</small>
              </div>
            ))}
          </div>
        ) : null}
        <div className="profile-editor-shell">
          <section className="profile-section">
            <div className="profile-section-title">{t("profileModelSection")}</div>
            <div className="profile-runtime-grid">
              <label className="field">
                <span>{t("profileRuntimeKind")}</span>
                <Select
                  value={normalizeRuntimeKind(profileDraft.runtime_kind || bootstrapConfig?.runtime_kind)}
                  onValueChange={(value) => onProfileDraftChange({ ...profileDraft, runtime_kind: value })}
                  triggerProps={{ "aria-label": t("profileRuntimeKind") }}
                  options={GATEWAY_RUNTIME_KIND_OPTIONS.map((option) => ({
                    value: option.value,
                    label: formatRuntimeKindLabel(option.value, t),
                  }))}
                />
              </label>
              <label className="field">
                <span>{t("profileProvider")}</span>
                <Select
                  value={profileDraft.provider}
                  onValueChange={(value) => {
                    onProfileDraftChange({ ...profileDraft, provider: value, model_id: "" });
                    onProfileModelsReset();
                  }}
                  triggerProps={{ "aria-label": t("profileProvider") }}
                  options={["csghub_lite", "codex", "claude_code", "api"].map((provider) => ({
                    value: provider,
                    label: formatProviderLabel(provider),
                  }))}
                />
              </label>
              <label className="field">
                {requiredFieldLabel(t("profileModel"))}
                <Select
                  value={profileDraft.model_id}
                  required
                  onValueChange={(value) => onProfileDraftChange({ ...profileDraft, model_id: value })}
                  triggerProps={{ "aria-label": t("profileModel"), "aria-required": true }}
                  options={[
                    { value: "", label: profileModelBusy ? t("profileLoadingModels") : t("profileSelectModel") },
                    ...profileModels.map((model) => ({ value: model, label: model })),
                    ...(profileDraft.model_id && !profileModels.includes(profileDraft.model_id)
                      ? [{ value: profileDraft.model_id, label: profileDraft.model_id }]
                      : []),
                  ]}
                />
              </label>
              <label className="field">
                <span>{t("profileReasoning")}</span>
                <Select
                  value={profileDraft.reasoning_effort}
                  onValueChange={(value) => onProfileDraftChange({ ...profileDraft, reasoning_effort: value })}
                  triggerProps={{ "aria-label": t("profileReasoning") }}
                  options={["low", "medium", "high", "xhigh"].map((effort) => ({
                    value: effort,
                    label: effort,
                  }))}
                />
              </label>
              <label className="selection-item compact-toggle-row">
                <input
                  type="checkbox"
                  checked={profileDraft.enable_fast_mode}
                  onChange={() =>
                    onProfileDraftChange({ ...profileDraft, enable_fast_mode: !profileDraft.enable_fast_mode })
                  }
                />
                <span>{t("profileFastMode")}</span>
              </label>
            </div>
            <CLIProxyAuthControl
              provider={profileDraft.provider}
              t={t}
              status={authStatuses[normalizeAuthProviderName(profileDraft.provider)]}
              busy={authBusyProvider === normalizeAuthProviderName(profileDraft.provider)}
              onLogin={onProviderLogin}
            />
          </section>
          {profileDraft.provider === "api" ? (
            <section className="profile-section">
              <div className="profile-section-title">{t("profileAPIProvider")}</div>
              <div className="profile-api-grid">
                <label className="field">
                  {requiredFieldLabel(t("profileBaseURL"))}
                  <input
                    value={profileDraft.base_url}
                    required
                    aria-required="true"
                    onInput={(event) => onProfileDraftChange({ ...profileDraft, base_url: event.currentTarget.value })}
                    placeholder="https://api.openai.com/v1"
                  />
                </label>
                <APIKeyField
                  value={profileDraft.api_key}
                  onInput={(event) => onProfileDraftChange({ ...profileDraft, api_key: event.currentTarget.value })}
                  profile={profileDraft}
                  required={!profileDraft.api_key_set}
                  t={t}
                />
                <label className="field span-2">
                  <span>{t("profileHeaders")}</span>
                  <textarea
                    className="compact-textarea"
                    value={profileDraft.headersText}
                    onInput={(event) =>
                      onProfileDraftChange({ ...profileDraft, headersText: event.currentTarget.value })
                    }
                  />
                </label>
              </div>
            </section>
          ) : null}
          <section className="profile-section">
            <div className="profile-section-title">{t("profileAdvanced")}</div>
            <div className="profile-advanced-grid">
              <label className="field">
                <span>{t("profileRequestOptions")}</span>
                <textarea
                  className="compact-json"
                  value={profileDraft.requestOptionsText}
                  onInput={(event) =>
                    onProfileDraftChange({ ...profileDraft, requestOptionsText: event.currentTarget.value })
                  }
                />
              </label>
              <div className="field">
                <span>{t("profileEnv")}</span>
                <EnvKeyValueEditor
                  rows={profileDraft.envRows}
                  t={t}
                  onChange={(rows) => onProfileDraftChange({ ...profileDraft, envRows: rows })}
                />
              </div>
            </div>
          </section>
        </div>
        {profileError ? <div className="form-error">{profileError}</div> : null}
        <div className="modal-actions">
          <Button
            variant="primary"
            size="md"
            disabled={profileBusy || !profileDraft.model_id || profileBaseURLMissing(profileDraft)}
            loading={profileBusy}
            onClick={onSave}
          >
            {t("profileSave")}
          </Button>
        </div>
      </div>
    </div>
  );
}
