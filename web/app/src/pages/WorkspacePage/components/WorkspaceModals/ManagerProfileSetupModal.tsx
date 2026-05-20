import { GATEWAY_RUNTIME_KIND_OPTIONS } from "@/shared/constants/agents";
import {
  APIKeyField,
  CLIProxyAuthControl,
  EnvKeyValueEditor,
  profileBaseURLMissing,
  requiredFieldLabel,
} from "@/components/business/ProfileControls";
import { Button } from "@/components/ui";
import {
  formatProviderLabel,
  formatRuntimeKindLabel,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
} from "@/models/agents";

export function ManagerProfileSetupModal({
  t,
  managerProfile,
  profileDraft,
  onProfileDraftChange,
  onProfileModelsReset,
  bootstrapConfig,
  profileModels,
  profileModelBusy,
  authStatuses,
  authBusyProvider,
  onProviderLogin,
  profileError,
  profileBusy,
  onSave,
}) {
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
                <select
                  value={normalizeRuntimeKind(profileDraft.runtime_kind || bootstrapConfig?.runtime_kind)}
                  onChange={(event) =>
                    onProfileDraftChange({ ...profileDraft, runtime_kind: event.currentTarget.value })
                  }
                >
                  {GATEWAY_RUNTIME_KIND_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {formatRuntimeKindLabel(option.value, t)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>{t("profileProvider")}</span>
                <select
                  value={profileDraft.provider}
                  onChange={(event) => {
                    onProfileDraftChange({ ...profileDraft, provider: event.currentTarget.value, model_id: "" });
                    onProfileModelsReset();
                  }}
                >
                  {["csghub_lite", "codex", "claude_code", "api"].map((provider) => (
                    <option key={provider} value={provider}>
                      {formatProviderLabel(provider)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                {requiredFieldLabel(t("profileModel"))}
                <select
                  value={profileDraft.model_id}
                  required
                  aria-required="true"
                  onChange={(event) => onProfileDraftChange({ ...profileDraft, model_id: event.currentTarget.value })}
                >
                  <option value="">{profileModelBusy ? t("profileLoadingModels") : t("profileSelectModel")}</option>
                  {profileModels.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                  {profileDraft.model_id && !profileModels.includes(profileDraft.model_id) ? (
                    <option value={profileDraft.model_id}>{profileDraft.model_id}</option>
                  ) : null}
                </select>
              </label>
              <label className="field">
                <span>{t("profileReasoning")}</span>
                <select
                  value={profileDraft.reasoning_effort}
                  onChange={(event) =>
                    onProfileDraftChange({ ...profileDraft, reasoning_effort: event.currentTarget.value })
                  }
                >
                  {["low", "medium", "high", "xhigh"].map((effort) => (
                    <option key={effort} value={effort}>
                      {effort}
                    </option>
                  ))}
                </select>
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
            className="send-button"
            disabled={profileBusy || !profileDraft.model_id || profileBaseURLMissing(profileDraft)}
            onClick={onSave}
          >
            {profileBusy ? "..." : t("profileSave")}
          </Button>
        </div>
      </div>
    </div>
  );
}
