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
  Server,
  Trash2,
  Unlink2,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { errorMessage } from "@/api/client";
import { REASONING_EFFORTS, SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import { AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY } from "@/shared/storage/keys";
import {
  EnvKeyValueEditor,
  FieldHelpTooltip,
  ModelOptionLabel,
  NotifierControls,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import {
  agentProfilePageSaveDisabled,
  agentProfileConfig,
  agentSandboxEnabled,
  agentRuntimeKind,
  agentRuntimeState,
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
  isManagerAgent,
  isNotifierRuntimeDraftOnAgentPage,
  runtimeOptionSchemasForAgent,
  supportsMCPServers,
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
import type { MCPServer } from "@/models/mcp";
import { skillSourceBadgeName } from "@/models/skillhub";
import type { SkillSummary } from "@/models/skillhub";
import type { SlashSkillOption } from "@/models/slashCommands";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeTemplateSourceTag } from "@/shared/i18n";
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
import { AgentActivityPanel } from "./AgentActivityPanel";

type VoidOrPromise = void | Promise<void>;
type AgentActionHandler = (item: AgentLike) => VoidOrPromise;
type AgentNoticeTone = "info" | "warning" | "success";
const AGENT_PROFILE_TAB_IDS = ["profile", "activity", "channels", "instructions", "skills", "mcp"] as const;
type AgentProfileTabID = (typeof AGENT_PROFILE_TAB_IDS)[number];
type UpdateAgentDraft = (patch: Partial<AgentDraft>) => void;
type RuntimeOptionSchemaList = ReturnType<typeof runtimeOptionSchemasForAgent>;
type ModelProviderSelectOption = ReturnType<typeof modelProviderSelectOptionsFromCatalog>[number];
const DEFAULT_AGENT_PROFILE_TAB_ID: AgentProfileTabID = "profile";

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
  rooms?: IMConversation[];
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
  mcpCandidates?: MCPServer[];
  mcpCandidatesError?: string;
  mcpCandidatesLoading?: boolean;
  mcpServers?: MCPServer[];
  mcpAddBusy?: boolean;
  mcpAddError?: string;
  mcpDeleteBusy?: boolean;
  mcpDeleteError?: string;
  skills?: SlashSkillOption[];
  skillsError?: string;
  skillsLoading?: boolean;
  t: TranslateFn;
  workspaceSupported?: boolean;
  onAddSkills?: (skillNames: string[]) => Promise<boolean> | boolean;
  onDeleteSkill?: (skill: SlashSkillOption | string) => Promise<boolean> | boolean;
  onInstallMCPServers?: (serverNames: string[]) => Promise<boolean> | boolean;
  onDeleteMCPServer?: (server: MCPServer | string) => Promise<boolean> | boolean;
  onRetryMCPServers?: () => void | Promise<unknown>;
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
  rooms = [],
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
  mcpCandidates = [],
  mcpCandidatesError = "",
  mcpCandidatesLoading = false,
  mcpServers = [],
  mcpAddBusy = false,
  mcpAddError = "",
  mcpDeleteBusy = false,
  mcpDeleteError = "",
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
  onInstallMCPServers,
  onDeleteMCPServer,
  onRetryMCPServers,
}: AgentDetailPaneProps) {
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const [isEditingName, setIsEditingName] = useState(false);
  const [activeProfileTab, setActiveProfileTab] = useState<AgentProfileTabID>(() => readAgentProfileActiveTab());
  const [addSkillsDialogOpen, setAddSkillsDialogOpen] = useState(false);
  const [selectedSkillNames, setSelectedSkillNames] = useState<string[]>([]);
  const [deleteSkillDialogOpen, setDeleteSkillDialogOpen] = useState(false);
  const [skillPendingDelete, setSkillPendingDelete] = useState<SlashSkillOption | null>(null);
  const [addMCPDialogOpen, setAddMCPDialogOpen] = useState(false);
  const [selectedMCPServerNames, setSelectedMCPServerNames] = useState<string[]>([]);
  const [deleteMCPDialogOpen, setDeleteMCPDialogOpen] = useState(false);
  const [mcpPendingDelete, setMCPPendingDelete] = useState<MCPServer | null>(null);
  const descriptionInputRef = useRef<HTMLTextAreaElement | null>(null);
  const nameInputRef = useRef<HTMLInputElement | null>(null);
  const isManager = isManagerAgent(item);
  const canEditAgentName = Boolean(draft && !isManager);
  const running = isAgentRunning(item);
  const draftBelongsToItem = Boolean(draft) && String(draft?.agent_id ?? "").trim() === String(item?.id ?? "").trim();
  const incomplete = isAgentIncomplete(item, draftBelongsToItem ? draft : undefined);
  const restartNeeded = isAgentRestartNeeded(item);
  const upgradeNeeded = isAgentUpgradeNeeded(item);
  const busyPrefix = `${item.id}:`;
  const profile = agentProfileConfig(item);
  const provider = item.provider || profile?.provider || providerNameForProviderID(profile?.model_provider_id || "");
  const runtimeKind = agentRuntimeKind(item);
  const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";
  const hasUnsavedChanges =
    hasUnsavedChangesProp ?? Boolean(draft && savedDraft && JSON.stringify(draft) !== JSON.stringify(savedDraft));
  const saveDisabled = agentProfilePageSaveDisabled(draft, item, { saving, savedDraft });
  const updateDraft = (patch: Partial<AgentDraft>) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });
  const runtimeOptionSchemas = runtimeOptionSchemasForAgent(draft?.runtime_kind || runtimeKind, item);
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
  const isNotifierDraft = Boolean(draft && isNotifierRuntimeDraftOnAgentPage(draft, item));
  const showMCPServers = Boolean(
    draft && !isNotifierDraft && supportsMCPServers(draft.runtime_kind || item.runtime_kind),
  );
  const profileTabs = useMemo(
    () =>
      draft
        ? [
            { id: "profile" as const, label: t("agentProfileTab") },
            { id: "activity" as const, label: t("agentActivityTab") },
            ...(!isNotifierDraft ? [{ id: "instructions" as const, label: t("agentInstructions") }] : []),
            ...(workspaceSupported
              ? [{ id: "skills" as const, label: t("agentProfileSkillsTab"), count: skills.length }]
              : []),
            ...(!isNotificationBotAgent(item) ? [{ id: "channels" as const, label: t("agentChannelsTitle") }] : []),
            ...(showMCPServers ? [{ id: "mcp" as const, label: t("agentProfileMCPTab") }] : []),
          ]
        : [],
    [draft, isNotifierDraft, item, showMCPServers, skills.length, t, workspaceSupported],
  );
  const visibleActiveProfileTab = profileTabs.some((tab) => tab.id === activeProfileTab)
    ? activeProfileTab
    : profileTabs[0]?.id;

  useEffect(() => {
    if (!draft) {
      setIsEditingDescription(false);
      setIsEditingName(false);
    }
  }, [draft]);

  useEffect(() => {
    if (!canEditAgentName) {
      setIsEditingName(false);
    }
  }, [canEditAgentName]);

  useEffect(() => {
    if (!isEditingName) {
      return;
    }
    nameInputRef.current?.focus();
    nameInputRef.current?.select();
  }, [isEditingName]);

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

  useEffect(() => {
    if (!addMCPDialogOpen) {
      setSelectedMCPServerNames([]);
    }
  }, [addMCPDialogOpen]);

  useEffect(() => {
    if (!showMCPServers) {
      setAddMCPDialogOpen(false);
      setDeleteMCPDialogOpen(false);
      setMCPPendingDelete(null);
    }
  }, [showMCPServers]);

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

  async function handleAddMCPConfirm(): Promise<void> {
    if (!selectedMCPServerNames.length) {
      return;
    }
    const installed = await onInstallMCPServers?.(selectedMCPServerNames);
    if (installed) {
      setAddMCPDialogOpen(false);
    }
  }

  async function handleDeleteMCPConfirm(): Promise<void> {
    if (!mcpPendingDelete) {
      return;
    }
    const deleted = await onDeleteMCPServer?.(mcpPendingDelete);
    if (deleted) {
      setDeleteMCPDialogOpen(false);
      setMCPPendingDelete(null);
    }
  }

  function selectProfileTab(tabID: AgentProfileTabID): void {
    setActiveProfileTab(tabID);
    saveAgentProfileActiveTab(tabID);
  }

  return (
    <section className="entity-pane agent-detail-pane">
      <div className="agent-profile-fixed-header">
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
              <AgentAvatarContent avatar={item.avatar} fallback={avatarFallbackText(item.avatar, item.name, item.id)} />
            </div>
          )}
          <div className="entity-heading">
            <div className="entity-title-row">
              {draft ? (
                canEditAgentName && isEditingName ? (
                  <label className="agent-title-edit-field">
                    <span className="sr-only">{t("agentName")}</span>
                    <input
                      ref={nameInputRef}
                      className="agent-title-input"
                      value={draft.name}
                      required
                      aria-required="true"
                      onBlur={() => setIsEditingName(false)}
                      onInput={(event) => updateDraft({ name: event.currentTarget.value })}
                      onKeyDown={(event) => {
                        if (event.key === "Escape" || event.key === "Enter") {
                          event.preventDefault();
                          event.currentTarget.blur();
                        }
                      }}
                      placeholder={t("agentName")}
                    />
                  </label>
                ) : canEditAgentName ? (
                  <button
                    type="button"
                    className={`agent-title-display ${draft.name ? "" : "is-empty"}`.trim()}
                    aria-label={t("editAgentName")}
                    onClick={() => setIsEditingName(true)}
                  >
                    <span className="agent-title-display-copy">{draft.name || t("agentName")}</span>
                    <span className="agent-title-display-icon" aria-hidden="true">
                      <Edit3 size={16} strokeWidth={1.8} />
                    </span>
                  </button>
                ) : (
                  <h1>{draft.name || item.name || t("agentName")}</h1>
                )
              ) : (
                <h1>{item.name}</h1>
              )}
              <span className={`agent-status-dot ${running ? "online" : ""}`} aria-hidden="true"></span>
              <span className={`status-pill ${running ? "online" : ""}`}>
                {agentStatusLabel(agentRuntimeState(item), t)}
              </span>
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
            {onUpgrade && upgradeNeeded ? (
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
        {profileTabs.length ? (
          <nav className="agent-profile-section-nav" aria-label={t("agentProfileSectionNavLabel")}>
            {profileTabs.map((section) => {
              const active = section.id === visibleActiveProfileTab;
              return (
                <button
                  key={section.id}
                  type="button"
                  className={`agent-profile-section-tab ${active ? "active" : ""}`.trim()}
                  aria-current={active ? "location" : undefined}
                  aria-controls={`agent-profile-${section.id}`}
                  onClick={() => selectProfileTab(section.id)}
                >
                  <span>{section.label}</span>
                  {typeof section.count === "number" ? (
                    <span className="agent-profile-section-tab-count" aria-label={String(section.count)}>
                      {section.count}
                    </span>
                  ) : null}
                </button>
              );
            })}
          </nav>
        ) : null}
      </div>
      <div className="agent-profile-scroll-region">
        {!draft ? (
          <>
            <div className="entity-grid">
              <div className="entity-field">
                <span>{t("profileRuntimeKind")}</span>
                <strong>{formatRuntimeKindLabel(runtimeKind, t)}</strong>
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
                <strong>{item.reasoning_effort || profile?.reasoning_effort || "medium"}</strong>
              </div>
              <div className="entity-field">
                <span>{t("profileFastMode")}</span>
                <strong>{item.enable_fast_mode || profile?.enable_fast_mode ? "on" : "off"}</strong>
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
            {visibleActiveProfileTab === "profile" ? (
              <div id="agent-profile-profile" className="agent-profile-tab-panel">
                <AgentRuntimePanel
                  draft={draft}
                  item={item}
                  locale={locale}
                  runtimeKind={runtimeKind}
                  runtimeOptionSchemas={runtimeOptionSchemas}
                  t={t}
                  onDraftChange={onDraftChange}
                />
                {!isNotifierDraft ? (
                  <AgentModelPanel
                    draft={draft}
                    modelBusy={modelBusy}
                    modelError={modelError}
                    providerOptions={providerOptions}
                    selectedModelValue={selectedModelValue}
                    selectedProviderID={selectedProviderID}
                    selectedProviderModels={selectedProviderModels}
                    t={t}
                    updateDraft={updateDraft}
                  />
                ) : (
                  <AgentNotifierPanel
                    draft={draft}
                    item={item}
                    notifierWebhookPublicOrigin={notifierWebhookPublicOrigin}
                    t={t}
                    updateDraft={updateDraft}
                  />
                )}
                <AgentAdvancedPanel draft={draft} item={item} t={t} updateDraft={updateDraft} />
              </div>
            ) : null}
            {visibleActiveProfileTab === "activity" ? (
              <AgentActivityPanel item={item} locale={locale} rooms={rooms} t={t} />
            ) : null}
            {visibleActiveProfileTab === "channels" && !isNotificationBotAgent(item) ? (
              <AgentChannelsSection
                item={item}
                t={t}
                busyKey={feishuConnectBusy.startsWith(`${item.id}:`) ? feishuConnectBusy : ""}
                pendingRegistration={feishuPendingRegistration}
                onStartFeishuConnect={onStartFeishuConnect}
                onDisconnectFeishu={onDisconnectFeishu}
              />
            ) : null}

            {visibleActiveProfileTab === "instructions" && !isNotifierDraft ? (
              <AgentInstructionsPanel draft={draft} t={t} updateDraft={updateDraft} />
            ) : null}

            {visibleActiveProfileTab === "skills" && workspaceSupported ? (
              <AgentSkillsPanel
                skillAddBusy={skillAddBusy}
                skillAddError={skillAddError}
                skillCandidatesLoading={skillCandidatesLoading}
                skillDeleteBusy={skillDeleteBusy}
                skillDeleteError={skillDeleteError}
                skills={skills}
                skillsError={skillsError}
                skillsLoading={skillsLoading}
                t={t}
                onOpenAddSkills={() => setAddSkillsDialogOpen(true)}
                onRequestDeleteSkill={(skill) => {
                  setSkillPendingDelete(skill);
                  setDeleteSkillDialogOpen(true);
                }}
              />
            ) : null}

            {showMCPServers && visibleActiveProfileTab === "mcp" ? (
              <AgentMCPPanel
                addBusy={mcpAddBusy}
                addError={mcpAddError}
                deleteBusy={mcpDeleteBusy}
                deleteError={mcpDeleteError}
                servers={mcpServers}
                t={t}
                onOpenAddMCP={() => setAddMCPDialogOpen(true)}
                onRequestDeleteMCP={(server) => {
                  setMCPPendingDelete(server);
                  setDeleteMCPDialogOpen(true);
                }}
              />
            ) : null}
          </div>
        ) : null}
      </div>
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
                  const sourceBadgeName = skillSourceBadgeName(skill);
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
                        <span className="agent-skill-name">
                          {skill.name}
                          {sourceBadgeName ? ` · ${localizeTemplateSourceTag(sourceBadgeName, locale)}` : ""}
                        </span>
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
        <DialogContent
          className="agent-skills-dialog agent-skill-delete-dialog"
          overlayClassName="agent-skill-delete-backdrop"
        >
          <DialogHeader className="agent-skills-dialog-header">
            <div className="agent-skills-dialog-copy">
              <DialogTitle>{t("agentDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("agentDeleteSkillConfirmMessage", { name: skillPendingDelete?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {skillDeleteError ? (
            <div className="agent-skills-dialog-body agent-skill-delete-dialog-body">
              <div className="form-error">{skillDeleteError}</div>
            </div>
          ) : null}
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
      <DialogRoot open={addMCPDialogOpen} onOpenChange={setAddMCPDialogOpen}>
        <DialogContent className="agent-skills-dialog agent-mcp-dialog">
          <DialogHeader className="agent-skills-dialog-header">
            <div className="agent-skills-dialog-copy">
              <DialogTitle>{t("agentMCPAdd")}</DialogTitle>
              <DialogDescription>{t("agentMCPAddSubtitle")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <div className="agent-skills-dialog-body">
            {mcpAddError ? <div className="form-error">{mcpAddError}</div> : null}
            {mcpCandidatesError ? (
              <div className="form-error">
                <span>{mcpCandidatesError}</span>
                {onRetryMCPServers ? (
                  <Button
                    variant="secondaryGray"
                    size="sm"
                    disabled={mcpCandidatesLoading}
                    onClick={() => {
                      void onRetryMCPServers();
                    }}
                  >
                    {t("retry")}
                  </Button>
                ) : null}
              </div>
            ) : mcpCandidatesLoading && !mcpCandidates.length ? (
              <div className="agent-skills-empty">{t("resourcesMCPLoading")}</div>
            ) : !mcpCandidates.length ? (
              <div className="agent-skills-empty">{t("agentMCPAddEmpty")}</div>
            ) : (
              <div className="agent-skill-candidates-list" role="list">
                {mcpCandidates.map((server) => {
                  const checked = selectedMCPServerNames.includes(server.name);
                  return (
                    <label key={server.name} className={`agent-skill-candidate ${checked ? "selected" : ""}`.trim()}>
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={(event) => {
                          const nextChecked = event.currentTarget.checked;
                          setSelectedMCPServerNames((current) =>
                            nextChecked ? [...current, server.name] : current.filter((name) => name !== server.name),
                          );
                        }}
                      />
                      <span className="agent-skill-candidate-copy">
                        <span className="agent-skill-name">{server.name}</span>
                        <span className="agent-skill-description">{server.description || "-"}</span>
                      </span>
                    </label>
                  );
                })}
              </div>
            )}
          </div>
          <div className="agent-skills-dialog-actions">
            <Button variant="secondaryGray" size="sm" onClick={() => setAddMCPDialogOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              loading={mcpAddBusy}
              loadingLabel={t("agentMCPAdd")}
              disabled={
                !selectedMCPServerNames.length ||
                mcpAddBusy ||
                Boolean(mcpCandidatesError) ||
                (mcpCandidatesLoading && !mcpCandidates.length)
              }
              onClick={handleAddMCPConfirm}
            >
              {t("agentMCPAdd")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={deleteMCPDialogOpen}
        onOpenChange={(open) => {
          setDeleteMCPDialogOpen(open);
          if (!open) {
            setMCPPendingDelete(null);
          }
        }}
      >
        <DialogContent
          className="agent-skills-dialog agent-skill-delete-dialog"
          overlayClassName="agent-skill-delete-backdrop"
        >
          <DialogHeader className="agent-skills-dialog-header">
            <div className="agent-skills-dialog-copy">
              <DialogTitle>{t("agentDeleteMCP")}</DialogTitle>
              <DialogDescription>
                {t("agentDeleteMCPConfirmMessage", { name: mcpPendingDelete?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {mcpDeleteError ? <div className="form-error">{mcpDeleteError}</div> : null}
          <div className="agent-skills-dialog-actions">
            <Button
              variant="secondaryGray"
              size="sm"
              onClick={() => {
                setDeleteMCPDialogOpen(false);
                setMCPPendingDelete(null);
              }}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              loading={mcpDeleteBusy}
              loadingLabel={t("agentDeleteMCP")}
              disabled={mcpDeleteBusy}
              onClick={handleDeleteMCPConfirm}
            >
              {t("agentDeleteMCP")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
    </section>
  );
}

function readAgentProfileActiveTab(): AgentProfileTabID {
  if (typeof window === "undefined") {
    return DEFAULT_AGENT_PROFILE_TAB_ID;
  }
  try {
    const raw = window.localStorage.getItem(AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY);
    return isAgentProfileTabID(raw) ? raw : DEFAULT_AGENT_PROFILE_TAB_ID;
  } catch {
    return DEFAULT_AGENT_PROFILE_TAB_ID;
  }
}

function saveAgentProfileActiveTab(tabID: AgentProfileTabID): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY, tabID);
  } catch {
    // Active tab persistence is best-effort.
  }
}

function isAgentProfileTabID(value: unknown): value is AgentProfileTabID {
  return AGENT_PROFILE_TAB_IDS.includes(value as AgentProfileTabID);
}

type AgentRuntimePanelProps = {
  draft: AgentDraft;
  item: AgentLike;
  locale: LocaleCode;
  onDraftChange?: (draft: AgentDraft) => void;
  runtimeKind: string;
  runtimeOptionSchemas: RuntimeOptionSchemaList;
  t: TranslateFn;
};

function AgentRuntimePanel({
  draft,
  item,
  locale,
  onDraftChange,
  runtimeKind,
  runtimeOptionSchemas,
  t,
}: AgentRuntimePanelProps) {
  const isNotifierDraft = isNotifierRuntimeDraftOnAgentPage(draft, item);
  const sandboxEnabled = draft.sandbox_enabled ?? agentSandboxEnabled(item);

  return (
    <section id="agent-profile-runtime" className="profile-section agent-profile-scroll-target">
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("profileRuntimeSection")}</div>
        <p className="profile-section-description">{t("profileRuntimeSectionDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="profile-grid-compact agent-page-form-content">
          <label className="field">
            <span>{t("profileRuntimeKind")}</span>
            <input value={formatRuntimeKindLabel(draft.runtime_kind || runtimeKind, t)} readOnly disabled />
          </label>
          {!isNotifierDraft ? (
            <div className="field agent-fast-mode-field agent-sandbox-readonly-field">
              <div className="field-label-with-help">
                <span>{t("profileSandboxEnabled")}</span>
                <FieldHelpTooltip detail={t("profileSandboxEnabledHelp")} />
              </div>
              <label className="selection-item compact-toggle-row agent-fast-mode-toggle agent-sandbox-toggle readonly">
                <input
                  type="checkbox"
                  checked={sandboxEnabled}
                  aria-label={t("profileSandboxEnabled")}
                  readOnly
                  disabled
                />
                <span className="agent-sandbox-copy">
                  <strong>{sandboxEnabled ? t("statusEnabled") : t("statusDisabled")}</strong>
                </span>
              </label>
            </div>
          ) : null}
          {!isNotifierDraft && runtimeOptionSchemas.length > 0 ? (
            <RuntimeOptionsFields
              draft={draft}
              locale={locale}
              schemas={runtimeOptionSchemas}
              onDraftChange={onDraftChange || (() => {})}
              embedded
            />
          ) : null}
        </div>
      </div>
    </section>
  );
}

type AgentMCPPanelProps = {
  addBusy: boolean;
  addError: string;
  deleteBusy: boolean;
  deleteError: string;
  onOpenAddMCP: () => void;
  onRequestDeleteMCP: (server: MCPServer) => void;
  servers: readonly MCPServer[];
  t: TranslateFn;
};

function AgentMCPPanel({
  addBusy,
  addError,
  deleteBusy,
  deleteError,
  onOpenAddMCP,
  onRequestDeleteMCP,
  servers,
  t,
}: AgentMCPPanelProps) {
  return (
    <section
      id="agent-profile-mcp"
      className="profile-section agent-skills-section agent-mcp-section agent-profile-scroll-target"
    >
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("profileMCPServers")}</div>
        <p className="profile-section-description">{t("profileMCPServersHubHint")}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-skills-form-content">
          <div className="agent-skills-title">
            <div className="agent-skills-title-copy">
              <span>{t("profileMCPServers")}</span>
              <small className="agent-section-count-badge">{servers.length}</small>
            </div>
            <Button
              className="agent-skill-add-button"
              variant="secondaryGray"
              size="sm"
              aria-label={t("agentMCPAdd")}
              title={t("agentMCPAdd")}
              disabled={addBusy}
              onClick={onOpenAddMCP}
            >
              <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
            </Button>
          </div>
          {addError ? <div className="form-error">{addError}</div> : null}
          {deleteError ? <div className="form-error">{deleteError}</div> : null}
          {!servers.length ? <div className="agent-skills-empty">{t("agentMCPEmpty")}</div> : null}
          {servers.length ? (
            <div className="agent-skills-list">
              {servers.map((server) => (
                <article key={server.name} className="agent-skill-card agent-mcp-card">
                  <div className="agent-skill-card-header">
                    <div className="agent-skill-name">
                      <Server aria-hidden="true" size={14} strokeWidth={2} />
                      <span>{server.name}</span>
                    </div>
                    <Button
                      className="agent-skill-icon-button"
                      variant="outlineDanger"
                      size="sm"
                      aria-label={t("agentDeleteMCP")}
                      title={t("agentDeleteMCP")}
                      disabled={deleteBusy}
                      onClick={() => onRequestDeleteMCP(server)}
                    >
                      <Trash2 aria-hidden="true" size={16} strokeWidth={1.9} />
                    </Button>
                  </div>
                  <p className="agent-skill-description">{server.description || "-"}</p>
                </article>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );
}

type AgentModelPanelProps = {
  draft: AgentDraft;
  modelBusy: boolean;
  modelError: unknown;
  providerOptions: readonly ModelProviderSelectOption[];
  selectedModelValue: string;
  selectedProviderID: string;
  selectedProviderModels: readonly string[];
  t: TranslateFn;
  updateDraft: UpdateAgentDraft;
};

function AgentModelPanel({
  draft,
  modelBusy,
  modelError,
  providerOptions,
  selectedModelValue,
  selectedProviderID,
  selectedProviderModels,
  t,
  updateDraft,
}: AgentModelPanelProps) {
  return (
    <section id="agent-profile-model" className="profile-section agent-profile-scroll-target">
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("profileModelSection")}</div>
        <p className="profile-section-description">{t("profileModelSectionDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-model-form-content">
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
        </div>
      </div>
    </section>
  );
}

type AgentNotifierPanelProps = {
  draft: AgentDraft;
  item: AgentLike;
  notifierWebhookPublicOrigin: string;
  t: TranslateFn;
  updateDraft: UpdateAgentDraft;
};

function AgentNotifierPanel({ draft, item, notifierWebhookPublicOrigin, t, updateDraft }: AgentNotifierPanelProps) {
  return (
    <div id="agent-profile-model" className="agent-profile-scroll-target">
      <NotifierControls
        agentID={item.id || ""}
        draft={draft}
        t={t}
        webhookPublicOrigin={notifierWebhookPublicOrigin}
        onPatch={(patch) => updateDraft(patch)}
      />
    </div>
  );
}

type AgentInstructionsPanelProps = {
  draft: AgentDraft;
  t: TranslateFn;
  updateDraft: UpdateAgentDraft;
};

function AgentInstructionsPanel({ draft, t, updateDraft }: AgentInstructionsPanelProps) {
  return (
    <section
      id="agent-profile-instructions"
      className="profile-section agent-instructions-section agent-profile-scroll-target"
    >
      <div className="profile-grid-compact">
        <label className="field span-2">
          <span>{t("agentInstructions")}</span>
          <textarea
            className="compact-textarea"
            value={draft.instructions || ""}
            onInput={(event) => updateDraft({ instructions: event.currentTarget.value })}
            placeholder={t("agentInstructionsPlaceholder")}
          />
        </label>
      </div>
    </section>
  );
}

type AgentSkillsPanelProps = {
  onOpenAddSkills: () => void;
  onRequestDeleteSkill: (skill: SlashSkillOption) => void;
  skillAddBusy: boolean;
  skillAddError: string;
  skillCandidatesLoading: boolean;
  skillDeleteBusy: boolean;
  skillDeleteError: string;
  skills: readonly SlashSkillOption[];
  skillsError: string;
  skillsLoading: boolean;
  t: TranslateFn;
};

function AgentSkillsPanel({
  onOpenAddSkills,
  onRequestDeleteSkill,
  skillAddBusy,
  skillAddError,
  skillCandidatesLoading,
  skillDeleteBusy,
  skillDeleteError,
  skills,
  skillsError,
  skillsLoading,
  t,
}: AgentSkillsPanelProps) {
  return (
    <section id="agent-profile-skills" className="profile-section agent-skills-section agent-profile-scroll-target">
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("agentSkillsTitle")}</div>
        <p className="profile-section-description">{t("agentSkillsDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-skills-form-content">
          <div className="agent-skills-title">
            <div className="agent-skills-title-copy">
              <span>{t("agentSkillsTitle")}</span>
              <small className="agent-section-count-badge">{skills.length}</small>
            </div>
            <Button
              className="agent-skill-add-button"
              variant="secondaryGray"
              size="sm"
              aria-label={t("agentSkillAdd")}
              title={t("agentSkillAdd")}
              disabled={skillCandidatesLoading || skillAddBusy}
              onClick={onOpenAddSkills}
            >
              <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
            </Button>
          </div>
          {skillsError ? <div className="form-error">{skillsError}</div> : null}
          {skillAddError ? <div className="form-error">{skillAddError}</div> : null}
          {skillDeleteError ? <div className="form-error">{skillDeleteError}</div> : null}
          {skillsLoading ? <div className="agent-skills-empty">{t("agentSkillsLoading")}</div> : null}
          {!skillsLoading && !skills.length ? <div className="agent-skills-empty">{t("agentSkillsEmpty")}</div> : null}
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
                      onClick={() => onRequestDeleteSkill(skill)}
                    >
                      <Trash2 aria-hidden="true" size={16} strokeWidth={1.9} />
                    </Button>
                  </div>
                  <p className="agent-skill-description">{skill.description || "-"}</p>
                </article>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );
}

type AgentAdvancedPanelProps = {
  draft: AgentDraft;
  item: AgentLike;
  t: TranslateFn;
  updateDraft: UpdateAgentDraft;
};

function AgentAdvancedPanel({ draft, item, t, updateDraft }: AgentAdvancedPanelProps) {
  return (
    <section id="agent-profile-advanced" className="profile-section agent-advanced-section agent-profile-scroll-target">
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("profileAdvanced")}</div>
        <p className="profile-section-description">{t("profileAdvancedDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="profile-advanced-grid agent-page-form-content">
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
      </div>
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
    <section
      id="agent-profile-channels"
      className="profile-section agent-channels-section agent-profile-scroll-target"
      aria-label={t("agentChannelsTitle")}
    >
      <div className="profile-section-heading">
        <h2 id="agent-channels-title" className="profile-section-title agent-channels-title">
          {t("agentChannelsTitle")}
        </h2>
        <p className="profile-section-description">{t("agentChannelsDescription")}</p>
      </div>
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
