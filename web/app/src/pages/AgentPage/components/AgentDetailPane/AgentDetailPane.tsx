import {
  Check,
  CheckCircle2,
  CircleDashed,
  Edit3,
  ExternalLink,
  Link2,
  MoreHorizontal,
  RefreshCw,
  Unlink2,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "@/api/client";
import { PROVIDERS, REASONING_EFFORTS, SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import {
  APIKeyField,
  CLIProxyAuthControl,
  EnvKeyValueEditor,
  NotifierControls,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import {
  agentProfilePageSaveDisabled,
  agentStatusLabel,
  agentModelID,
  agentToDraft,
  formatProviderLabel,
  formatRuntimeKindLabel,
  hasConnectedAgentChannel,
  isAgentIncomplete,
  isNotificationBotAgent,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
  isAgentRunning,
  isNotifierRuntimeDraftOnAgentPage,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  runtimeOptionSchemasForAgent,
} from "@/models/agents";
import type { AgentDraft, AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { LocaleCode } from "@/models/conversations";
import type { SlashSkillOption } from "@/models/slashCommands";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
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

type VoidOrPromise = void | Promise<void>;
type AgentActionHandler = (item: AgentLike) => VoidOrPromise;
type AgentNoticeTone = "info" | "warning" | "success";

type FeishuPendingRegistrationView = {
  connect_url?: string;
  expires_at?: string;
  next_poll_seconds?: number;
  registration_id?: string;
  status?: string;
  user_code?: string;
} | null;

export type AgentDetailPaneProps = {
  activeRoom?: IMConversation | null;
  authBusyProvider?: string;
  authStatuses?: CLIProxyAuthStatusMap;
  busyKey?: string;
  draft?: AgentDraft | null;
  error?: string;
  feishuConnectBusy?: string;
  feishuPendingRegistration?: FeishuPendingRegistrationView;
  hasUnsavedChanges?: boolean;
  item: AgentLike;
  modelBusy?: boolean;
  modelError?: unknown;
  models?: string[];
  notice?: string;
  noticeTone?: AgentNoticeTone;
  notifierWebhookPublicOrigin?: string;
  onDelete: AgentActionHandler;
  onDraftChange?: (draft: AgentDraft) => void;
  onInvite: AgentActionHandler;
  onOpenDM: AgentActionHandler;
  onProviderLogin?: (provider: string) => VoidOrPromise;
  onPublish?: () => VoidOrPromise;
  onRecreate: AgentActionHandler;
  onSave?: () => VoidOrPromise;
  onStart: AgentActionHandler;
  onStartFeishuConnect?: AgentActionHandler;
  onStop: AgentActionHandler;
  onFinalizeFeishuConnect?: AgentActionHandler;
  onDisconnectFeishu?: AgentActionHandler;
  onUpgrade?: AgentActionHandler;
  publishBusy?: boolean;
  locale?: LocaleCode;
  saveError?: string;
  savedDraft?: AgentDraft | null;
  saving?: boolean;
  skills?: SlashSkillOption[];
  skillsError?: string;
  skillsLoading?: boolean;
  t: TranslateFn;
  workspaceSupported?: boolean;
};

export function AgentDetailPane({
  item,
  t,
  activeRoom = null,
  busyKey = "",
  error = "",
  feishuConnectBusy = "",
  feishuPendingRegistration = null,
  draft,
  savedDraft = null,
  hasUnsavedChanges: hasUnsavedChangesProp = undefined,
  models = [],
  notice = "",
  noticeTone = "warning",
  modelBusy = false,
  modelError = null,
  saving = false,
  publishBusy = false,
  saveError = "",
  authStatuses = {},
  authBusyProvider = "",
  locale = "en",
  notifierWebhookPublicOrigin = "",
  skills = [],
  skillsLoading = false,
  skillsError = "",
  workspaceSupported = false,
  onDraftChange,
  onSave,
  onPublish,
  onProviderLogin,
  onStart,
  onStop,
  onRecreate,
  onStartFeishuConnect,
  onDisconnectFeishu,
  onUpgrade,
  onDelete,
  onInvite,
  onOpenDM,
}: AgentDetailPaneProps) {
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const descriptionInputRef = useRef<HTMLTextAreaElement | null>(null);
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
  const saveDisabled = agentProfilePageSaveDisabled(draft, item, { saving, savedDraft });
  const updateDraft = (patch: Partial<AgentDraft>) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });
  const runtimeOptionSchemas = runtimeOptionSchemasForAgent(draft?.runtime_kind || item.runtime_kind, item);

  useEffect(() => {
    if (!draft) {
      setIsEditingDescription(false);
    }
  }, [draft]);

  useEffect(() => {
    if (!isEditingDescription) {
      return;
    }
    descriptionInputRef.current?.focus();
  }, [isEditingDescription]);

  return (
    <section className="entity-pane agent-detail-pane">
      <header className="entity-header">
        {draft ? (
          <div className="entity-avatar agent-header-avatar-picker">
            <AgentAvatarPicker
              value={draft.avatar || item.avatar}
              t={t}
              mode="edit"
              onChange={(avatar) => updateDraft({ avatar })}
            />
          </div>
        ) : (
          <div className="entity-avatar">
            <AgentAvatarContent
              avatar={item.avatar}
              fallback={avatarFallbackText(item.avatar, item.name, item.handle, item.id)}
            />
          </div>
        )}
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>{item.name}</h1>
            <span className={`agent-status-dot ${running ? "online" : ""}`} aria-hidden="true"></span>
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
          {draft ? (
            isEditingDescription ? (
              <label className="field entity-description-field">
                <span className="sr-only">{t("agentDescription")}</span>
                <textarea
                  ref={descriptionInputRef}
                  className="compact-textarea"
                  value={draft.description}
                  onBlur={() => setIsEditingDescription(false)}
                  onInput={(event) => updateDraft({ description: event.currentTarget.value })}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") {
                      event.preventDefault();
                      event.currentTarget.blur();
                    }
                  }}
                  placeholder={t("agentDescription")}
                />
              </label>
            ) : (
              <button
                type="button"
                className={`entity-description-display ${draft.description ? "" : "is-empty"}`.trim()}
                aria-label={t("editDescription")}
                onClick={() => setIsEditingDescription(true)}
              >
                <span className="entity-description-display-copy">{draft.description || t("agentDescription")}</span>
                <span className="entity-description-display-icon" aria-hidden="true">
                  <Edit3 size={16} strokeWidth={1.8} />
                </span>
              </button>
            )
          ) : item.description ? (
            <div className="entity-description-text">{item.description}</div>
          ) : null}
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
          {onUpgrade ? (
            <Button
              variant="primary"
              size="md"
              disabled={busyKey.startsWith(busyPrefix) || incomplete}
              onClick={() => onUpgrade(item)}
            >
              {t("agentUpgrade")}
            </Button>
          ) : null}
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
          ) : draft && incomplete ? (
            <span className="agent-save-status warn" role="status">
              {t("agentProfileSetupRequired")}
            </span>
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
      {notice ? (
        <div className={`form-warning ${noticeTone === "warning" ? "" : noticeTone}`.trim()} role="status">
          {notice}
        </div>
      ) : null}
      {!isNotificationBotAgent(item) ? (
        <AgentChannelsSection
          item={item}
          t={t}
          busyKey={feishuConnectBusy.startsWith(`${item.id}:`) ? feishuConnectBusy : ""}
          pendingRegistration={feishuPendingRegistration}
          onStartFeishuConnect={onStartFeishuConnect}
          onDisconnectFeishu={onDisconnectFeishu}
        />
      ) : null}
      {!draft ? (
        <>
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
          <section className="profile-section agent-instructions-section">
            <div className="profile-section-title">{t("agentInstructions")}</div>
            <div className="agent-instructions-body">{item.instructions || "-"}</div>
          </section>
        </>
      ) : null}
      {draft ? (
        <div className="profile-editor-shell agent-page-editor">
          <section className="profile-section">
            <div className="profile-section-title">{t("profileBasics")}</div>
            <div className="profile-grid-compact">
              {!isNotifierRuntimeDraftOnAgentPage(draft, item) ? (
                <div className="agent-runtime-image-row span-2">
                  <label className="field">
                    <span>{t("profileRuntimeKind")}</span>
                    <input value={draft.runtime_kind || item.runtime_kind || ""} readOnly disabled />
                  </label>
                  <label className="field agent-image-field">
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
                </div>
              ) : (
                <label className="field">
                  <span>{t("profileRuntimeKind")}</span>
                  <input value={draft.runtime_kind || item.runtime_kind || ""} readOnly disabled />
                </label>
              )}
              {!isNotifierRuntimeDraftOnAgentPage(draft, item) && runtimeOptionSchemas.length > 0 ? (
                <RuntimeOptionsFields
                  draft={draft}
                  locale={locale}
                  schemas={runtimeOptionSchemas}
                  onDraftChange={onDraftChange || (() => {})}
                  embedded
                />
              ) : null}
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
                  {modelError ? (
                    <span className="field-hint error">{errorMessage(modelError, t("modelLoadFailed"))}</span>
                  ) : null}
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
              agentID={item.id || ""}
              draft={draft}
              t={t}
              webhookPublicOrigin={notifierWebhookPublicOrigin}
              onPatch={(patch) => updateDraft(patch)}
            />
          )}

          {!isNotifierRuntimeDraftOnAgentPage(draft, item) ? (
            <section className="profile-section">
              <div className="profile-grid-compact">
                <label className="field span-2">
                  <span>{t("agentInstructions")}</span>
                  <textarea
                    className="compact-textarea"
                    value={draft.instructions || ""}
                    onInput={(event) => updateDraft({ instructions: event.currentTarget.value })}
                  />
                </label>
              </div>
            </section>
          ) : null}

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

          {workspaceSupported ? (
            <section className="profile-section agent-skills-section">
              <div className="profile-section-title agent-skills-title">
                <span>{t("agentSkillsTitle")}</span>
                <small>{skills.length}</small>
              </div>
              {skillsError ? <div className="form-error">{skillsError}</div> : null}
              {skillsLoading ? <div className="agent-skills-empty">{t("agentSkillsLoading")}</div> : null}
              {!skillsLoading && !skills.length ? (
                <div className="agent-skills-empty">{t("agentSkillsEmpty")}</div>
              ) : null}
              {!skillsLoading && skills.length ? (
                <div className="agent-skills-list">
                  {skills.map((skill) => (
                    <article key={skill.name} className="agent-skill-card">
                      <div className="agent-skill-name">{skill.name}</div>
                      <p className="agent-skill-description">{skill.description || "-"}</p>
                    </article>
                  ))}
                </div>
              ) : null}
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
        </div>
      ) : null}
    </section>
  );
}

