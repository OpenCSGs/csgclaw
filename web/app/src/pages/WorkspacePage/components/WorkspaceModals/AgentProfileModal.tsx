import { PROVIDERS, RUNTIME_KIND_OPTIONS } from "@/shared/constants/agents";
import {
  AgentCreateProgress,
  APIKeyField,
  CLIProxyAuthControl,
  EnvKeyValueEditor,
  isBlank,
  NotifierControls,
  profileBaseURLMissing,
  requiredFieldLabel,
} from "@/components/business/ProfileControls";
import { Button } from "@/components/ui";
import {
  applyTemplateToDraft,
  ensureNotifierPullSubscriptionDraft,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isNotifierRuntimeDraftOnAgentPage,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  normalizeTemplateSelection,
  notifierFormIsComplete,
  pickDefaultAgentTemplate,
  runtimeImageForKind,
  templateMatchesRuntime,
} from "@/models/agents";

export function AgentProfileModal({
  t,
  agentModalMode,
  editingAgent,
  agentDraft,
  onAgentDraftChange,
  onAgentModelsReset,
  hubTemplates,
  bootstrapConfig,
  managerAgent,
  agentModels,
  agentModelBusy,
  authStatuses,
  authBusyProvider,
  notifierWebhookOrigin,
  setNotifierWebhookOrigin,
  onProviderLogin,
  agentError,
  agentProgress,
  agentBusy,
  onClose,
  onSave,
}) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card profile-modal agent-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">
              {agentModalMode === "create" ? t("createAgentTitle") : t("editAgentTitle")}
            </div>
            <div className="modal-subtitle">
              {agentModalMode === "create"
                ? isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent)
                  ? t("createAgentSubtitleNotifier")
                  : t("createAgentSubtitle")
                : t("editAgentSubtitle")}
            </div>
          </div>
          <Button className="modal-close" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
        <div className="profile-editor-shell">
          <section className="profile-section">
            <div className="profile-section-title">{t("profileBasics")}</div>
            <div className="profile-grid profile-grid-compact">
              {agentModalMode === "create" ? (
                <label className="field span-2">
                  <span>{t("templateLabel")}</span>
                  <select
                    value={agentDraft.from_template || ""}
                    onChange={(event) => {
                      const nextTemplate = normalizeTemplateSelection(
                        hubTemplates.find((item) => item.id === event.currentTarget.value) || null,
                      );
                      onAgentDraftChange((current) =>
                        applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || ""),
                      );
                    }}
                  >
                    <option value="">{t("templateNone")}</option>
                    {hubTemplates.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name || item.id}
                      </option>
                    ))}
                  </select>
                </label>
              ) : null}
              <label className="field">
                {requiredFieldLabel(t("agentName"))}
                <input
                  value={agentDraft.name}
                  readOnly={agentModalMode === "edit"}
                  disabled={agentModalMode === "edit"}
                  required
                  aria-required="true"
                  onInput={(event) => onAgentDraftChange({ ...agentDraft, name: event.currentTarget.value })}
                  placeholder={t("agentNamePlaceholder")}
                />
              </label>
              {agentModalMode === "create" ? (
                <label className="field">
                  <span>{t("roleLabel")}</span>
                  <input value={agentDraft.role || "worker"} readOnly disabled />
                </label>
              ) : null}
              <label className="field">
                <span>{t("profileRuntimeKind")}</span>
                {agentModalMode === "create" ? (
                  <select
                    value={normalizeRuntimeKind(agentDraft.runtime_kind) || "picoclaw_sandbox"}
                    onChange={(event) => {
                      const runtimeKind = normalizeRuntimeKind(event.currentTarget.value);
                      const currentTemplate = normalizeTemplateSelection(
                        hubTemplates.find((item) => item.id === agentDraft.from_template) || null,
                      );
                      const nextTemplate = templateMatchesRuntime(currentTemplate, runtimeKind)
                        ? currentTemplate
                        : pickDefaultAgentTemplate(hubTemplates, runtimeKind, bootstrapConfig);
                      let nextDraft = {
                        ...agentDraft,
                        role: "worker",
                        runtime_kind: runtimeKind,
                        image: runtimeImageForKind(
                          runtimeKind,
                          bootstrapConfig,
                          agentDraft.default_image || managerAgent?.image || "",
                        ),
                      };
                      nextDraft = applyTemplateToDraft(
                        nextDraft,
                        nextTemplate,
                        bootstrapConfig,
                        managerAgent?.image || "",
                      );
                      if (runtimeKind === "notifier") {
                        nextDraft = ensureNotifierPullSubscriptionDraft({
                          ...nextDraft,
                          from_template: "",
                          image: "",
                          notifier_delivery_mode: nextDraft.notifier_delivery_mode || "webhook",
                          template_name: "",
                        });
                      }
                      onAgentDraftChange(nextDraft);
                    }}
                  >
                    {RUNTIME_KIND_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>
                        {formatRuntimeKindLabel(option.value, t)}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input value={agentDraft.runtime_kind || editingAgent?.runtime_kind || ""} readOnly disabled />
                )}
              </label>
              {!isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent) ? (
                <label className="field">
                  <span>{t("agentImage")}</span>
                  <input
                    value={agentDraft.image}
                    readOnly={agentModalMode === "edit"}
                    disabled={agentModalMode === "edit"}
                    onInput={(event) => onAgentDraftChange({ ...agentDraft, image: event.currentTarget.value })}
                    placeholder={t("agentImagePlaceholder")}
                  />
                </label>
              ) : null}
              <label className="field span-2">
                <span>{t("agentDescription")}</span>
                <textarea
                  className="compact-textarea"
                  value={agentDraft.description}
                  onInput={(event) => onAgentDraftChange({ ...agentDraft, description: event.currentTarget.value })}
                />
              </label>
            </div>
          </section>
          {!isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent) ? (
            <section className="profile-section">
              <div className="profile-section-title">{t("profileModelSection")}</div>
              <div className="profile-runtime-grid">
                <label className="field">
                  <span>{t("profileProvider")}</span>
                  <select
                    value={agentDraft.provider}
                    onChange={(event) => {
                      onAgentDraftChange({ ...agentDraft, provider: event.currentTarget.value, model_id: "" });
                      onAgentModelsReset();
                    }}
                  >
                    {PROVIDERS.map((provider) => (
                      <option key={provider} value={provider}>
                        {formatProviderLabel(provider)}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="field">
                  {requiredFieldLabel(t("profileModel"))}
                  <select
                    value={agentDraft.model_id}
                    required
                    aria-required="true"
                    onChange={(event) => onAgentDraftChange({ ...agentDraft, model_id: event.currentTarget.value })}
                  >
                    <option value="">{agentModelBusy ? t("profileLoadingModels") : t("profileSelectModel")}</option>
                    {agentModels.map((model) => (
                      <option key={model} value={model}>
                        {model}
                      </option>
                    ))}
                    {agentDraft.model_id && !agentModels.includes(agentDraft.model_id) ? (
                      <option value={agentDraft.model_id}>{agentDraft.model_id}</option>
                    ) : null}
                  </select>
                </label>
                <label className="field">
                  <span>{t("profileReasoning")}</span>
                  <select
                    value={agentDraft.reasoning_effort}
                    onChange={(event) =>
                      onAgentDraftChange({ ...agentDraft, reasoning_effort: event.currentTarget.value })
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
                    checked={agentDraft.enable_fast_mode}
                    onChange={() =>
                      onAgentDraftChange({ ...agentDraft, enable_fast_mode: !agentDraft.enable_fast_mode })
                    }
                  />
                  <span>{t("profileFastMode")}</span>
                </label>
              </div>
              <CLIProxyAuthControl
                provider={agentDraft.provider}
                t={t}
                status={authStatuses[normalizeAuthProviderName(agentDraft.provider)]}
                busy={authBusyProvider === normalizeAuthProviderName(agentDraft.provider)}
                onLogin={onProviderLogin}
              />
            </section>
          ) : (
            <NotifierControls
              agentID={agentModalMode === "edit" ? editingAgent?.id : ""}
              draft={agentDraft}
              t={t}
              webhookOrigin={notifierWebhookOrigin}
              setWebhookOrigin={setNotifierWebhookOrigin}
              onPatch={(patch) => onAgentDraftChange({ ...agentDraft, ...patch })}
            />
          )}
          {!isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent) && agentDraft.provider === "api" ? (
            <section className="profile-section">
              <div className="profile-section-title">{t("profileAPIProvider")}</div>
              <div className="profile-api-grid">
                <label className="field">
                  {requiredFieldLabel(t("profileBaseURL"))}
                  <input
                    value={agentDraft.base_url}
                    required
                    aria-required="true"
                    onInput={(event) => onAgentDraftChange({ ...agentDraft, base_url: event.currentTarget.value })}
                    placeholder="https://api.openai.com/v1"
                  />
                </label>
                <APIKeyField
                  value={agentDraft.api_key}
                  onInput={(event) => onAgentDraftChange({ ...agentDraft, api_key: event.currentTarget.value })}
                  profile={agentDraft}
                  required={!agentDraft.api_key_set}
                  t={t}
                />
                <label className="field span-2">
                  <span>{t("profileHeaders")}</span>
                  <textarea
                    className="compact-textarea"
                    value={agentDraft.headersText}
                    onInput={(event) => onAgentDraftChange({ ...agentDraft, headersText: event.currentTarget.value })}
                  />
                </label>
              </div>
            </section>
          ) : null}
          <section className="profile-section">
            <div className="profile-section-title">{t("profileAdvanced")}</div>
            <div className="profile-advanced-grid">
              {!isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent) ? (
                <label className="field">
                  <span>{t("profileRequestOptions")}</span>
                  <textarea
                    className="compact-json"
                    value={agentDraft.requestOptionsText}
                    onInput={(event) =>
                      onAgentDraftChange({ ...agentDraft, requestOptionsText: event.currentTarget.value })
                    }
                  />
                </label>
              ) : null}
              <div className="field">
                <span>{t("profileEnv")}</span>
                <EnvKeyValueEditor
                  rows={agentDraft.envRows}
                  t={t}
                  onChange={(rows) => onAgentDraftChange({ ...agentDraft, envRows: rows })}
                />
              </div>
            </div>
          </section>
        </div>
        {agentError ? <div className="form-error">{agentError}</div> : null}
        <AgentCreateProgress progress={agentProgress} t={t} />
        <div className="modal-actions">
          <Button className="secondary-button" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            className="send-button"
            disabled={
              agentBusy ||
              isBlank(agentDraft.name) ||
              (isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent)
                ? !notifierFormIsComplete(agentDraft, editingAgent)
                : !agentDraft.model_id || profileBaseURLMissing(agentDraft))
            }
            onClick={onSave}
          >
            {agentBusy ? "..." : agentModalMode === "create" ? t("agentCreateSave") : t("agentUpdateSave")}
          </Button>
        </div>
      </div>
    </div>
  );
}
