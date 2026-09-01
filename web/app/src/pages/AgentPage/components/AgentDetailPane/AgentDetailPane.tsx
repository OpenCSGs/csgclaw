import {
  AlertCircle,
  Check,
  CheckCircle2,
  CircleDashed,
  Edit3,
  ExternalLink,
  FileCode2,
  Link2,
  MoreHorizontal,
  Play,
  Plus,
  RefreshCw,
  Save,
  Server,
  Square,
  Terminal,
  Trash2,
  Unlink2,
  UploadCloud,
  UserPlus,
  X,
} from "lucide-react";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";
import { errorMessage } from "@/api/client";
import {
  fetchAgentInstructionsDocument,
  fetchAgentMemoryDocument,
  updateAgentEffectiveInstructions,
  updateAgentMemoryEnabled,
  type AgentMemoryDocument,
} from "@/api/agents";
import { SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import { AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY } from "@/shared/storage/keys";
import { localizeAPIError } from "@/shared/i18n";
import {
  EnvKeyValueEditor,
  FieldHelpTooltip,
  ClipboardCopyButton,
  ModelOptionLabel,
  NotifierControls,
  ReasoningControls,
  reasoningEffortLabel,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import {
  agentProfilePageSaveDisabled,
  agentProfileConfig,
  agentSandboxEnabled,
  agentAvailabilityStatusLabel,
  agentGatewayUnavailableLabel,
  agentRuntimeStatusDetailLabel,
  agentRuntimeKind,
  isAgentGatewayDegraded,
  isAgentAvailable,
  agentModelID,
  agentToDraft,
  formatProviderLabel,
  formatRuntimeKindLabel,
  hasConnectedAgentChannel,
  isAgentIncomplete,
  isAgentLifecycleRunning,
  isNotificationBotAgent,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
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
import { mcpManagedKnowledgeBaseSource } from "@/models/mcp";
import type { MCPServer, MCPServerSourceStatus } from "@/models/mcp";
import { skillSourceBadgeName } from "@/models/skillhub";
import type { SkillSummary } from "@/models/skillhub";
import type { SlashSkillOption } from "@/models/slashCommands";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import type { AgentTemplatePublishTarget } from "@/api/hub";
import {
  Button,
  Checkbox,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Select,
  Tooltip,
} from "@/components/ui";
type VoidOrPromise = void | Promise<void>;
type AgentActionHandler = (item: AgentLike) => VoidOrPromise;
type AgentMetadataSavePatch = Pick<Partial<AgentDraft>, "description" | "name">;
type AgentNoticeTone = "info" | "warning" | "success";
type LarkCLIDialogView = {
  kind: "message" | "install";
  message?: string;
  title?: string;
} | null;
const LARK_CLI_INSTALL_COMMAND = "npm install -g @larksuite/cli@latest";
const LARK_CLI_INSTALL_DOCS_URL = "https://github.com/larksuite/cli#installation--quick-start";
const LARK_CLI_RELEASES_URL = "https://github.com/larksuite/cli/releases/latest";
const NODEJS_DOWNLOAD_URL = "https://nodejs.org/en/download";
const AGENT_PROFILE_TAB_IDS = ["profile", "channels", "instructions", "memory", "skills", "mcp"] as const;
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
  dialogPortalContainer?: HTMLElement | null;
  draft?: AgentDraft | null;
  error?: string;
  feishuConnectBusy?: string;
  feishuPendingRegistration?: FeishuPendingRegistrationView;
  hasUnsavedChanges?: boolean;
  item: AgentLike;
  larkCLIDialog?: LarkCLIDialogView;
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
  onDismissNotice?: () => void;
  onOpenDM: AgentActionHandler;
  onRetryModels?: () => void | Promise<unknown>;
  onProviderLogin?: (provider: string) => VoidOrPromise;
  onPublish?: (
    target: AgentTemplatePublishTarget,
    name: string,
    description: string,
    includeMemory: boolean,
  ) => boolean | Promise<boolean>;
  onRecreate: AgentActionHandler;
  onSave?: () => VoidOrPromise;
  onMetadataSave?: (patch: AgentMetadataSavePatch) => VoidOrPromise;
  onMemoryChange?: () => VoidOrPromise;
  onStart: AgentActionHandler;
  onStartFeishuConnect?: AgentActionHandler;
  onStop: AgentActionHandler;
  onFinalizeFeishuConnect?: AgentActionHandler;
  onDisconnectFeishu?: AgentActionHandler;
  onDismissLarkCLIDialog?: () => void;
  onInitLarkCLI?: AgentActionHandler;
  onShowLarkCLIInstall?: AgentActionHandler;
  onUpgrade?: AgentActionHandler;
  publishBusy?: boolean;
  publishDisabled?: boolean;
  publishError?: string;
  locale?: LocaleCode;
  saveError?: string;
  saveBillingURL?: string;
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
  mcpSourceBusyNames?: ReadonlySet<string>;
  mcpSourceErrorNames?: ReadonlySet<string>;
  mcpSourceSyncBusyName?: string;
  mcpUpdateAvailableNames?: ReadonlySet<string>;
  mcpAddBusy?: boolean;
  mcpAddError?: string;
  mcpDeleteBusy?: boolean;
  mcpDeleteError?: string;
  skills?: SlashSkillOption[];
  skillsError?: string;
  skillsLoading?: boolean;
  t: TranslateFn;
  workspaceSupported?: boolean;
  directoryPickerAvailable?: boolean;
  onAddSkills?: (skillNames: string[]) => Promise<boolean> | boolean;
  onDeleteSkill?: (skill: SlashSkillOption | string) => Promise<boolean> | boolean;
  onInstallMCPServers?: (serverNames: string[]) => Promise<boolean> | boolean;
  onCheckMCPServerSource?: (
    server: MCPServer | string,
  ) => Promise<MCPServerSourceStatus | null> | MCPServerSourceStatus | null;
  onUpdateMCPServer?: (server: MCPServer | string) => Promise<boolean> | boolean;
  onDeleteMCPServer?: (server: MCPServer | string) => Promise<boolean> | boolean;
  onRetryMCPServers?: () => void | Promise<unknown>;
};

export type AgentDetailPaneHandle = {
  cancelActiveMetadataEdit: () => (keyof AgentMetadataSavePatch)[];
  commitActiveMetadataEdit: () => (keyof AgentMetadataSavePatch)[];
};

export const AgentDetailPane = forwardRef<AgentDetailPaneHandle, AgentDetailPaneProps>(function AgentDetailPane(
  {
    item,
    t,
    activeRoom = null,
    busyKey = "",
    dialogPortalContainer = null,
    error = "",
    feishuConnectBusy = "",
    feishuPendingRegistration = null,
    larkCLIDialog = null,
    draft,
    savedDraft = null,
    hasUnsavedChanges: hasUnsavedChangesProp = undefined,
    modelOptions = [],
    models = [],
    notice = "",
    noticeTone = "warning",
    modelBusy = false,
    modelError = null,
    onRetryModels,
    saving = false,
    publishBusy = false,
    publishDisabled = false,
    publishError = "",
    modelProviders = null,
    saveError = "",
    saveBillingURL = "",
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
    mcpCandidates = [],
    mcpCandidatesError = "",
    mcpCandidatesLoading = false,
    mcpServers = [],
    mcpSourceBusyNames = new Set<string>(),
    mcpSourceErrorNames = new Set<string>(),
    mcpSourceSyncBusyName = "",
    mcpUpdateAvailableNames = new Set<string>(),
    mcpAddBusy = false,
    mcpAddError = "",
    mcpDeleteBusy = false,
    mcpDeleteError = "",
    workspaceSupported = false,
    directoryPickerAvailable = true,
    onDraftChange,
    onSave,
    onPublish,
    onStart,
    onStop,
    onRecreate,
    onMetadataSave,
    onMemoryChange,
    onStartFeishuConnect,
    onDisconnectFeishu,
    onDismissLarkCLIDialog,
    onInitLarkCLI,
    onShowLarkCLIInstall,
    onUpgrade,
    onDelete,
    onInvite,
    onDismissNotice,
    onOpenDM,
    onAddSkills,
    onDeleteSkill,
    onInstallMCPServers,
    onCheckMCPServerSource,
    onUpdateMCPServer,
    onDeleteMCPServer,
    onRetryMCPServers,
  },
  ref,
) {
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
  const [publishTarget, setPublishTarget] = useState<AgentTemplatePublishTarget | null>(null);
  const [publishTemplateName, setPublishTemplateName] = useState("");
  const [publishTemplateDescription, setPublishTemplateDescription] = useState("");
  const [publishTemplateIncludeMemory, setPublishTemplateIncludeMemory] = useState(false);
  const [publishTemplateNameError, setPublishTemplateNameError] = useState("");
  const [publishAttempted, setPublishAttempted] = useState(false);
  const [isProfileScrolling, setIsProfileScrolling] = useState(false);
  const descriptionInputRef = useRef<HTMLTextAreaElement | null>(null);
  const nameInputRef = useRef<HTMLInputElement | null>(null);
  const skipMetadataAutosaveRef = useRef<Partial<Record<keyof AgentMetadataSavePatch, boolean>>>({});
  const profileScrollTimerRef = useRef<number | null>(null);
  const isManager = isManagerAgent(item);
  const canEditAgentName = Boolean(draft && !isManager);
  const running = isAgentAvailable(item);
  const lifecycleRunning = isAgentLifecycleRunning(item);
  const gatewayDegraded = isAgentGatewayDegraded(item);
  const statusLabel = agentAvailabilityStatusLabel(item, t);
  const runtimeStatusDetail = agentRuntimeStatusDetailLabel(item, t);
  const draftBelongsToItem = Boolean(draft) && String(draft?.agent_id ?? "").trim() === String(item?.id ?? "").trim();
  const incomplete = isAgentIncomplete(item, draftBelongsToItem ? draft : undefined);
  const restartNeeded = isAgentRestartNeeded(item);
  const upgradeNeeded = isAgentUpgradeNeeded(item);
  const busyPrefix = `${item.id}:`;
  const profile = agentProfileConfig(item);
  const provider = item.provider || profile?.provider || providerNameForProviderID(profile?.model_provider_id || "");
  const runtimeKind = agentRuntimeKind(item);
  const canPublishLocal = !isManager && (runtimeKind === "codex" || runtimeKind === "openclaw_sandbox");
  const canPublishCommunity = !isManager && runtimeKind === "codex";
  const supportsTemplateMemory = runtimeKind === "codex" || runtimeKind === "openclaw_sandbox";
  const hasUnsavedChanges =
    hasUnsavedChangesProp ?? Boolean(draft && savedDraft && JSON.stringify(draft) !== JSON.stringify(savedDraft));
  const saveDisabled = agentProfilePageSaveDisabled(draft, item, { saving, savedDraft });
  const updateDraft = useCallback(
    (patch: Partial<AgentDraft>) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch }),
    [draft, item, onDraftChange],
  );
  const openPublishDialog = useCallback(
    (target: AgentTemplatePublishTarget) => {
      setPublishTarget(target);
      setPublishTemplateName(String(item.name || "").trim());
      setPublishTemplateDescription(String(item.description || "").trim());
      setPublishTemplateIncludeMemory(false);
      setPublishTemplateNameError("");
      setPublishAttempted(false);
    },
    [item.description, item.name],
  );
  const submitPublishTemplate = useCallback(
    async (target = publishTarget) => {
      const name = publishTemplateName.trim();
      if (!/^[A-Za-z][A-Za-z0-9_-]{0,23}$/.test(name)) {
        setPublishTemplateNameError(t("agentPublishTemplateNameInvalid"));
        return;
      }
      if (!target || !onPublish) {
        return;
      }
      setPublishTemplateNameError("");
      setPublishAttempted(true);
      const published = await onPublish(target, name, publishTemplateDescription.trim(), publishTemplateIncludeMemory);
      if (published) {
        setPublishTarget(null);
      }
    },
    [onPublish, publishTarget, publishTemplateDescription, publishTemplateIncludeMemory, publishTemplateName, t],
  );
  const saveMetadataField = useCallback(
    <K extends keyof AgentMetadataSavePatch>(field: K, value: AgentMetadataSavePatch[K]): boolean => {
      if (skipMetadataAutosaveRef.current[field]) {
        skipMetadataAutosaveRef.current = { ...skipMetadataAutosaveRef.current, [field]: false };
        return false;
      }
      if (!onMetadataSave || saving) {
        return false;
      }
      void onMetadataSave({ [field]: value } as Pick<AgentMetadataSavePatch, K>);
      return true;
    },
    [onMetadataSave, saving],
  );
  const cancelNameEdit = useCallback(() => {
    skipMetadataAutosaveRef.current = { ...skipMetadataAutosaveRef.current, name: true };
    updateDraft({ name: savedDraft?.name ?? item.name ?? "" });
    setIsEditingName(false);
  }, [item.name, savedDraft?.name, updateDraft]);
  const cancelDescriptionEdit = useCallback(() => {
    skipMetadataAutosaveRef.current = { ...skipMetadataAutosaveRef.current, description: true };
    updateDraft({ description: savedDraft?.description ?? item.description ?? "" });
    setIsEditingDescription(false);
  }, [item.description, savedDraft?.description, updateDraft]);
  useImperativeHandle(
    ref,
    () => ({
      cancelActiveMetadataEdit: () => {
        const canceledFields: (keyof AgentMetadataSavePatch)[] = [];
        if (isEditingName) {
          cancelNameEdit();
          canceledFields.push("name");
        }
        if (isEditingDescription) {
          cancelDescriptionEdit();
          canceledFields.push("description");
        }
        return canceledFields;
      },
      commitActiveMetadataEdit: () => {
        const committedFields: (keyof AgentMetadataSavePatch)[] = [];
        if (isEditingName) {
          setIsEditingName(false);
          if (saveMetadataField("name", nameInputRef.current?.value ?? draft?.name ?? "")) {
            committedFields.push("name");
          }
        }
        if (isEditingDescription) {
          setIsEditingDescription(false);
          if (saveMetadataField("description", descriptionInputRef.current?.value ?? draft?.description ?? "")) {
            committedFields.push("description");
          }
        }
        return committedFields;
      },
    }),
    [
      cancelDescriptionEdit,
      cancelNameEdit,
      draft?.description,
      draft?.name,
      isEditingDescription,
      isEditingName,
      saveMetadataField,
    ],
  );
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
            ...(!isNotifierDraft ? [{ id: "instructions" as const, label: t("agentInstructions") }] : []),
            ...(item.memory_supported ? [{ id: "memory" as const, label: t("agentMemoryTab") }] : []),
            ...(workspaceSupported
              ? [{ id: "skills" as const, label: t("agentProfileSkillsTab"), count: skills.length }]
              : []),
            ...(showMCPServers ? [{ id: "mcp" as const, label: t("agentProfileMCPTab") }] : []),
            ...(!isNotificationBotAgent(item) ? [{ id: "channels" as const, label: t("agentChannelsTitle") }] : []),
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

  useEffect(
    () => () => {
      if (profileScrollTimerRef.current) {
        window.clearTimeout(profileScrollTimerRef.current);
      }
    },
    [],
  );

  function onProfileScroll() {
    setIsProfileScrolling(true);
    if (profileScrollTimerRef.current) {
      window.clearTimeout(profileScrollTimerRef.current);
    }
    profileScrollTimerRef.current = window.setTimeout(() => {
      setIsProfileScrolling(false);
      profileScrollTimerRef.current = null;
    }, 700);
  }

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
                portalContainer={dialogPortalContainer}
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
                      onBlur={(event) => {
                        setIsEditingName(false);
                        saveMetadataField("name", event.currentTarget.value);
                      }}
                      onInput={(event) => updateDraft({ name: event.currentTarget.value })}
                      onKeyDown={(event) => {
                        if (event.key === "Escape") {
                          event.preventDefault();
                          event.stopPropagation();
                          event.nativeEvent.stopImmediatePropagation();
                          cancelNameEdit();
                          event.currentTarget.blur();
                        } else if (event.key === "Enter") {
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
              <span className={`status-pill ${running ? "online" : ""}`}>{statusLabel}</span>
              {runtimeStatusDetail ? (
                <span className="status-pill profile-state-pill warn">{runtimeStatusDetail}</span>
              ) : null}
              {gatewayDegraded ? (
                <span className="status-pill profile-state-pill warn">{agentGatewayUnavailableLabel(item, t)}</span>
              ) : null}
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
                    onBlur={(event) => {
                      setIsEditingDescription(false);
                      saveMetadataField("description", event.currentTarget.value);
                    }}
                    onInput={(event) => updateDraft({ description: event.currentTarget.value })}
                    onKeyDown={(event) => {
                      if (event.key === "Escape") {
                        event.preventDefault();
                        event.stopPropagation();
                        event.nativeEvent.stopImmediatePropagation();
                        cancelDescriptionEdit();
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
              running={lifecycleRunning}
              upgradeNeeded={upgradeNeeded}
              canPublishLocal={canPublishLocal}
              canPublishCommunity={canPublishCommunity}
              publishBusy={publishBusy}
              publishDisabled={publishDisabled}
              onStart={onStart}
              onStop={onStop}
              onRecreate={onRecreate}
              onInvite={onInvite}
              onDelete={onDelete}
              onPublish={openPublishDialog}
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
        {saveError ? (
          <div className="form-error">
            {saveError}
            {saveBillingURL ? (
              <>
                {" "}
                <a className="agent-billing-action" href={saveBillingURL} target="_blank" rel="noopener noreferrer">
                  {t("rechargeAccount")}
                </a>
              </>
            ) : null}
          </div>
        ) : null}
        {notice ? (
          <div className={`form-warning ${noticeTone === "warning" ? "" : noticeTone}`.trim()} role="status">
            <span>{notice}</span>
            <button
              type="button"
              className="agent-notice-close"
              aria-label={t("close")}
              title={t("close")}
              onClick={onDismissNotice}
            >
              <X size={16} strokeWidth={2.3} aria-hidden="true" />
            </button>
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
      <div
        className={`agent-profile-scroll-region${
          visibleActiveProfileTab === "instructions" ? " agent-profile-scroll-region-instructions" : ""
        }${isProfileScrolling ? " is-scrolling" : ""}`}
        onScroll={onProfileScroll}
      >
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
                <strong>{reasoningEffortLabel(t, item.reasoning_effort || profile?.reasoning_effort)}</strong>
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
          <div
            className={`profile-editor-shell agent-page-editor${
              visibleActiveProfileTab === "instructions" ? " agent-page-editor-instructions" : ""
            }`}
          >
            {visibleActiveProfileTab === "profile" ? (
              <div id="agent-profile-profile" className="agent-profile-tab-panel">
                <AgentRuntimePanel
                  draft={draft}
                  directoryPickerAvailable={directoryPickerAvailable}
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
                    onRetryModels={onRetryModels}
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
            {visibleActiveProfileTab === "channels" && !isNotificationBotAgent(item) ? (
              <AgentChannelsSection
                item={item}
                t={t}
                busyKey={feishuConnectBusy.startsWith(`${item.id}:`) ? feishuConnectBusy : ""}
                pendingRegistration={feishuPendingRegistration}
                onStartFeishuConnect={onStartFeishuConnect}
                onDisconnectFeishu={onDisconnectFeishu}
                onInitLarkCLI={onInitLarkCLI}
                onShowLarkCLIInstall={onShowLarkCLIInstall}
              />
            ) : null}

            {visibleActiveProfileTab === "instructions" && !isNotifierDraft ? (
              <AgentInstructionsPanel draft={draft} t={t} updateDraft={updateDraft} />
            ) : null}

            {visibleActiveProfileTab === "memory" && item.memory_supported ? (
              <AgentMemoryPanel agentID={String(item.id || "")} onMemoryChange={onMemoryChange} t={t} />
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
                sourceBusyNames={mcpSourceBusyNames}
                sourceErrorNames={mcpSourceErrorNames}
                sourceSyncBusyName={mcpSourceSyncBusyName}
                updateAvailableNames={mcpUpdateAvailableNames}
                t={t}
                onOpenAddMCP={() => setAddMCPDialogOpen(true)}
                onRequestDeleteMCP={(server) => {
                  setMCPPendingDelete(server);
                  setDeleteMCPDialogOpen(true);
                }}
                onCheckMCPSource={onCheckMCPServerSource}
                onUpdateMCP={onUpdateMCPServer}
              />
            ) : null}
          </div>
        ) : null}
      </div>
      <DialogRoot
        open={Boolean(larkCLIDialog?.message)}
        onOpenChange={(open) => {
          if (!open) {
            onDismissLarkCLIDialog?.();
          }
        }}
      >
        <DialogContent
          className={larkCLIDialog?.kind === "install" ? "lark-cli-install-dialog" : undefined}
          portalContainer={dialogPortalContainer}
        >
          <DialogHeader>
            <div className="lark-cli-dialog-copy">
              <DialogTitle>{larkCLIDialog?.title || t("larkCLIInitFailed")}</DialogTitle>
              <DialogDescription>{larkCLIDialog?.message || ""}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {larkCLIDialog?.kind === "install" ? (
            <DialogBody className="lark-cli-install-dialog-body">
              <div className="lark-cli-install-step">
                <strong>{t("larkCLIInstallNodeTitle")}</strong>
                <span>{t("larkCLIInstallNodeDescription")}</span>
                <a href={NODEJS_DOWNLOAD_URL} target="_blank" rel="noreferrer">
                  {t("larkCLIInstallNodeLink")}
                  <ExternalLink aria-hidden="true" size={14} strokeWidth={2} />
                </a>
              </div>
              <div className="lark-cli-install-step">
                <strong>{t("larkCLIInstallCommandTitle")}</strong>
                <div className="lark-cli-install-command">
                  <code>{LARK_CLI_INSTALL_COMMAND}</code>
                  <ClipboardCopyButton
                    className="lark-cli-install-copy"
                    label={t("larkCLIInstallCopyCommand")}
                    text={LARK_CLI_INSTALL_COMMAND}
                  />
                </div>
                <span>{t("larkCLIInstallCommandHint")}</span>
              </div>
              <div className="lark-cli-install-alternative">
                <span>{t("larkCLIInstallNativeDescription")}</span>
                <a href={LARK_CLI_RELEASES_URL} target="_blank" rel="noreferrer">
                  {t("larkCLIInstallReleasesLink")}
                  <ExternalLink aria-hidden="true" size={14} strokeWidth={2} />
                </a>
              </div>
              <p className="lark-cli-install-retry-hint">{t("larkCLIInstallRetryHint")}</p>
            </DialogBody>
          ) : null}
          <DialogFooter>
            {larkCLIDialog?.kind === "install" ? (
              <>
                <a
                  className="lark-cli-install-docs-link"
                  href={LARK_CLI_INSTALL_DOCS_URL}
                  target="_blank"
                  rel="noreferrer"
                >
                  {t("larkCLIInstallDocsLink")}
                  <ExternalLink aria-hidden="true" size={14} strokeWidth={2} />
                </a>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => {
                    onDismissLarkCLIDialog?.();
                    void onInitLarkCLI?.(item);
                  }}
                >
                  <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
                  {t("larkCLIRetryConfigure")}
                </Button>
              </>
            ) : (
              <Button variant="primary" size="sm" onClick={() => onDismissLarkCLIDialog?.()}>
                {t("confirm")}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot open={addSkillsDialogOpen} onOpenChange={setAddSkillsDialogOpen}>
        <DialogContent className="agent-skills-dialog" portalContainer={dialogPortalContainer}>
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
          portalContainer={dialogPortalContainer}
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
        <DialogContent className="agent-skills-dialog agent-mcp-dialog" portalContainer={dialogPortalContainer}>
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
          portalContainer={dialogPortalContainer}
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
      <DialogRoot open={publishTarget !== null} onOpenChange={(open) => (!open ? setPublishTarget(null) : undefined)}>
        <DialogContent className="agent-publish-dialog" portalContainer={dialogPortalContainer}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("agentPublishTemplateTitle")}</DialogTitle>
              <DialogDescription>
                {publishTarget !== "local"
                  ? t("agentPublishTemplateCommunitySubtitle")
                  : t("agentPublishTemplateLocalSubtitle")}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogBody className="agent-publish-dialog-body">
            <label className="agent-publish-field">
              <span>{t("agentPublishTemplateName")}</span>
              <input
                value={publishTemplateName}
                maxLength={24}
                aria-label={t("agentPublishTemplateName")}
                aria-invalid={Boolean(publishTemplateNameError)}
                aria-describedby={publishTemplateNameError ? "agent-publish-template-name-error" : undefined}
                onChange={(event) => {
                  setPublishTemplateName(event.target.value);
                  setPublishTemplateNameError("");
                  setPublishAttempted(false);
                }}
              />
              <small>{t("agentPublishTemplateNameHint")}</small>
              {publishTemplateNameError ? (
                <span id="agent-publish-template-name-error" className="agent-publish-field-error" role="alert">
                  {publishTemplateNameError}
                </span>
              ) : null}
            </label>
            <label className="agent-publish-field">
              <span>{t("agentPublishTemplateDescription")}</span>
              <textarea
                rows={4}
                value={publishTemplateDescription}
                aria-label={t("agentPublishTemplateDescription")}
                onChange={(event) => setPublishTemplateDescription(event.target.value)}
              />
            </label>
            {supportsTemplateMemory ? (
              <label className="agent-publish-memory-option">
                <Checkbox
                  aria-label={t("agentPublishIncludeMemory")}
                  checked={publishTemplateIncludeMemory}
                  onCheckedChange={(checked) => setPublishTemplateIncludeMemory(checked === true)}
                />
                <span>
                  <strong>{t("agentPublishIncludeMemory")}</strong>
                  <small>{t("agentPublishIncludeMemoryWarning")}</small>
                </span>
              </label>
            ) : null}
            {publishAttempted && publishError ? <div className="form-error">{publishError}</div> : null}
          </DialogBody>
          <DialogFooter>
            <Button variant="secondaryGray" size="md" disabled={publishBusy} onClick={() => setPublishTarget(null)}>
              {t("cancel")}
            </Button>
            {publishTarget === "local" ? (
              <Button
                variant="primary"
                size="md"
                loading={publishBusy}
                loadingLabel={t("agentPublishing")}
                disabled={publishBusy}
                onClick={() => void submitPublishTemplate("local")}
              >
                {t("agentSaveLocalTemplate")}
              </Button>
            ) : (
              <>
                <Button
                  variant="secondaryGray"
                  size="md"
                  loading={publishBusy}
                  loadingLabel={t("agentPublishing")}
                  disabled={publishBusy}
                  onClick={() => void submitPublishTemplate("official")}
                >
                  {t("agentPublishCommunityTemplateOnly")}
                </Button>
                <Button
                  variant="primary"
                  size="md"
                  loading={publishBusy}
                  loadingLabel={t("agentPublishing")}
                  disabled={publishBusy}
                  onClick={() => void submitPublishTemplate("official_deploy")}
                >
                  {t("agentPublishCommunityAndDeploy")}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </section>
  );
});

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
  directoryPickerAvailable: boolean;
  item: AgentLike;
  locale: LocaleCode;
  onDraftChange?: (draft: AgentDraft) => void;
  runtimeKind: string;
  runtimeOptionSchemas: RuntimeOptionSchemaList;
  t: TranslateFn;
};

function AgentRuntimePanel({
  draft,
  directoryPickerAvailable,
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
              directoryPickerAvailable={directoryPickerAvailable}
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
  onCheckMCPSource?: (server: MCPServer) => Promise<MCPServerSourceStatus | null> | MCPServerSourceStatus | null;
  onUpdateMCP?: (server: MCPServer) => Promise<boolean> | boolean;
  servers: readonly MCPServer[];
  sourceBusyNames: ReadonlySet<string>;
  sourceErrorNames: ReadonlySet<string>;
  sourceSyncBusyName: string;
  updateAvailableNames: ReadonlySet<string>;
  t: TranslateFn;
};

function AgentMCPPanel({
  addBusy,
  addError,
  deleteBusy,
  deleteError,
  onOpenAddMCP,
  onRequestDeleteMCP,
  onCheckMCPSource,
  onUpdateMCP,
  servers,
  sourceBusyNames,
  sourceErrorNames,
  sourceSyncBusyName,
  updateAvailableNames,
  t,
}: AgentMCPPanelProps) {
  return (
    <section
      id="agent-profile-mcp"
      className="profile-section agent-skills-section agent-mcp-section agent-profile-scroll-target"
    >
      <div className="agent-skills-summary-heading">
        <div className="profile-section-heading">
          <div className="profile-section-title">{t("profileMCPServers")}</div>
          <p className="profile-section-description">{t("profileMCPServersHubHint")}</p>
        </div>
        <div className="agent-skills-summary-actions">
          <span className="agent-skills-summary-count">{t("agentMCPCount", { count: servers.length })}</span>
          <Tooltip content={t("agentMCPAdd")}>
            <span>
              <Button
                className="agent-skill-add-button"
                variant="secondaryGray"
                size="sm"
                aria-label={t("agentMCPAdd")}
                disabled={addBusy}
                onClick={onOpenAddMCP}
              >
                <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
              </Button>
            </span>
          </Tooltip>
        </div>
      </div>
      {addError ? <div className="form-error">{addError}</div> : null}
      {deleteError ? <div className="form-error">{deleteError}</div> : null}
      {!servers.length ? (
        <div className="agent-skills-summary-empty">
          <span className="agent-skills-summary-icon" aria-hidden="true">
            <Server size={18} strokeWidth={1.8} />
          </span>
          <div className="agent-skills-summary-empty-copy">
            <strong>{t("agentMCPEmpty")}</strong>
            <p>{t("agentMCPEmptyHint")}</p>
          </div>
          <Button variant="secondaryGray" size="sm" disabled={addBusy} onClick={onOpenAddMCP}>
            <Plus aria-hidden="true" size={15} strokeWidth={2.2} />
            {t("agentMCPAdd")}
          </Button>
        </div>
      ) : null}
      {servers.length ? (
        <div className="agent-skills-summary-list">
          {servers.map((server) => {
            const managedSource = Boolean(mcpManagedKnowledgeBaseSource(server.config));
            const sourceBusy = sourceBusyNames.has(server.name);
            const sourceError = sourceErrorNames.has(server.name);
            const updateAvailable = updateAvailableNames.has(server.name);
            const syncBusy = sourceSyncBusyName === server.name;
            return (
              <article key={server.name} className="agent-skills-summary-row agent-mcp-summary-row">
                <span className="agent-skills-summary-icon" aria-hidden="true">
                  <Server size={18} strokeWidth={1.8} />
                </span>
                <div className="agent-skills-summary-copy">
                  <div className="agent-mcp-name-row">
                    <div className="agent-skills-summary-name">{server.name}</div>
                    {managedSource ? (
                      <span className="agent-mcp-knowledge-badge">{t("agentKnowledgeMCPBadge")}</span>
                    ) : null}
                  </div>
                  <p>{server.description || "-"}</p>
                  {managedSource && (updateAvailable || sourceError) ? (
                    <p
                      className={`agent-mcp-source-hint ${
                        updateAvailable ? "update-available" : sourceError ? "source-error" : ""
                      }`.trim()}
                    >
                      {updateAvailable ? t("agentKnowledgeMCPUpdateAvailable") : t("agentKnowledgeMCPCheckFailed")}
                    </p>
                  ) : null}
                </div>
                <div className="agent-mcp-summary-actions">
                  {updateAvailable && !sourceError && onUpdateMCP ? (
                    <Button
                      variant="secondaryGray"
                      size="sm"
                      loading={syncBusy}
                      loadingLabel={t("agentMCPUpdateConfig")}
                      disabled={sourceBusy || deleteBusy || Boolean(sourceSyncBusyName && !syncBusy)}
                      onClick={() => {
                        void onUpdateMCP(server);
                      }}
                    >
                      <RefreshCw aria-hidden="true" size={14} strokeWidth={2} />
                      {t("agentMCPUpdateConfig")}
                    </Button>
                  ) : managedSource && sourceError && onCheckMCPSource ? (
                    <Button
                      variant="secondaryGray"
                      size="sm"
                      loading={sourceBusy}
                      loadingLabel={t("agentMCPSourceRetry")}
                      disabled={deleteBusy || Boolean(sourceSyncBusyName)}
                      onClick={() => {
                        void onCheckMCPSource(server);
                      }}
                    >
                      <RefreshCw aria-hidden="true" size={14} strokeWidth={2} />
                      {t("agentMCPSourceRetry")}
                    </Button>
                  ) : null}
                  <Tooltip content={t("agentDeleteMCP")}>
                    <span className="agent-skills-summary-delete">
                      <Button
                        className="agent-skill-icon-button"
                        variant="outlineDanger"
                        size="sm"
                        aria-label={t("agentDeleteMCP")}
                        disabled={addBusy || deleteBusy || Boolean(sourceSyncBusyName)}
                        onClick={() => onRequestDeleteMCP(server)}
                      >
                        <Trash2 aria-hidden="true" size={16} strokeWidth={1.9} />
                      </Button>
                    </span>
                  </Tooltip>
                </div>
              </article>
            );
          })}
        </div>
      ) : null}
    </section>
  );
}

type AgentModelPanelProps = {
  draft: AgentDraft;
  modelBusy: boolean;
  modelError: unknown;
  onRetryModels?: () => void | Promise<unknown>;
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
  onRetryModels,
  providerOptions,
  selectedModelValue,
  selectedProviderID,
  selectedProviderModels,
  t,
  updateDraft,
}: AgentModelPanelProps) {
  const selectedProviderOption = providerOptions.find((option) => option.id === selectedProviderID);
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
              {requiredFieldLabel(t("profileModelProvider"))}
              <Select
                value={selectedProviderID}
                required
                selectedLabel={
                  selectedProviderOption ? (
                    <ModelOptionLabel
                      avatar={selectedProviderOption.avatar}
                      model={selectedProviderOption.displayName}
                    />
                  ) : undefined
                }
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
                triggerProps={{ "aria-label": t("profileModelProvider"), "aria-required": true }}
                contentProps={{ side: "bottom", align: "start", avoidCollisions: false }}
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
            </label>
            <ReasoningControls
              value={draft.reasoning_effort}
              onChange={(value) => updateDraft({ reasoning_effort: value })}
              t={t}
            />
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
            {modelError ? (
              <div className="agent-model-load-error" role="alert">
                <AlertCircle className="agent-model-load-error-icon" aria-hidden="true" size={20} strokeWidth={2} />
                <div className="agent-model-load-error-content">
                  <strong>{t("modelLoadFailed")}</strong>
                  <p>{t("profileModelLoadErrorHelp")}</p>
                  {selectedModelValue ? <small>{t("profileModelCurrentSelectionRetained")}</small> : null}
                  <details className="agent-model-load-error-details">
                    <summary>{t("profileModelErrorDetails")}</summary>
                    <div className="agent-model-load-error-technical">
                      <pre>{localizeAPIError(modelError, t, errorMessage(modelError, t("modelLoadFailed")))}</pre>
                    </div>
                  </details>
                </div>
                {onRetryModels ? (
                  <Button
                    className="agent-model-load-error-retry"
                    loading={modelBusy}
                    loadingLabel={t("profileLoadingModels")}
                    size="sm"
                    variant="secondaryGray"
                    onClick={() => void onRetryModels()}
                  >
                    <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
                    {t("retry")}
                  </Button>
                ) : null}
              </div>
            ) : null}
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

type AgentMemoryPanelProps = {
  agentID: string;
  onMemoryChange?: () => VoidOrPromise;
  t: TranslateFn;
};

function AgentMemoryPanel({ agentID, onMemoryChange, t }: AgentMemoryPanelProps) {
  const [document, setDocument] = useState<AgentMemoryDocument | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [loadError, setLoadError] = useState("");

  const loadMemory = useCallback(async () => {
    if (!agentID) return;
    setLoading(true);
    setLoadError("");
    try {
      setDocument(await fetchAgentMemoryDocument(agentID));
    } catch (error) {
      setLoadError(errorMessage(error, t("agentMemoryLoadFailed")));
    } finally {
      setLoading(false);
    }
  }, [agentID, t]);

  useEffect(() => {
    setDocument(null);
    void loadMemory();
  }, [loadMemory]);

  async function setMemoryEnabled(enabled: boolean) {
    if (!agentID || updating) return;
    setUpdating(true);
    setLoadError("");
    try {
      setDocument(await updateAgentMemoryEnabled(agentID, enabled));
      await onMemoryChange?.();
    } catch (error) {
      setLoadError(errorMessage(error, t("agentMemoryUpdateFailed")));
    } finally {
      setUpdating(false);
    }
  }

  return (
    <div id="agent-profile-memory" className="agent-memory-tab agent-profile-tab-panel agent-profile-scroll-target">
      <section className="profile-section agent-skills-section agent-memory-settings-section">
        <div className="agent-memory-section-heading">
          <div className="profile-section-heading">
            <div className="profile-section-title">{t("agentMemoryTitle")}</div>
            <p className="profile-section-description">{t("agentMemoryDescription")}</p>
          </div>
          <div className="agent-memory-setting-actions">
            <span className="agent-memory-status">{document?.enabled ? t("statusEnabled") : t("statusDisabled")}</span>
            <label className="agent-memory-switch agent-fast-mode-toggle">
              <input
                type="checkbox"
                checked={document?.enabled ?? false}
                disabled={!document || loading || updating}
                aria-label={t("agentMemoryEnabled")}
                onChange={(event) => void setMemoryEnabled(event.currentTarget.checked)}
              />
              <span className="sr-only">{t("agentMemoryEnabled")}</span>
            </label>
          </div>
        </div>
        <p className="agent-memory-toggle-hint">{t("agentMemoryToggleHint")}</p>
      </section>

      <section className="profile-section agent-skills-section agent-memory-document-section">
        <div className="agent-memory-section-heading">
          <div className="profile-section-heading">
            <div className="profile-section-title">{document?.name || t("agentMemoryDocumentTitle")}</div>
            <p className="profile-section-description">{t("agentMemoryDocumentDescription")}</p>
            {document?.location ? <code className="agent-memory-location">{document.location}</code> : null}
          </div>
          <div className="agent-skills-summary-actions">
            <Tooltip content={t("agentMemoryRefresh")}>
              <span>
                <Button
                  className="agent-skill-add-button"
                  type="button"
                  variant="secondaryGray"
                  size="sm"
                  aria-label={t("agentMemoryRefresh")}
                  disabled={loading || updating}
                  onClick={() => void loadMemory()}
                >
                  <RefreshCw size={16} strokeWidth={2.2} aria-hidden="true" />
                </Button>
              </span>
            </Tooltip>
          </div>
        </div>

        {loadError ? <div className="form-error">{loadError}</div> : null}
        {loading ? (
          <div className="agent-skills-summary-empty agent-memory-summary-empty" role="status">
            <span className="agent-skills-summary-icon" aria-hidden="true">
              <CircleDashed size={18} strokeWidth={1.8} />
            </span>
            <div className="agent-skills-summary-empty-copy">
              <strong>{t("agentMemoryLoading")}</strong>
            </div>
          </div>
        ) : document?.ready ? (
          <div className="agent-section-form agent-memory-document-shell">
            <textarea
              className="compact-textarea agent-memory-document"
              value={document.content || ""}
              readOnly
              aria-label={t("agentMemoryDocumentLabel")}
            />
          </div>
        ) : (
          <div className="agent-skills-summary-empty agent-memory-summary-empty">
            <span className="agent-skills-summary-icon" aria-hidden="true">
              <FileCode2 size={18} strokeWidth={1.8} />
            </span>
            <div className="agent-skills-summary-empty-copy">
              <strong>{t("agentMemoryEmptyTitle")}</strong>
              <p>{t("agentMemoryEmptyDescription")}</p>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

type AgentInstructionsPanelProps = {
  draft: AgentDraft;
  t: TranslateFn;
  updateDraft: UpdateAgentDraft;
};

function AgentInstructionsPanel({ draft, t, updateDraft }: AgentInstructionsPanelProps) {
  const [mode, setMode] = useState<"default" | "advanced">("default");
  const [effective, setEffective] = useState("");
  const [advancedError, setAdvancedError] = useState("");
  const [advancedSaving, setAdvancedSaving] = useState(false);
  useEffect(() => {
    const agentID = String(draft.agent_id || "").trim();
    if (!agentID || mode !== "advanced") {
      return;
    }
    let canceled = false;
    setAdvancedError("");
    void fetchAgentInstructionsDocument(agentID)
      .then((document) => {
        if (!canceled) {
          setEffective(document.effective || "");
        }
      })
      .catch((error) => {
        if (!canceled) {
          setAdvancedError(errorMessage(error, t("agentInstructionsLoadFailed")));
        }
      });
    return () => {
      canceled = true;
    };
  }, [draft.agent_id, mode, t]);

  async function saveEffectiveInstructions() {
    const agentID = String(draft.agent_id || "").trim();
    if (!agentID) return;
    setAdvancedSaving(true);
    setAdvancedError("");
    try {
      const document = await updateAgentEffectiveInstructions(agentID, effective);
      setEffective(document.effective || "");
      updateDraft({ instructions: document.instructions || "" });
    } catch (error) {
      setAdvancedError(errorMessage(error, t("agentInstructionsSaveFailed")));
    } finally {
      setAdvancedSaving(false);
    }
  }
  return (
    <section
      id="agent-profile-instructions"
      className="profile-section agent-instructions-section agent-profile-scroll-target"
    >
      <div className="agent-instructions-header">
        <div className="profile-section-heading">
          <div className="profile-section-title">{t("agentInstructions")}</div>
          <p className="profile-section-description">
            {mode === "default" ? t("agentInstructionsDefaultHint") : t("agentInstructionsAdvancedHint")}
          </p>
        </div>
        <div className="agent-instructions-mode-switch" role="group" aria-label={t("agentInstructionsViewMode")}>
          <button type="button" aria-pressed={mode === "default"} onClick={() => setMode("default")}>
            {t("agentInstructionsDefaultMode")}
          </button>
          <button type="button" aria-pressed={mode === "advanced"} onClick={() => setMode("advanced")}>
            {t("agentInstructionsAdvancedMode")}
          </button>
        </div>
      </div>
      <div className="agent-instructions-content">
        <div className="profile-grid-compact">
          <label className="field span-2">
            <span className="sr-only">
              {mode === "advanced" ? t("agentInstructionsEffective") : t("agentInstructions")}
            </span>
            <textarea
              className="compact-textarea agent-instructions-editor"
              value={mode === "advanced" ? effective : draft.instructions || ""}
              onInput={(event) =>
                mode === "advanced"
                  ? setEffective(event.currentTarget.value)
                  : updateDraft({ instructions: event.currentTarget.value })
              }
              placeholder={t("agentInstructionsPlaceholder")}
            />
          </label>
        </div>
      </div>
      {advancedError ? <div className="form-error">{advancedError}</div> : null}
      {mode === "advanced" ? (
        <div className="form-actions">
          <Button type="button" variant="secondaryGray" loading={advancedSaving} onClick={saveEffectiveInstructions}>
            {t("save")}
          </Button>
        </div>
      ) : null}
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
      <div className="agent-skills-summary-heading">
        <div className="profile-section-heading">
          <div className="profile-section-title">{t("agentSkillsTitle")}</div>
          <p className="profile-section-description">{t("agentSkillsDescription")}</p>
        </div>
        <div className="agent-skills-summary-actions">
          <span className="agent-skills-summary-count">{t("agentSkillsCount", { count: skills.length })}</span>
          <Tooltip content={t("agentSkillAdd")}>
            <span>
              <Button
                className="agent-skill-add-button"
                variant="secondaryGray"
                size="sm"
                aria-label={t("agentSkillAdd")}
                disabled={skillCandidatesLoading || skillAddBusy}
                onClick={onOpenAddSkills}
              >
                <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
              </Button>
            </span>
          </Tooltip>
        </div>
      </div>
      {skillsError ? <div className="form-error">{skillsError}</div> : null}
      {skillAddError ? <div className="form-error">{skillAddError}</div> : null}
      {skillDeleteError ? <div className="form-error">{skillDeleteError}</div> : null}
      {skillsLoading ? (
        <div className="agent-skills-summary-empty">
          <span className="agent-skills-summary-icon" aria-hidden="true">
            <FileCode2 size={18} strokeWidth={1.8} />
          </span>
          <div>
            <strong>{t("agentSkillsLoading")}</strong>
          </div>
        </div>
      ) : null}
      {!skillsLoading && !skills.length ? (
        <div className="agent-skills-summary-empty">
          <span className="agent-skills-summary-icon" aria-hidden="true">
            <FileCode2 size={18} strokeWidth={1.8} />
          </span>
          <div className="agent-skills-summary-empty-copy">
            <strong>{t("agentSkillsEmpty")}</strong>
            <p>{t("agentSkillsEmptyHint")}</p>
          </div>
          <Button
            variant="secondaryGray"
            size="sm"
            disabled={skillCandidatesLoading || skillAddBusy}
            onClick={onOpenAddSkills}
          >
            <Plus aria-hidden="true" size={15} strokeWidth={2.2} />
            {t("agentSkillAdd")}
          </Button>
        </div>
      ) : null}
      {!skillsLoading && skills.length ? (
        <div className="agent-skills-summary-list">
          {skills.map((skill) => (
            <article key={skill.name} className="agent-skills-summary-row">
              <span className="agent-skills-summary-icon" aria-hidden="true">
                <FileCode2 size={18} strokeWidth={1.8} />
              </span>
              <div className="agent-skills-summary-copy">
                <div className="agent-skills-summary-name">{skill.name}</div>
                <p>{skill.description || "-"}</p>
              </div>
              <Tooltip content={t("agentDeleteSkill")}>
                <span className="agent-skills-summary-delete">
                  <Button
                    className="agent-skill-icon-button"
                    variant="outlineDanger"
                    size="sm"
                    aria-label={t("agentDeleteSkill")}
                    disabled={skillDeleteBusy}
                    onClick={() => onRequestDeleteSkill(skill)}
                  >
                    <Trash2 aria-hidden="true" size={16} strokeWidth={1.9} />
                  </Button>
                </span>
              </Tooltip>
            </article>
          ))}
        </div>
      ) : null}
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
  onInitLarkCLI?: AgentActionHandler;
  onShowLarkCLIInstall?: AgentActionHandler;
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
  onInitLarkCLI,
  onShowLarkCLIInstall,
  onStartFeishuConnect,
}: AgentChannelsSectionProps) {
  const connected = hasConnectedAgentChannel(item, "feishu");
  const pending = Boolean(pendingRegistration?.registration_id);
  const actionBusy = Boolean(busyKey);
  const connectBusy = busyKey.endsWith(":feishu:connect") || busyKey.endsWith(":feishu:finalize");
  const disconnectBusy = busyKey.endsWith(":feishu:disconnect");
  const larkCLIInitBusy = busyKey.endsWith(":feishu:lark-cli");
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
  const larkCLIState = String(item.lark_cli?.state || "")
    .trim()
    .toLowerCase();
  const larkCLIUnavailable = larkCLIState === "unavailable";
  const larkCLIMismatch = larkCLIState === "mismatch";
  const larkCLIBound = Boolean(item.lark_cli?.bound) && !larkCLIUnavailable && !larkCLIMismatch;
  const canInitLarkCLI = larkCLIUnavailable ? Boolean(onShowLarkCLIInstall) : Boolean(onInitLarkCLI);
  const larkCLIButtonLabel = larkCLIUnavailable
    ? t("larkCLIInstallRequiredAction")
    : larkCLIMismatch
      ? t("larkCLIReinit")
      : larkCLIBound
        ? t("larkCLIConfiguredAction")
        : t("larkCLIInit");

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
              variant="secondaryGray"
              size="sm"
              type="button"
              loading={larkCLIInitBusy}
              loadingLabel={larkCLIButtonLabel}
              disabled={!canInitLarkCLI || actionBusy}
              onClick={() => (larkCLIUnavailable ? onShowLarkCLIInstall?.(item) : onInitLarkCLI?.(item))}
            >
              {larkCLIUnavailable ? (
                <AlertCircle aria-hidden="true" size={15} strokeWidth={2} />
              ) : larkCLIMismatch ? (
                <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
              ) : larkCLIBound ? (
                <CheckCircle2 aria-hidden="true" size={15} strokeWidth={2} />
              ) : (
                <Terminal aria-hidden="true" size={15} strokeWidth={2} />
              )}
              {larkCLIButtonLabel}
            </Button>
          ) : null}
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
  canPublishLocal: boolean;
  canPublishCommunity: boolean;
  incomplete: boolean;
  isManager: boolean;
  item: AgentLike;
  onDelete: AgentActionHandler;
  onInvite: AgentActionHandler;
  onPublish?: (target: AgentTemplatePublishTarget) => VoidOrPromise;
  onRecreate: AgentActionHandler;
  onStart: AgentActionHandler;
  onStop: AgentActionHandler;
  onUpgrade?: AgentActionHandler;
  publishBusy: boolean;
  publishDisabled: boolean;
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
  canPublishLocal,
  canPublishCommunity,
  publishBusy,
  publishDisabled,
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
            {running ? (
              <Square aria-hidden="true" size={15} strokeWidth={2} />
            ) : (
              <Play aria-hidden="true" size={15} strokeWidth={2} />
            )}
            <span>{running ? t("agentStop") : t("agentStart")}</span>
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem disabled={busy || incomplete} onSelect={() => onRecreate(item)}>
          <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
          <span>{t("agentRecreate")}</span>
        </DropdownMenuItem>
        {SHOW_AGENT_LIFECYCLE_ACTIONS && activeRoom && !isManager ? (
          <DropdownMenuItem disabled={busy} onSelect={() => onInvite(item)}>
            <UserPlus aria-hidden="true" size={15} strokeWidth={2} />
            <span>{t("inviteToRoom")}</span>
          </DropdownMenuItem>
        ) : null}
        {canPublishLocal ? (
          <>
            <DropdownMenuItem disabled={publishBusy} onSelect={() => onPublish?.("local")}>
              <Save aria-hidden="true" size={15} strokeWidth={2} />
              <span>{publishBusy ? t("agentPublishing") : t("agentSaveLocalTemplate")}</span>
            </DropdownMenuItem>
            {canPublishCommunity ? (
              <DropdownMenuItem
                disabled={publishBusy || publishDisabled}
                title={publishDisabled ? t("agentPublishLoginRequired") : undefined}
                onSelect={() => onPublish?.("official")}
              >
                <UploadCloud aria-hidden="true" size={15} strokeWidth={2} />
                <span>{publishBusy ? t("agentPublishing") : t("agentPublishCommunity")}</span>
              </DropdownMenuItem>
            ) : null}
          </>
        ) : null}
        {!isManager ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem danger disabled={busy} onSelect={() => onDelete(item)}>
              <Trash2 aria-hidden="true" size={15} strokeWidth={2} />
              <span>{t("agentDelete")}</span>
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenuRoot>
  );
}
