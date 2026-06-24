import {
  Check,
  CheckCircle2,
  CircleDashed,
  Edit3,
  ExternalLink,
  Link2,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Trash2,
  Unlink2,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "@/api/client";
import { REASONING_EFFORTS, SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import {
  EnvKeyValueEditor,
  ModelOptionLabel,
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
  normalizeRuntimeKind,
  runtimeOptionSchemasForAgent,
} from "@/models/agents";
import type { AgentDraft, AgentLike } from "@/models/agents";
import {
  modelProviderAvatarPath,
  modelProviderSelectOptionsFromCatalog,
  providerNameForProviderID,
  selectorForProviderModel,
  type ModelProviderCatalog,
  type ModelProviderOption,
} from "@/models/modelProviders";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { LocaleCode } from "@/models/conversations";
import type { SkillSummary } from "@/models/skillhub";
import type { SlashSkillOption } from "@/models/slashCommands";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import {
  Button,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
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
  authStatuses?: unknown;
  busyKey?: string;
  draft?: AgentDraft | null;
  error?: string;
  feishuConnectBusy?: string;
  feishuPendingRegistration?: FeishuPendingRegistrationView;
  hasUnsavedChanges?: boolean;
  item: AgentLike;
  modelBusy?: boolean;
  modelError?: unknown;
  modelOptions?: ModelProviderOption[];
  modelProviders?: ModelProviderCatalog | null;
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
  skillAddBusy?: boolean;
  skillAddError?: string;
  skillCandidates?: SkillSummary[];
  skillCandidatesError?: string;
  skillCandidatesLoading?: boolean;
  skillDeleteBusy?: boolean;
  skillDeleteError?: string;
  skills?: SlashSkillOption[];
  skillsError?: string;
  skillsLoading?: boolean;
  t: TranslateFn;
  workspaceSupported?: boolean;
  onAddSkills?: (skillNames: string[]) => Promise<boolean> | boolean;
  onDeleteSkill?: (skill: SlashSkillOption | string) => Promise<boolean> | boolean;
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
  modelOptions = [],
  models = [],
  notice = "",
  noticeTone = "warning",
  modelBusy = false,
  modelError = null,
  saving = false,
  publishBusy = false,
  modelProviders = null,
  saveError = "",
  locale = "en",
  notifierWebhookPublicOrigin = "",
  skills = [],
  skillsLoading = false,
  skillsError = "",
  skillCandidates = [],
  skillCandidatesLoading = false,
  skillCandidatesError = "",
  skillAddBusy = false,
  skillAddError = "",
  skillDeleteBusy = false,
  skillDeleteError = "",
  workspaceSupported = false,
  onDraftChange,
  onSave,
  onPublish,
  onStart,
  onStop,
  onRecreate,
  onStartFeishuConnect,
  onDisconnectFeishu,
  onUpgrade,
  onDelete,
  onInvite,
  onOpenDM,
  onAddSkills,
  onDeleteSkill,
}: AgentDetailPaneProps) {
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const [addSkillsDialogOpen, setAddSkillsDialogOpen] = useState(false);
  const [selectedSkillNames, setSelectedSkillNames] = useState<string[]>([]);
  const [deleteSkillDialogOpen, setDeleteSkillDialogOpen] = useState(false);
  const [skillPendingDelete, setSkillPendingDelete] = useState<SlashSkillOption | null>(null);
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
  const fallbackProviderID = String(draft?.model_provider_id || "").trim();
  const fallbackModelOptions =
    modelOptions.length > 0
      ? modelOptions
      : fallbackProviderID
        ? models.map((model) => ({
            value: selectorForProviderModel(fallbackProviderID, model),
            label: `${fallbackProviderID} / ${model}`,
            providerID: fallbackProviderID,
            providerDisplayName: fallbackProviderID,
            providerAvatar: modelProviderAvatarPath(fallbackProviderID),
            modelID: model,
          }))
        : [];
  const providerOptions = modelProviderSelectOptionsFromCatalog(modelProviders, fallbackModelOptions);
  const selectedProviderID =
    draft?.model_provider_id ||
    providerOptions.find((option) => option.models.includes(draft?.model_id || ""))?.id ||
    "";
  const selectedProvider = providerOptions.find((option) => option.id === selectedProviderID) ?? null;
  const selectedProviderModels = selectedProvider?.models ?? [];
  const selectedModelValue = draft?.model_id || "";

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

  useEffect(() => {
    if (!addSkillsDialogOpen) {
      setSelectedSkillNames([]);
    }
  }, [addSkillsDialogOpen]);

  async function handleAddSkillsConfirm(): Promise<void> {
    if (!selectedSkillNames.length) {
      return;
    }
    const added = await onAddSkills?.(selectedSkillNames);
    if (added) {
      setAddSkillsDialogOpen(false);
    }
  }

  async function handleDeleteSkillConfirm(): Promise<void> {
    if (!skillPendingDelete) {
      return;
    }
    const deleted = await onDeleteSkill?.(skillPendingDelete);
    if (deleted) {
      setDeleteSkillDialogOpen(false);
      setSkillPendingDelete(null);
    }
  }

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
              <div className="profile-runtime-grid agent-model-config-grid">
                <label className="field">
                  {requiredFieldLabel(t("profileProvider"))}
                  <Select
                    value={selectedProviderID}
                    required
                    onValueChange={(value) => {
                      const nextProvider = providerOptions.find((option) => option.id === value);
                      if (!nextProvider) {
                        updateDraft({ model_id: "", model_provider_id: "" });
                        return;
                      }
                      updateDraft({
                        provider: providerNameForProviderID(nextProvider.id),
                        model_provider_id: nextProvider.id,
                        model_id: nextProvider.models[0] || "",
                      });
                    }}
                    triggerProps={{ "aria-label": t("profileProvider"), "aria-required": true }}
                    options={[
                      { value: "", label: modelBusy ? t("profileLoadingModels") : t("profileProviderSelect") },
                      ...providerOptions.map((option) => ({
                        value: option.value,
                        label: (
                          <ModelOptionLabel
                            avatar={option.avatar}
                            model={option.displayName}
                            provider={
                              option.models.length
                                ? t("modelProviderModelCount", { count: option.models.length })
                                : t("modelProviderNoModels")
                            }
                          />
                        ),
                        textValue: option.displayName,
                      })),
                    ]}
                  />
                </label>
                <label className="field">
                  {requiredFieldLabel(t("profileModel"))}
                  <Select
                    value={selectedModelValue}
                    required
                    disabled={!selectedProviderID || !selectedProviderModels.length}
                    onValueChange={(value) => updateDraft({ model_id: value })}
                    searchable
                    searchPlaceholder={t("modelProviderModelSearch")}
                    emptyLabel={t("modelProviderNoModels")}
                    triggerProps={{ "aria-label": t("profileModel"), "aria-required": true }}
                    options={[
                      {
                        value: "",
                        label: selectedProviderID ? t("profileSelectModel") : t("profileProviderSelectFirst"),
                      },
                      ...selectedProviderModels.map((modelID) => ({
                        value: modelID,
                        label: <ModelOptionLabel model={modelID} showAvatar={false} />,
                        textValue: modelID,
                      })),
                      ...(selectedModelValue && !selectedProviderModels.includes(selectedModelValue)
                        ? [
                            {
                              value: selectedModelValue,
                              label: <ModelOptionLabel model={selectedModelValue} showAvatar={false} />,
                              textValue: selectedModelValue,
                            },
                          ]
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
                <div className="field agent-fast-mode-field">
                  <span>{t("profileFastMode")}</span>
                  <label className="selection-item compact-toggle-row agent-fast-mode-toggle">
                    <input
                      type="checkbox"
                      checked={draft.enable_fast_mode}
                      aria-label={t("profileFastMode")}
                      onChange={() => updateDraft({ enable_fast_mode: !draft.enable_fast_mode })}
                    />
                    <small className="agent-fast-mode-help">{t("profileFastModeHelp")}</small>
                  </label>
                </div>
              </div>
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

          {workspaceSupported ? (
            <section className="profile-section agent-skills-section">
              <div className="profile-section-title agent-skills-title">
                <div className="agent-skills-title-copy">
                  <span>{t("agentSkillsTitle")}</span>
                  <small>{skills.length}</small>
                </div>
                <Button
                  className="agent-skill-icon-button"
                  variant="secondaryGray"
                  size="sm"
                  aria-label={t("agentSkillAdd")}
                  title={t("agentSkillAdd")}
                  disabled={skillCandidatesLoading || skillAddBusy}
                  onClick={() => setAddSkillsDialogOpen(true)}
                >
                  <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
                </Button>
              </div>
              {skillsError ? <div className="form-error">{skillsError}</div> : null}
              {skillAddError ? <div className="form-error">{skillAddError}</div> : null}
              {skillDeleteError ? <div className="form-error">{skillDeleteError}</div> : null}
              {skillsLoading ? <div className="agent-skills-empty">{t("agentSkillsLoading")}</div> : null}
              {!skillsLoading && !skills.length ? (
                <div className="agent-skills-empty">{t("agentSkillsEmpty")}</div>
              ) : null}
              {!skillsLoading && skills.length ? (
                <div className="agent-skills-list">
                  {skills.map((skill) => (
                    <article key={skill.name} className="agent-skill-card">
                      <div className="agent-skill-card-header">
                        <div className="agent-skill-name">{skill.name}</div>
                        <Button
                          className="agent-skill-icon-button"
                          variant="outlineDanger"
                          size="sm"
                          aria-label={t("agentDeleteSkill")}
                          title={t("agentDeleteSkill")}
                          disabled={skillDeleteBusy}
                          onClick={() => {
                            setSkillPendingDelete(skill);
                            setDeleteSkillDialogOpen(true);
                          }}
                        >
                          <Trash2 aria-hidden="true" size={16} strokeWidth={1.9} />
                        </Button>
                      </div>
                      <p className="agent-skill-description">{skill.description || "-"}</p>
                    </article>
                  ))}
                </div>
              ) : null}
            </section>
          ) : null}

          <details className="profile-section agent-advanced-section">
            <summary className="profile-section-title agent-advanced-summary">{t("profileAdvanced")}</summary>
            <div className="profile-advanced-grid">
              <div className="field profile-env-field">
                <span>{t("profileEnv")}</span>
                <EnvKeyValueEditor rows={draft.envRows} t={t} onChange={(rows) => updateDraft({ envRows: rows })} />
              </div>
            </div>
          </details>
        </div>
      ) : null}
      <DialogRoot open={addSkillsDialogOpen} onOpenChange={setAddSkillsDialogOpen}>
        <DialogContent className="agent-skills-dialog">
          <DialogHeader className="agent-skills-dialog-header">
            <div className="agent-skills-dialog-copy">
              <DialogTitle>{t("agentSkillAdd")}</DialogTitle>
              <DialogDescription>{t("agentSkillAddSubtitle")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <div className="agent-skills-dialog-body">
            {skillCandidatesError ? <div className="form-error">{skillCandidatesError}</div> : null}
            {skillAddError ? <div className="form-error">{skillAddError}</div> : null}
            {skillCandidatesLoading ? (
              <div className="agent-skills-empty">{t("agentSkillsLoading")}</div>
            ) : !skillCandidates.length ? (
              <div className="agent-skills-empty">{t("agentSkillAddEmpty")}</div>
            ) : (
              <div className="agent-skill-candidates-list" role="list">
                {skillCandidates.map((skill) => {
                  const checked = selectedSkillNames.includes(skill.name);
                  return (
                    <label key={skill.name} className={`agent-skill-candidate ${checked ? "selected" : ""}`.trim()}>
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={(event) => {
                          const nextChecked = event.currentTarget.checked;
                          setSelectedSkillNames((current) =>
                            nextChecked ? [...current, skill.name] : current.filter((name) => name !== skill.name),
                          );
                        }}
                      />
                      <span className="agent-skill-candidate-copy">
                        <span className="agent-skill-name">{skill.name}</span>
                        <span className="agent-skill-description">{skill.description || "-"}</span>
                      </span>
                    </label>
                  );
                })}
              </div>
            )}
          </div>
          <div className="agent-skills-dialog-actions">
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={skillAddBusy}
              onClick={() => setAddSkillsDialogOpen(false)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              loading={skillAddBusy}
              loadingLabel={t("agentSkillAdd")}
              disabled={!selectedSkillNames.length || skillCandidatesLoading}
              onClick={handleAddSkillsConfirm}
            >
              {t("agentSkillAdd")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={deleteSkillDialogOpen}
        onOpenChange={(open) => {
          setDeleteSkillDialogOpen(open);
          if (!open) {
            setSkillPendingDelete(null);
          }
        }}
      >
        <DialogContent className="agent-skills-dialog agent-skill-delete-dialog">
          <DialogHeader className="agent-skills-dialog-header">
            <div className="agent-skills-dialog-copy">
              <DialogTitle>{t("agentDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("agentDeleteSkillConfirmMessage", { name: skillPendingDelete?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <div className="agent-skills-dialog-body">
            {skillDeleteError ? <div className="form-error">{skillDeleteError}</div> : null}
          </div>
          <div className="agent-skills-dialog-actions">
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={skillDeleteBusy}
              onClick={() => {
                setDeleteSkillDialogOpen(false);
                setSkillPendingDelete(null);
              }}
            >
              {t("cancel")}
            </Button>
            <Button variant="danger" size="sm" loading={skillDeleteBusy} onClick={handleDeleteSkillConfirm}>
              {t("agentDeleteSkill")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
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