type AgentChannelsSectionProps = {
  busyKey: string;
  item: AgentLike;
  onDisconnectFeishu?: AgentActionHandler;
  onStartFeishuConnect?: AgentActionHandler;
  pendingRegistration?: FeishuPendingRegistrationView;
  t: TranslateFn;
};

function AgentChannelsSection({
  item,
  t,
  busyKey,
  pendingRegistration = null,
  onDisconnectFeishu,
  onStartFeishuConnect,
}: AgentChannelsSectionProps) {
  const connected = hasConnectedAgentChannel(item, "feishu");
  const pending = Boolean(pendingRegistration?.registration_id);
  const actionBusy = Boolean(busyKey);
  const connectBusy = busyKey.endsWith(":feishu:connect") || busyKey.endsWith(":feishu:finalize");
  const disconnectBusy = busyKey.endsWith(":feishu:disconnect");
  const statusLabel = connected ? t("feishuConnected") : pending ? t("feishuPending") : t("feishuDisconnected");
  const statusIcon = connected ? (
    <CheckCircle2 aria-hidden="true" size={16} strokeWidth={2.2} />
  ) : pending ? (
    <CircleDashed aria-hidden="true" size={16} strokeWidth={2.2} />
  ) : (
    <Link2 aria-hidden="true" size={16} strokeWidth={2.2} />
  );
  const connectLabel = connected ? t("feishuReconnect") : t("feishuConnect");
  const canStart = Boolean(onStartFeishuConnect);
  const canDisconnect = connected && Boolean(onDisconnectFeishu);
  const connectURL = String(pendingRegistration?.connect_url || "").trim();

  return (
    <section className="profile-section agent-channels-section" aria-labelledby="agent-channels-title">
      <h2 id="agent-channels-title" className="profile-section-title agent-channels-title">
        {t("agentChannelsTitle")}
      </h2>
      <div className="agent-channel-row">
        <span className="agent-channel-icon" aria-hidden="true">
          <img src="icons/feishu.png" alt="" />
        </span>
        <span className="agent-channel-main">
          <span className="agent-channel-name">{t("feishuChannelName")}</span>
          <span className={`agent-channel-status ${connected ? "connected" : pending ? "pending" : ""}`.trim()}>
            {statusIcon}
            {statusLabel}
          </span>
          {pending ? <span className="agent-channel-detail">{t("feishuPendingDetail")}</span> : null}
        </span>
        <span className="agent-channel-actions">
          {pending && connectURL ? (
            <Button
              variant="secondaryGray"
              size="sm"
              type="button"
              disabled={actionBusy}
              onClick={() => window.open(connectURL, "_blank", "noopener,noreferrer")}
            >
              <ExternalLink aria-hidden="true" size={15} strokeWidth={2} />
              {t("feishuOpenConnection")}
            </Button>
          ) : null}
          <Button
            variant={connected ? "secondaryGray" : "primary"}
            size="sm"
            type="button"
            loading={connectBusy && !pending}
            loadingLabel={connectLabel}
            disabled={!canStart || actionBusy}
            onClick={() => onStartFeishuConnect?.(item)}
          >
            {connected ? (
              <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
            ) : (
              <Link2 aria-hidden="true" size={15} strokeWidth={2} />
            )}
            {connectLabel}
          </Button>
          {connected ? (
            <Button
              variant="outlineDanger"
              size="sm"
              type="button"
              loading={disconnectBusy}
              loadingLabel={t("feishuDisconnect")}
              disabled={!canDisconnect || actionBusy}
              onClick={() => onDisconnectFeishu?.(item)}
            >
              <Unlink2 aria-hidden="true" size={15} strokeWidth={2} />
              {t("feishuDisconnect")}
            </Button>
          ) : null}
        </span>
      </div>
    </section>
  );
}

type AgentActionsMenuProps = {
  activeRoom?: IMConversation | null;
  busy: boolean;
  canPublish: boolean;
  incomplete: boolean;
  isManager: boolean;
  item: AgentLike;
  onDelete: AgentActionHandler;
  onInvite: AgentActionHandler;
  onPublish?: () => VoidOrPromise;
  onRecreate: AgentActionHandler;
  onStart: AgentActionHandler;
  onStop: AgentActionHandler;
  onUpgrade?: AgentActionHandler;
  publishBusy: boolean;
  running: boolean;
  t: TranslateFn;
  upgradeNeeded: boolean;
};

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
}: AgentActionsMenuProps) {
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
