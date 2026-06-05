import { Check, MoreHorizontal } from "lucide-react";
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
import { WorkspaceFilePreview, WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";
import {
  agentStatusLabel,
  agentModelID,
  agentToDraft,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
  isAgentRunning,
  isNotifierRuntimeDraftOnAgentPage,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  notifierFormIsComplete,
} from "@/models/agents";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import {
  Button,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Select,
} from "@/components/ui";

export function AgentDetailPane({
  item,
  t,
  activeRoom,
  busyKey,
  error,
  draft,
  savedDraft = null,
  hasUnsavedChanges: hasUnsavedChangesProp = undefined,
  models,
  modelBusy,
  saving,
  publishBusy,
  saveError,
  authStatuses,
  authBusyProvider,
  notifierWebhookPublicOrigin,
  workspaceEntries = [],
  workspaceLoading = false,
  workspaceError = "",
  workspaceSupported = false,
  selectedWorkspacePath = "",
  workspaceFile = null,
  workspaceFileLoading = false,
  workspaceFileError = "",
  onSelectWorkspaceFile = () => {},
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
  const upgradeNeeded = isAgentUpgradeNeeded(item);
  const busyPrefix = `${item.id}:`;
  const provider = item.provider || item.agent_profile?.provider;
  const runtimeKind = normalizeRuntimeKind(item.runtime_kind);
  const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";
  const hasUnsavedChanges =
    hasUnsavedChangesProp ?? Boolean(draft && savedDraft && JSON.stringify(draft) !== JSON.stringify(savedDraft));
  const saveDisabled =
    saving ||
    isBlank(draft?.name) ||
    (isNotifierRuntimeDraftOnAgentPage(draft, item)
      ? !notifierFormIsComplete(draft, item)
      : !draft?.model_id || profileBaseURLMissing(draft));
  const updateDraft = (patch) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });
  return (
    <section className="entity-pane agent-detail-pane">
      <header className="entity-header">
        <div className="entity-avatar">
          <AgentAvatarContent
            avatar={item.avatar}
            fallback={avatarFallbackText(item.avatar, item.name, item.handle, item.id)}
          />
        </div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>{item.name}</h1>
            <span className={`status-pill ${running ? "online" : ""}`}>{agentStatusLabel(item.status, t)}</span>
            <span className={`status-pill profile-state-pill ${incomplete ? "warn" : "ready"}`}>
              {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
            </span>
            {upgradeNeeded ? (
              <span className="status-pill profile-state-pill warn">{t("profileUpgradeRequired")}</span>
            ) : null}
            {restartNeeded ? (
              <span className="status-pill profile-state-pill warn">{t("profileRestartRequired")}</span>
            ) : null}
          </div>
        </div>
        <div className="entity-toolbar">
          <Button
            variant="secondaryGray"
            size="md"
            disabled={busyKey.startsWith(busyPrefix)}
            onClick={() => onOpenDM(item)}
          >
            {t("openDM")}
          </Button>
          <AgentActionsMenu
            item={item}
            t={t}
            activeRoom={activeRoom}
            busy={busyKey.startsWith(busyPrefix)}
            incomplete={incomplete}
            isManager={isManager}
            running={running}
            upgradeNeeded={upgradeNeeded}
            canPublish={canPublish}
            publishBusy={publishBusy}
            onStart={onStart}
            onStop={onStop}
            onRecreate={onRecreate}
            onInvite={onInvite}
            onDelete={onDelete}
            onPublish={onPublish}
          />
          {draft && (hasUnsavedChanges || saving) ? (
            <Button
              variant="primary"
              size="md"
              loading={saving}
              loadingLabel={t("agentSavingChanges")}
              disabled={saveDisabled}
              onClick={onSave}
            >
              {t("agentSaveChanges")}
            </Button>
          ) : draft ? (
            <span className="agent-save-status" role="status">
              <Check aria-hidden="true" size={16} strokeWidth={2.5} />
              {t("agentSaved")}
            </span>
          ) : null}
        </div>
      </header>
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
                <label className="field span-2 agent-image-field">
                  <span>{t("agentImage")}</span>
                  <input
                    className="long-image-input"
                    value={draft.image}
                    title={draft.image}
                    readOnly
                    disabled
                    onInput={(event) => updateDraft({ image: event.currentTarget.value })}
                    placeholder={t("agentImagePlaceholder")}
                  />
                </label>
              ) : null}
              <div className="field span-2 agent-avatar-field">
                <span>{t("agentAvatar")}</span>
                <AgentAvatarPicker value={draft.avatar} t={t} onChange={(avatar) => updateDraft({ avatar })} />
              </div>
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
                  <Select
                    value={draft.provider}
                    onValueChange={(value) => updateDraft({ provider: value, model_id: "" })}
                    triggerProps={{ "aria-label": t("profileProvider") }}
                    options={PROVIDERS.map((provider) => ({
                      value: provider,
                      label: formatProviderLabel(provider),
                    }))}
                  />
                </label>
                <label className="field">
                  {requiredFieldLabel(t("profileModel"))}
                  <Select
                    value={draft.model_id}
                    required
                    onValueChange={(value) => updateDraft({ model_id: value })}
                    triggerProps={{ "aria-label": t("profileModel"), "aria-required": true }}
                    options={[
                      { value: "", label: modelBusy ? t("profileLoadingModels") : t("profileSelectModel") },
                      ...models.map((model) => ({ value: model, label: model })),
                      ...(draft.model_id && !models.includes(draft.model_id)
                        ? [{ value: draft.model_id, label: draft.model_id }]
                        : []),
                    ]}
                  />
                </label>
                <label className="field">
                  <span>{t("profileReasoning")}</span>
                  <Select
                    value={draft.reasoning_effort}
                    onValueChange={(value) => updateDraft({ reasoning_effort: value })}
                    triggerProps={{ "aria-label": t("profileReasoning") }}
                    options={REASONING_EFFORTS.map((effort) => ({ value: effort, label: effort }))}
                  />
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
              webhookPublicOrigin={notifierWebhookPublicOrigin}
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
                <label className="field profile-json-field">
                  <span>{t("profileRequestOptions")}</span>
                  <textarea
                    className="compact-json"
                    value={draft.requestOptionsText}
                    onInput={(event) => updateDraft({ requestOptionsText: event.currentTarget.value })}
                  />
                </label>
              ) : null}
              <div className="field profile-env-field">
                <span>{t("profileEnv")}</span>
                <EnvKeyValueEditor rows={draft.envRows} t={t} onChange={(rows) => updateDraft({ envRows: rows })} />
              </div>
            </div>
          </section>

          {workspaceSupported ? (
            <section className="profile-section agent-workspace-section">
              <div className="profile-section-title">{t("agentWorkspaceTitle")}</div>
              {workspaceError ? <div className="form-error">{workspaceError}</div> : null}
              <div className="agent-workspace-panels">
                <WorkspaceFileTree
                  entries={workspaceEntries}
                  loading={workspaceLoading}
                  loadingText={t("agentWorkspaceLoading")}
                  emptyText={t("agentWorkspaceEmpty")}
                  selectedPath={selectedWorkspacePath}
                  onSelectFile={onSelectWorkspaceFile}
                />
                <WorkspaceFilePreview
                  className="agent-workspace-preview"
                  file={workspaceFile}
                  loading={workspaceFileLoading}
                  error={workspaceFileError}
                  loadingText={t("agentWorkspaceFileLoading")}
                  emptyTitle={t("agentWorkspacePreviewTitle")}
                  emptyHint={t("agentWorkspacePreviewHint")}
                  binaryText={t("agentWorkspaceBinary")}
                  emptyFileText={t("agentWorkspaceEmptyFile")}
                  previewText={t("workspacePreviewPreviewTab")}
                  codeText={t("workspacePreviewCodeTab")}
                  viewToggleLabel={t("workspacePreviewViewMode")}
                  closeText={t("close")}
                  truncatedText={t("workspacePreviewTruncated")}
                />
              </div>
            </section>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function AgentActionsMenu({
  item,
  t,
  activeRoom,
  busy,
  incomplete,
  isManager,
  running,
  upgradeNeeded,
  canPublish,
  publishBusy,
  onStart,
  onStop,
  onRecreate,
  onInvite,
  onDelete,
  onPublish,
}) {
  return (
    <DropdownMenuRoot>
      <DropdownMenuTrigger asChild>
        <Button variant="secondaryGray" size="md" className="agent-actions-menu-trigger">
          <MoreHorizontal aria-hidden="true" size={18} strokeWidth={2} />
          <span>{t("agentMoreActions")}</span>
          {upgradeNeeded ? <span className="agent-actions-alert-dot" aria-hidden="true"></span> : null}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="agent-actions-menu" aria-label={t("agentMoreActions")}>
        {SHOW_AGENT_LIFECYCLE_ACTIONS ? (
          <DropdownMenuItem disabled={busy || incomplete} onSelect={() => (running ? onStop(item) : onStart(item))}>
            {running ? t("agentStop") : t("agentStart")}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem danger disabled={busy || incomplete} onSelect={() => onRecreate(item)}>
          {t("agentRecreate")}
        </DropdownMenuItem>
        {SHOW_AGENT_LIFECYCLE_ACTIONS && activeRoom && !isManager ? (
          <DropdownMenuItem disabled={busy} onSelect={() => onInvite(item)}>
            {t("inviteToRoom")}
          </DropdownMenuItem>
        ) : null}
        {canPublish ? (
          <DropdownMenuItem disabled={publishBusy} onSelect={() => onPublish?.()}>
            {publishBusy ? t("agentPublishing") : t("agentPublish")}
          </DropdownMenuItem>
        ) : null}
        {!isManager ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem danger disabled={busy} onSelect={() => onDelete(item)}>
              {t("agentDelete")}
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenuRoot>
  );
}
