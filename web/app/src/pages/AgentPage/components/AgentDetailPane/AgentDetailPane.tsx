import { PROVIDERS, REASONING_EFFORTS, SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import {
  APIKeyField,
  CLIProxyAuthControl,
  EnvKeyValueEditor,
  isBlank,
  NotifierControls,
  profileBaseURLMissing,
  requiredFieldLabel,
} from "@/components/business/ProfileControls";
import {
  agentModelID,
  agentToDraft,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentRunning,
  isNotifierRuntimeDraftOnAgentPage,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  notifierFormIsComplete,
} from "@/models/agents";
import { AgentIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";

export function AgentDetailPane({
  item,
  t,
  activeRoom,
  busyKey,
  error,
  draft,
  models,
  modelBusy,
  saving,
  publishBusy,
  saveError,
  authStatuses,
  authBusyProvider,
  notifierWebhookOrigin,
  setNotifierWebhookOrigin,
  onDraftChange,
  onSave,
  onPublish,
  onProviderLogin,
  onStart,
  onStop,
  onRecreate,
  onDelete,
  onInvite,
  onOpenDM,
}) {
  const isManager = item.role === "manager" || item.id === "u-manager";
  const running = isAgentRunning(item);
  const draftBelongsToItem = Boolean(draft) && String(draft?.agent_id ?? "").trim() === String(item?.id ?? "").trim();
  const incomplete = isAgentIncomplete(item, draftBelongsToItem ? draft : undefined);
  const restartNeeded = isAgentRestartNeeded(item);
  const busyPrefix = `${item.id}:`;
  const provider = item.provider || item.agent_profile?.provider;
  const runtimeKind = normalizeRuntimeKind(item.runtime_kind);
  const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";
  const updateDraft = (patch) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });
  return (
    <section className="entity-pane agent-detail-pane">
      <header className="entity-header">
        <div className="entity-avatar">
          <AgentIcon />
        </div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>{item.name}</h1>
            <span className={`status-pill ${running ? "online" : ""}`}>{item.status || "unknown"}</span>
          </div>
          <p>{item.description || item.agent_profile?.description || ""}</p>
        </div>
      </header>
      <div className="entity-toolbar">
        <Button
          variant="primary"
          className="preview-action-button preview-action-button-primary"
          disabled={
            saving ||
            isBlank(draft?.name) ||
            (isNotifierRuntimeDraftOnAgentPage(draft, item)
              ? !notifierFormIsComplete(draft, item)
              : !draft?.model_id || profileBaseURLMissing(draft))
          }
          onClick={onSave}
        >
          {saving ? t("profileLoadingModels") : t("agentUpdateSave")}
        </Button>
        {SHOW_AGENT_LIFECYCLE_ACTIONS ? (
          <Button
            className="preview-action-button"
            disabled={busyKey.startsWith(busyPrefix) || incomplete}
            onClick={() => (running ? onStop(item) : onStart(item))}
          >
            {running ? t("agentStop") : t("agentStart")}
          </Button>
        ) : null}
        <Button
          className="preview-action-button"
          disabled={busyKey.startsWith(busyPrefix)}
          onClick={() => onOpenDM(item)}
        >
          {t("openDM")}
        </Button>
        <Button
          variant="outlineDanger"
          className="preview-action-button preview-action-button-danger"
          disabled={busyKey.startsWith(busyPrefix) || incomplete}
          onClick={() => onRecreate(item)}
        >
          {t("agentRecreate")}
        </Button>
        {SHOW_AGENT_LIFECYCLE_ACTIONS && activeRoom && !isManager ? (
          <Button
            className="preview-action-button"
            disabled={busyKey.startsWith(busyPrefix)}
            onClick={() => onInvite(item)}
          >
            {t("inviteToRoom")}
          </Button>
        ) : null}
        {!isManager ? (
          <Button
            variant="outlineDanger"
            className="preview-action-button preview-action-button-danger"
            disabled={busyKey.startsWith(busyPrefix)}
            onClick={() => onDelete(item)}
          >
            {t("agentDelete")}
          </Button>
        ) : null}
        {canPublish ? (
          <Button
            variant="primary"
            className="preview-action-button preview-action-button-primary entity-toolbar-publish"
            disabled={publishBusy}
            onClick={onPublish}
          >
            {publishBusy ? t("agentPublishing") : t("agentPublish")}
          </Button>
        ) : null}
      </div>
      {error ? <div className="form-error">{error}</div> : null}
      {saveError ? <div className="form-error">{saveError}</div> : null}
      {!draft ? (
        <div className="entity-grid">
          <div className="entity-field">
            <span>{t("profileRuntimeKind")}</span>
            <strong>{formatRuntimeKindLabel(item.runtime_kind, t)}</strong>
          </div>
          <div className="entity-field">
            <span>{t("profileProvider")}</span>
            <strong>{formatProviderLabel(provider)}</strong>
          </div>
          <div className="entity-field">
            <span>{t("profileModel")}</span>
            <strong>{agentModelID(item)}</strong>
          </div>
          <div className="entity-field">
            <span>{t("profileReasoning")}</span>
            <strong>{item.reasoning_effort || item.agent_profile?.reasoning_effort || "medium"}</strong>
          </div>
          <div className="entity-field">
            <span>{t("profileFastMode")}</span>
            <strong>{item.enable_fast_mode || item.agent_profile?.enable_fast_mode ? "on" : "off"}</strong>
          </div>
        </div>
      ) : null}
      <div className="entity-badge-row">
        <span className={`agent-badge ${incomplete ? "warn" : ""}`}>
          {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
        </span>
        {restartNeeded ? <span className="agent-badge warn">{t("profileRestartRequired")}</span> : null}
      </div>
      {draft ? (
        <div className="profile-editor-shell agent-page-editor">
          <section className="profile-section">
            <div className="profile-section-title">{t("profileBasics")}</div>
            <div className="profile-grid-compact">
              <label className="field">
                {requiredFieldLabel(t("agentName"))}
                <input
                  value={draft.name}
                  readOnly
                  disabled
                  required
                  aria-required="true"
                  onInput={(event) => updateDraft({ name: event.currentTarget.value })}
                  placeholder={t("agentNamePlaceholder")}
                />
              </label>
              <label className="field">
                <span>{t("profileRuntimeKind")}</span>
                <input value={draft.runtime_kind || item.runtime_kind || ""} readOnly disabled />
              </label>
              {!isNotifierRuntimeDraftOnAgentPage(draft, item) ? (
                <label className="field">
                  <span>{t("agentImage")}</span>
                  <input
                    value={draft.image}
                    readOnly
                    disabled
                    onInput={(event) => updateDraft({ image: event.currentTarget.value })}
                    placeholder={t("agentImagePlaceholder")}
                  />
                </label>
              ) : null}
              <label className="field span-2">
                <span>{t("agentDescription")}</span>
                <textarea
                  className="compact-textarea"
                  value={draft.description}
                  onInput={(event) => updateDraft({ description: event.currentTarget.value })}
                />
              </label>
            </div>
          </section>

          {!isNotifierRuntimeDraftOnAgentPage(draft, item) ? (
            <section className="profile-section">
              <div className="profile-section-title">{t("profileModelSection")}</div>
              <div className="profile-runtime-grid">
                <label className="field">
                  <span>{t("profileProvider")}</span>
                  <select
                    value={draft.provider}
                    onChange={(event) => updateDraft({ provider: event.currentTarget.value, model_id: "" })}
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
                    value={draft.model_id}
                    required
                    aria-required="true"
                    onChange={(event) => updateDraft({ model_id: event.currentTarget.value })}
                  >
                    <option value="">{modelBusy ? t("profileLoadingModels") : t("profileSelectModel")}</option>
                    {models.map((model) => (
                      <option key={model} value={model}>
                        {model}
                      </option>
                    ))}
                    {draft.model_id && !models.includes(draft.model_id) ? (
                      <option value={draft.model_id}>{draft.model_id}</option>
                    ) : null}
                  </select>
                </label>
                <label className="field">
                  <span>{t("profileReasoning")}</span>
                  <select
                    value={draft.reasoning_effort}
                    onChange={(event) => updateDraft({ reasoning_effort: event.currentTarget.value })}
                  >
                    {REASONING_EFFORTS.map((effort) => (
                      <option key={effort} value={effort}>
                        {effort}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="selection-item compact-toggle-row">
                  <input
                    type="checkbox"
                    checked={draft.enable_fast_mode}
                    onChange={() => updateDraft({ enable_fast_mode: !draft.enable_fast_mode })}
                  />
                  <span>{t("profileFastMode")}</span>
                </label>
              </div>
              <CLIProxyAuthControl
                provider={draft.provider}
                t={t}
                status={authStatuses?.[normalizeAuthProviderName(draft.provider)]}
                busy={authBusyProvider === normalizeAuthProviderName(draft.provider)}
                onLogin={onProviderLogin}
              />
            </section>
          ) : (
            <NotifierControls
              agentID={item.id}
              draft={draft}
              t={t}
              webhookOrigin={notifierWebhookOrigin}
              setWebhookOrigin={setNotifierWebhookOrigin}
              onPatch={(patch) => updateDraft(patch)}
            />
          )}

          {!isNotifierRuntimeDraftOnAgentPage(draft, item) && draft.provider === "api" ? (
            <section className="profile-section">
              <div className="profile-section-title">{t("profileAPIProvider")}</div>
              <div className="profile-api-grid">
                <label className="field">
                  {requiredFieldLabel(t("profileBaseURL"))}
                  <input
                    value={draft.base_url}
                    required
                    aria-required="true"
                    onInput={(event) => updateDraft({ base_url: event.currentTarget.value })}
                    placeholder="https://api.openai.com/v1"
                  />
                </label>
                <APIKeyField
                  value={draft.api_key}
                  onInput={(event) => updateDraft({ api_key: event.currentTarget.value })}
                  profile={draft}
                  required={!draft.api_key_set}
                  t={t}
                />
                <label className="field span-2">
                  <span>{t("profileHeaders")}</span>
                  <textarea
                    className="compact-textarea"
                    value={draft.headersText}
                    onInput={(event) => updateDraft({ headersText: event.currentTarget.value })}
                  />
                </label>
              </div>
            </section>
          ) : null}

          <section className="profile-section">
            <div className="profile-section-title">{t("profileAdvanced")}</div>
            <div className="profile-advanced-grid">
              {!isNotifierRuntimeDraftOnAgentPage(draft, item) ? (
                <label className="field">
                  <span>{t("profileRequestOptions")}</span>
                  <textarea
                    className="compact-json"
                    value={draft.requestOptionsText}
                    onInput={(event) => updateDraft({ requestOptionsText: event.currentTarget.value })}
                  />
                </label>
              ) : null}
              <div className="field">
                <span>{t("profileEnv")}</span>
                <EnvKeyValueEditor rows={draft.envRows} t={t} onChange={(rows) => updateDraft({ envRows: rows })} />
              </div>
            </div>
          </section>
        </div>
      ) : null}
    </section>
  );
}
