import { EntityField, EntityAvatar, EntityPane } from "@/components/business/EntityDetail";
import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import agentDetailPaneStyles from "./AgentDetailPane.module.css";
import agentSkillsStyles from "./AgentSkills.module.css";
import agentPublicationStyles from "./AgentPublication.module.css";
import agentMemoryStyles from "./AgentMemory.module.css";
import agentInstructionsStyles from "./AgentInstructions.module.css";
import agentChannelsStyles from "./AgentChannels.module.css";
import { AgentProfileHeader } from "./AgentProfileHeader";
import type { HubTemplate } from "@/models/hubWorkspace";
import { AgentModelPanel } from "./components/AgentModelPanel";
import { defaultTemplateName, isValidTemplateName, TEMPLATE_NAME_MAX_LENGTH } from "@/models/templateNaming";
import { SidebarMcpIcon } from "@/components/business/Icons";
import { ResourceLoadMore } from "../ResourceLoadMore";
import type { ResourceContinuation } from "@/hooks/workspace/useInfiniteAgentResources";
import { mcpManagedKnowledgeBaseSource } from "@/models/mcp";
import {
  Archive,
  BookOpen,
  Check,
  CheckCircle2,
  CircleDashed,
  Copy,
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
  Trash2,
  Unlink2,
  UploadCloud,
  UserPlus,
  X,
} from "lucide-react";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";
import {
  ResourceList,
  ResourceListCard,
  ResourceFeaturedIcon,
  ResourceStatus,
} from "@/components/business/ResourceListCard";
import { AgentResourcePicker } from "../AgentResourcePicker";
import { AgentResourceDetails } from "../AgentResourceDetails";
import type { AgentResourceKind, AgentSkillInstallOptions } from "@/api/agents";
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
import {
  EnvKeyValueEditor,
  ClipboardCopyButton,
  NotifierControls,
  reasoningEffortLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import {
  agentProfilePageSaveDisabled,
  agentProfileConfig,
  agentAvailabilityStatusLabel,
  agentGatewayUnavailableLabel,
  agentRuntimeStatusDetailLabel,
  agentRuntimeKind,
  agentSandboxEnabled,
  canPublishCommunityTemplateRuntime,
  canStartAgent,
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
  modelIDsForProviderSelection,
  modelProviderAvatarPath,
  modelProviderSelectOptionsFromCatalog,
  providerNameForProviderID,
  selectorForProviderModel,
  type ModelProviderCatalog,
  type ModelProviderOption,
} from "@/models/modelProviders";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { LocaleCode } from "@/models/conversations";
import { mcpServerDisplayName } from "@/models/mcp";
import type { MCPServer } from "@/models/mcp";
import type { SlashSkillOption } from "@/models/slashCommands";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";
import "@/components/business/MessageContent/MessageContent.css";
import { avatarFallbackText } from "@/shared/avatar";
import type { RelayPublicationController } from "@/hooks/workspace/useRelayPublication";
import { RelayPublication } from "../RelayPublication";
import type { AgentTemplatePublishTarget } from "@/api/hub";
import {
  StatusBadge,
  DismissibleInlineError,
  InlineError,
  Button,
  Field,
  TextInput,
  TextArea,
  SegmentedControl,
  Switch,
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
  Tooltip,
} from "@/components/ui";
import { SidebarPuzzlePiece02Icon } from "@/components/business/Icons";
import { AgentAppsPanel, useAgentApps } from "@/components/business/Apps";

type VoidOrPromise = void | Promise<void>;
type AgentActionHandler = (item: AgentLike) => VoidOrPromise;
type AgentMetadataSavePatch = Pick<Partial<AgentDraft>, "description" | "name">;
type AgentNoticeTone = "info" | "warning" | "success";

const AGENT_PROFILE_TAB_IDS = [
  "profile",
  "channels",
  "instructions",
  "memory",
  "skills",
  "apps",
  "mcp",
  "knowledge",
] as const;
type AgentProfileTabID = (typeof AGENT_PROFILE_TAB_IDS)[number];
type UpdateAgentDraft = (patch: Partial<AgentDraft>) => void;
type RuntimeOptionSchemaList = ReturnType<typeof runtimeOptionSchemasForAgent>;
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
  requestedProfileTab?: string;
  requestedAppID?: string;
  requestedAddAppID?: string;
  onProfileTabChange?: (tab: string, appID?: string) => void;
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
  modelBusy?: boolean;
  modelError?: unknown;
  modelOptions?: ModelProviderOption[];
  modelProviders?: ModelProviderCatalog | null;
  models?: string[];
  notice?: string;
  noticeTone?: AgentNoticeTone;
  notifierWebhookPublicOrigin?: string;
  onDelete: (item: AgentLike, returnFocus?: HTMLElement | null) => VoidOrPromise;
  onCheckDelete?: (item: AgentLike) => Promise<{ message: string; onResolve?: () => void } | null>;
  onDraftChange?: (draft: AgentDraft) => void;
  onInvite: AgentActionHandler;
  onDismissNotice?: () => void;
  onOpenDM: AgentActionHandler;
  onNewTask?: AgentActionHandler;
  onRetryModels?: () => void | Promise<unknown>;
  onProviderLogin?: (provider: string) => VoidOrPromise;
  onRequireOpenCSGAuth?: () => boolean;
  relayPublication?: RelayPublicationController;
  onPublish?: (
    target: AgentTemplatePublishTarget,
    name: string,
    description: string,
    includeMemory: boolean,
    displayName: string,
    commitMessage?: string,
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
  onUpgrade?: AgentActionHandler;
  linkedTemplate?: HubTemplate;
  publishBusy?: boolean;
  publishDisabled?: boolean;
  onPublishLogin?: () => void;
  publishError?: string;
  locale?: LocaleCode;
  saveError?: string;
  saveBillingURL?: string;
  savedDraft?: AgentDraft | null;
  saving?: boolean;
  resourceBusy?: string;
  resourceError?: string;
  onRetryResource?: () => Promise<void>;
  onSetResourceEnabled?: (kind: AgentResourceKind, name: string, enabled: boolean) => Promise<void>;
  skillAddBusy?: boolean;
  skillAddError?: string;
  skillDeleteBusy?: boolean;
  skillDeleteError?: string;
  mcpServers?: MCPServer[];
  knowledgeServers?: MCPServer[];
  resourceAuthenticated?: boolean;
  mcpSourceBusyNames?: ReadonlySet<string>;
  mcpSourceUnavailableNames?: ReadonlySet<string>;
  mcpSourceSyncBusyName?: string;
  mcpUpdateAvailableNames?: ReadonlySet<string>;
  mcpAddBusy?: boolean;
  mcpAddError?: string;
  mcpDeleteBusy?: boolean;
  mcpDeleteError?: string;
  skillContinuation?: ResourceContinuation;
  mcpContinuation?: ResourceContinuation;
  knowledgeContinuation?: ResourceContinuation;
  knowledgeListError?: string;
  mcpListError?: string;
  onRefreshSkills?: () => void | Promise<unknown>;
  skills?: SlashSkillOption[];
  skillsError?: string;
  skillsLoading?: boolean;
  t: TranslateFn;
  workspaceSupported?: boolean;
  directoryPickerAvailable?: boolean;
  onAddSkills?: (skillNames: string[], options?: AgentSkillInstallOptions) => Promise<boolean> | boolean;
  onDeleteSkill?: (skill: SlashSkillOption | string) => Promise<boolean> | boolean;
  onInstallMCPServers?: (serverNames: string[]) => Promise<boolean> | boolean;
  onUpdateMCPServer?: (server: MCPServer | string) => Promise<boolean> | boolean;
  onDeleteMCPServer?: (server: MCPServer | string) => Promise<boolean> | boolean;
};

export type AgentDetailPaneHandle = {
  cancelActiveMetadataEdit: () => (keyof AgentMetadataSavePatch)[];
  commitActiveMetadataEdit: () => (keyof AgentMetadataSavePatch)[];
};

export const AgentDetailPane = forwardRef<AgentDetailPaneHandle, AgentDetailPaneProps>(function AgentDetailPane(
  {
    item,
    t,
    requestedProfileTab,
    requestedAppID,
    requestedAddAppID,
    onProfileTabChange,
    activeRoom = null,
    busyKey = "",
    dialogPortalContainer = null,
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
    onRetryModels,
    saving = false,
    linkedTemplate,
    publishBusy = false,
    publishDisabled = false,
    onPublishLogin,
    publishError = "",
    modelProviders = null,
    saveError = "",
    saveBillingURL = "",
    locale = "en",
    notifierWebhookPublicOrigin = "",
    resourceBusy = "",
    resourceError = "",
    onRetryResource,
    onSetResourceEnabled,
    skills = [],
    skillsLoading = false,
    skillsError = "",
    skillContinuation,
    mcpContinuation,
    knowledgeContinuation,
    knowledgeListError = "",
    mcpListError = "",
    onRefreshSkills,
    skillAddBusy = false,
    skillAddError = "",
    skillDeleteBusy = false,
    skillDeleteError = "",
    mcpServers = [],
    knowledgeServers: providedKnowledgeServers,
    resourceAuthenticated = true,
    mcpSourceBusyNames = new Set<string>(),
    mcpSourceUnavailableNames = new Set<string>(),
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
    relayPublication,
    onStart,
    onStop,
    onMetadataSave,
    onMemoryChange,
    onStartFeishuConnect,
    onDisconnectFeishu,
    onUpgrade,
    onRecreate,
    onDelete,
    onInvite,
    onDismissNotice,
    onOpenDM,
    onNewTask,
    onAddSkills,
    onDeleteSkill,
    onInstallMCPServers,
    onUpdateMCPServer,
    onDeleteMCPServer,
    onRequireOpenCSGAuth = () => true,
  },
  ref,
) {
  const knowledgeServers =
    providedKnowledgeServers ?? mcpServers.filter((server) => mcpManagedKnowledgeBaseSource(server.config));
  const [recreateTarget, setRecreateTarget] = useState<AgentLike | null>(null);
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const [isEditingName, setIsEditingName] = useState(false);
  const [activeProfileTab, setActiveProfileTab] = useState<AgentProfileTabID>(() => readAgentProfileActiveTab());
  const [appSettingsID, setAppSettingsID] = useState<string | undefined>();
  const [resourceDetail, setResourceDetail] = useState<{
    agentID: string;
    kind: AgentResourceKind;
    name: string;
  } | null>(null);
  const canToggleResources = ["codex", "dsh"].includes(String(agentRuntimeKind(item)));
  const detailSkill =
    resourceDetail && resourceDetail.agentID === item?.id && resourceDetail.kind === "skill"
      ? skills.find((skill) => skill.name === resourceDetail.name)
      : undefined;
  const detailMCP =
    resourceDetail && resourceDetail.agentID === item?.id && resourceDetail.kind === "mcp"
      ? [...mcpServers, ...knowledgeServers].find((server) => server.name === resourceDetail.name)
      : undefined;
  const openResourceDetail = (kind: AgentResourceKind, name: string) =>
    setResourceDetail({ agentID: String(item?.id || ""), kind, name });
  const resourceMutationBusy = Boolean(
    resourceBusy || skillAddBusy || skillDeleteBusy || mcpAddBusy || mcpDeleteBusy || mcpSourceSyncBusyName || saving,
  );
  const [addSkillsDialogOpen, setAddSkillsDialogOpen] = useState(false);
  const [deleteSkillDialogOpen, setDeleteSkillDialogOpen] = useState(false);
  const [skillPendingDelete, setSkillPendingDelete] = useState<SlashSkillOption | null>(null);
  const [addMCPDialogOpen, setAddMCPDialogOpen] = useState(false);
  const [deleteMCPDialogOpen, setDeleteMCPDialogOpen] = useState(false);
  const [addKnowledgeDialogOpen, setAddKnowledgeDialogOpen] = useState(false);
  const toolServers = providedKnowledgeServers
    ? mcpServers
    : mcpServers.filter((server) => !mcpManagedKnowledgeBaseSource(server.config));
  const [mcpPendingDelete, setMCPPendingDelete] = useState<MCPServer | null>(null);
  useEffect(() => {
    setAddSkillsDialogOpen(false);
    setAddMCPDialogOpen(false);
    setAddKnowledgeDialogOpen(false);
  }, [item.id]);

  const [publishTarget, setPublishTarget] = useState<AgentTemplatePublishTarget | null>(null);
  const [publishTemplateDisplayName, setPublishTemplateDisplayName] = useState("");
  const [publishTemplateName, setPublishTemplateName] = useState("");
  const [publishTemplateDescription, setPublishTemplateDescription] = useState("");
  const [publishCommitMessage, setPublishCommitMessage] = useState("");
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
  const canPublishLocal = !isManager && (runtimeKind === "codex" || runtimeKind === "dsh");
  const canPublishCommunity = !isManager && canPublishCommunityTemplateRuntime(runtimeKind);
  const supportsTemplateMemory = ["codex", "dsh"].includes(runtimeKind);
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
      setPublishTemplateName(linkedTemplate?.name || defaultTemplateName(String(item.id || "")));
      setPublishTemplateDisplayName(linkedTemplate?.display_name || String(item.name || "").trim());
      setPublishTemplateDescription(String(item.description || "").trim());
      setPublishCommitMessage("");
      setPublishTemplateIncludeMemory(false);
      setPublishTemplateNameError("");
      setPublishAttempted(false);
    },
    [item.description, item.id, item.name, linkedTemplate?.name, linkedTemplate?.display_name],
  );
  const submitPublishTemplate = useCallback(
    async (target = publishTarget) => {
      const name = publishTemplateName.trim();
      if (!isValidTemplateName(name)) {
        setPublishTemplateNameError(t("agentPublishTemplateNameInvalid"));
        return;
      }
      if (!target || !onPublish) {
        return;
      }
      setPublishTemplateNameError("");
      setPublishAttempted(true);
      const published = await onPublish(
        target,
        name,
        publishTemplateDescription.trim(),
        publishTemplateIncludeMemory,
        publishTemplateDisplayName.trim(),
        publishCommitMessage.trim(),
      );
      if (published) {
        setPublishTarget(null);
      }
    },
    [
      onPublish,
      publishTarget,
      publishTemplateDisplayName,
      publishCommitMessage,
      publishTemplateDescription,
      publishTemplateIncludeMemory,
      publishTemplateName,
      t,
    ],
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
  useEffect(() => {
    if (!draft || String(draft.model_provider_id || "").trim() || !selectedProviderID) {
      return;
    }
    onDraftChange?.({
      ...draft,
      provider: providerNameForProviderID(selectedProviderID),
      model_provider_id: selectedProviderID,
    });
  }, [draft, onDraftChange, selectedProviderID]);
  const selectedProvider = providerOptions.find((option) => option.id === selectedProviderID) ?? null;
  const selectedProviderModels = modelIDsForProviderSelection(selectedProviderID, selectedProvider?.models ?? []);
  const selectedModelValue = draft?.model_id || "";
  const isNotifierDraft = Boolean(draft && isNotifierRuntimeDraftOnAgentPage(draft, item));
  const showMCPServers = Boolean(
    draft && !isNotifierDraft && supportsMCPServers(draft.runtime_kind || item.runtime_kind),
  );
  const showApps = !isNotifierDraft && (showMCPServers || ["codex", "dsh"].includes(runtimeKind));
  const profileTabs = useMemo(
    () =>
      draft
        ? [
            { id: "profile" as const, label: t("agentProfileTab") },
            ...(!isNotifierDraft ? [{ id: "instructions" as const, label: t("agentInstructions") }] : []),
            ...(item.memory_supported ? [{ id: "memory" as const, label: t("agentMemoryTab") }] : []),
            ...(workspaceSupported
              ? [
                  {
                    id: "skills" as const,
                    label: t("agentProfileSkillsTab"),
                    count: skillContinuation?.total ?? skills.length,
                  },
                ]
              : []),
            ...(showApps ? [{ id: "apps" as const, label: t("agentAppsTab") }] : []),
            ...(!showApps && showMCPServers ? [{ id: "mcp" as const, label: t("agentAppsTab") }] : []),
            ...(showMCPServers
              ? [
                  {
                    id: "knowledge" as const,
                    label: t("profileKnowledgeTitle"),
                    count: knowledgeContinuation?.total ?? knowledgeServers.length,
                  },
                ]
              : []),
            ...(!isNotificationBotAgent(item) ? [{ id: "channels" as const, label: t("agentChannelsTitle") }] : []),
          ]
        : [],
    [
      draft,
      isNotifierDraft,
      item,
      showApps,
      showMCPServers,
      skills.length,
      knowledgeServers.length,
      knowledgeContinuation?.total,
      skillContinuation?.total,
      t,
      workspaceSupported,
    ],
  );
  const requestedTab = requestedProfileTab || activeProfileTab;
  const selectedProfileTab =
    showApps && (requestedTab === "mcp" || requestedTab === "connectors")
      ? "apps"
      : !showApps && showMCPServers && requestedTab === "apps"
        ? "mcp"
        : requestedTab;
  const visibleActiveProfileTab = profileTabs.some((tab) => tab.id === selectedProfileTab)
    ? selectedProfileTab
    : profileTabs[0]?.id;
  useEffect(() => {
    if (visibleActiveProfileTab === "skills" && workspaceSupported) void onRefreshSkills?.();
  }, [item.id, visibleActiveProfileTab, workspaceSupported, onRefreshSkills]);
  const appsController = useAgentApps(
    String(item.id || ""),
    showApps && (visibleActiveProfileTab === "apps" || visibleActiveProfileTab === "mcp"),
  );
  function selectAppSettings(id: string | undefined) {
    setAppSettingsID(id);
    setActiveProfileTab("apps");
    onProfileTabChange?.("apps", id);
  }

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
    if (!showMCPServers) {
      setAddMCPDialogOpen(false);
      setAddKnowledgeDialogOpen(false);
      setDeleteMCPDialogOpen(false);
      setMCPPendingDelete(null);
      setResourceDetail(null);
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

  async function handleDeleteSkillConfirm(): Promise<void> {
    if (!skillPendingDelete) {
      return;
    }
    const deleted = await onDeleteSkill?.(skillPendingDelete);
    if (deleted) {
      setDeleteSkillDialogOpen(false);
      setSkillPendingDelete(null);
      setResourceDetail(null);
    }
  }

  async function handleDeleteMCPConfirm(): Promise<void> {
    if (!mcpPendingDelete) {
      return;
    }
    const agentID = String(item.id);
    const name = mcpPendingDelete.name;
    const deleted = await onDeleteMCPServer?.(mcpPendingDelete);
    if (deleted) {
      setDeleteMCPDialogOpen(false);
      setMCPPendingDelete(null);
      setResourceDetail((current) =>
        current?.agentID === agentID && current.kind === "mcp" && current.name === name ? null : current,
      );
    }
  }

  function selectProfileTab(tabID: AgentProfileTabID): void {
    setActiveProfileTab(tabID);
    saveAgentProfileActiveTab(tabID);
    setAppSettingsID(undefined);
    onProfileTabChange?.(tabID);
  }

  const toolConnectors = (
    <AgentMCPPanel
      embedded={showApps}
      continuation={mcpContinuation}
      toggleBusyName={resourceBusy.startsWith("mcp:") ? resourceBusy.slice(4) : ""}
      listError={mcpListError}
      onOpenDetail={(server) => openResourceDetail("mcp", server.name)}
      onToggle={
        canToggleResources && onSetResourceEnabled
          ? (server) => onSetResourceEnabled("mcp", server.name, server.config.enabled === false)
          : undefined
      }
      mutationBusy={resourceMutationBusy || Boolean(appsController.busyID)}
      addBusy={mcpAddBusy}
      addError={mcpAddError}
      deleteError={mcpDeleteError}
      servers={toolServers}
      hasManagedApps={appsController.items.length > 0}
      sourceBusyNames={mcpSourceBusyNames}
      sourceUnavailableNames={mcpSourceUnavailableNames}
      sourceSyncBusyName={mcpSourceSyncBusyName}
      updateAvailableNames={mcpUpdateAvailableNames}
      t={t}
      onOpenAddMCP={() => setAddMCPDialogOpen(true)}
      onUpdateMCP={onUpdateMCPServer}
    />
  );

  return (
    <EntityPane className={"agent-detail-pane"}>
      <div className="agent-profile-fixed-header">
        <AgentProfileHeader
          avatar={
            draft ? (
              <EntityAvatar className={"agent-header-avatar-picker"}>
                <AgentAvatarPicker
                  value={draft.avatar || item.avatar}
                  t={t}
                  mode="edit"
                  onChange={(avatar) => updateDraft({ avatar })}
                />
              </EntityAvatar>
            ) : (
              <EntityAvatar>
                <AgentAvatarContent
                  avatar={item.avatar}
                  fallback={avatarFallbackText(item.avatar, item.name, item.id)}
                />
              </EntityAvatar>
            )
          }
          title={
            <>
              {draft ? (
                canEditAgentName && isEditingName ? (
                  <label className="agent-title-edit-field">
                    <span className="sr-only">{t("agentName")}</span>
                    <TextInput
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
                    title={draft.name || t("agentName")}
                    onClick={() => setIsEditingName(true)}
                  >
                    <span className="agent-title-display-copy">{draft.name || t("agentName")}</span>
                    <span className="agent-title-display-icon" aria-hidden="true">
                      <Edit3 size={16} strokeWidth={1.8} />
                    </span>
                  </button>
                ) : (
                  <h1 title={draft.name || item.name || t("agentName")}>{draft.name || item.name || t("agentName")}</h1>
                )
              ) : (
                <h1 title={item.name || undefined}>{item.name}</h1>
              )}
              <span className={`agent-status-dot ${running ? "online" : ""}`} aria-hidden="true"></span>
              <StatusBadge tone={running ? "positive" : "neutral"} className={`${running ? "online" : ""}`}>
                {statusLabel}
              </StatusBadge>
              {runtimeStatusDetail ? (
                <StatusBadge tone="warning" className={"profile-state-pill warn"}>
                  {runtimeStatusDetail}
                </StatusBadge>
              ) : null}
              {gatewayDegraded ? (
                <StatusBadge tone="warning" className={"profile-state-pill warn"}>
                  {agentGatewayUnavailableLabel(item, t)}
                </StatusBadge>
              ) : null}
              <StatusBadge
                tone={incomplete ? "warning" : "neutral"}
                className={`profile-state-pill ${incomplete ? "warn" : "ready"}`}
              >
                {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
              </StatusBadge>
              {upgradeNeeded ? (
                <StatusBadge tone="warning" className={"profile-state-pill warn"}>
                  {t("profileUpgradeRequired")}
                </StatusBadge>
              ) : null}
              {restartNeeded ? (
                <Button
                  variant="secondaryGray"
                  size="sm"
                  className={"status-pill profile-state-pill profile-recreate-button warn"}
                  title={t("agentRecreate")}
                  disabled={saving || busyKey.startsWith(busyPrefix) || isNotificationBotAgent(item)}
                  onClick={() => setRecreateTarget(item)}
                >
                  {t("profileRestartRequired")}
                </Button>
              ) : null}
            </>
          }
          description={
            draft ? (
              isEditingDescription ? (
                <Field className={"entity-description-field"}>
                  <TextArea
                    aria-label={t("agentDescription")}
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
                </Field>
              ) : (
                <button
                  type="button"
                  className={`entity-description-display ${draft.description ? "" : "is-empty"}`.trim()}
                  aria-label={t("editDescription")}
                  title={draft.description || t("agentDescription")}
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
            ) : null
          }
          actions={
            <>
              {canStartAgent(item) ? (
                <Button
                  variant="secondaryGray"
                  size="md"
                  disabled={busyKey.startsWith(busyPrefix) || incomplete}
                  onClick={() => onStart(item)}
                >
                  <Play aria-hidden="true" size={16} strokeWidth={2} />
                  {t("agentStart")}
                </Button>
              ) : null}
              <Button
                variant={onNewTask ? "primary" : "secondaryGray"}
                size="md"
                disabled={busyKey.startsWith(busyPrefix)}
                title={onNewTask ? t("agentStartTaskHint") : undefined}
                onClick={() => (onNewTask ? onNewTask(item) : onOpenDM(item))}
              >
                {t(onNewTask ? "agentStartTask" : "openDM")}
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
                onInvite={onInvite}
                onDelete={onDelete}
                relayPublication={
                  !agentSandboxEnabled(item) && (runtimeKind === "codex" || runtimeKind === "dsh")
                    ? relayPublication
                    : undefined
                }
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
            </>
          }
        />
        {error ? <InlineError>{error}</InlineError> : null}
        {saveError ? (
          <InlineError className={"whitespace-pre-wrap break-words"} role="alert">
            {saveError}
            {saveBillingURL ? (
              <>
                {" "}
                <a className="agent-billing-action" href={saveBillingURL} target="_blank" rel="noopener noreferrer">
                  {t("rechargeAccount")}
                </a>
              </>
            ) : null}
          </InlineError>
        ) : null}
        {notice ? (
          <div className={`form-warning ${noticeTone === "warning" ? "" : noticeTone}`.trim()} role="status">
            <span>{notice}</span>
            <button
              type="button"
              className={agentDetailPaneStyles["agent-notice-close"]}
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
                    <span
                      className={agentDetailPaneStyles["agent-profile-section-tab-count"]}
                      aria-label={String(section.count)}
                    >
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
          visibleActiveProfileTab === "instructions"
            ? ` ${agentDetailPaneStyles["agent-profile-scroll-region-instructions"]}`
            : ""
        }${isProfileScrolling ? " is-scrolling" : ""}`}
        onScroll={onProfileScroll}
      >
        {!draft ? (
          <>
            <div className={`entity-grid ${agentDetailPaneStyles["entity-grid"]}`}>
              <EntityField label={t("profileRuntimeKind")}>
                <strong>{formatRuntimeKindLabel(runtimeKind, t)}</strong>
              </EntityField>
              <EntityField label={t("profileProvider")}>
                <strong>{formatProviderLabel(provider)}</strong>
              </EntityField>
              <EntityField label={t("profileModel")}>
                <strong>{agentModelID(item)}</strong>
              </EntityField>
              <EntityField label={t("profileReasoning")}>
                <strong>{reasoningEffortLabel(t, item.reasoning_effort || profile?.reasoning_effort)}</strong>
              </EntityField>
              <EntityField label={t("profileFastMode")}>
                <strong>{item.enable_fast_mode || profile?.enable_fast_mode ? "on" : "off"}</strong>
              </EntityField>
            </div>
            <section
              className={`profile-section ${profileFormLayoutStyles["profile-section"]} agent-instructions-section`}
            >
              <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
                {t("agentInstructions")}
              </div>
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
              <div id="agent-profile-profile" className={agentDetailPaneStyles["agent-profile-tab-panel"]}>
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
                    onRequireOpenCSGAuth={onRequireOpenCSGAuth}
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
              />
            ) : null}

            {visibleActiveProfileTab === "instructions" && !isNotifierDraft ? (
              <AgentInstructionsPanel draft={draft} t={t} updateDraft={updateDraft} />
            ) : null}

            {visibleActiveProfileTab === "memory" && item.memory_supported ? (
              <AgentMemoryPanel agentID={String(item.id || "")} onMemoryChange={onMemoryChange} t={t} />
            ) : null}

            {["skills", "mcp", "apps", "knowledge"].includes(visibleActiveProfileTab) && resourceError ? (
              <InlineError role="alert">
                {resourceError}
                {onRetryResource ? (
                  <Button size="sm" disabled={resourceMutationBusy} onClick={() => void onRetryResource()}>
                    {t("retry")}
                  </Button>
                ) : null}
              </InlineError>
            ) : null}
            {visibleActiveProfileTab === "skills" && workspaceSupported ? (
              <AgentSkillsPanel
                continuation={skillContinuation}
                toggleBusyName={resourceBusy.startsWith("skill:") ? resourceBusy.slice(6) : ""}
                onOpenDetail={(skill) => openResourceDetail("skill", skill.name)}
                onToggle={
                  canToggleResources && onSetResourceEnabled
                    ? (skill) => onSetResourceEnabled("skill", skill.name, skill.enabled === false)
                    : undefined
                }
                mutationBusy={resourceMutationBusy || Boolean(appsController.busyID)}
                skillAddBusy={skillAddBusy}
                skillAddError={skillAddError}
                skillDeleteError={skillDeleteError}
                skills={skills}
                skillsError={skillsError}
                skillsLoading={skillsLoading}
                t={t}
                onOpenAddSkills={() => {
                  setAddSkillsDialogOpen(true);
                }}
              />
            ) : null}

            {showMCPServers && visibleActiveProfileTab === "knowledge" ? (
              <AgentMCPPanel
                kind="knowledge"
                continuation={knowledgeContinuation}
                toggleBusyName={resourceBusy.startsWith("mcp:") ? resourceBusy.slice(4) : ""}
                listError={knowledgeListError}
                onOpenDetail={(server) => openResourceDetail("mcp", server.name)}
                onToggle={
                  canToggleResources && onSetResourceEnabled
                    ? (server) => onSetResourceEnabled("mcp", server.name, server.config.enabled === false)
                    : undefined
                }
                mutationBusy={resourceMutationBusy}
                addBusy={mcpAddBusy}
                addError={mcpAddError}
                deleteError={mcpDeleteError}
                servers={knowledgeServers}
                sourceBusyNames={mcpSourceBusyNames}
                sourceUnavailableNames={mcpSourceUnavailableNames}
                sourceSyncBusyName={mcpSourceSyncBusyName}
                updateAvailableNames={mcpUpdateAvailableNames}
                t={t}
                onOpenAddMCP={() => setAddKnowledgeDialogOpen(true)}
                onUpdateMCP={onUpdateMCPServer}
              />
            ) : null}

            {showApps && visibleActiveProfileTab === "apps" ? (
              <AgentAppsPanel
                key={String(item.id)}
                agentID={String(item.id || "")}
                cliSupported={["codex", "dsh"].includes(runtimeKind)}
                controller={appsController}
                t={t}
                portalContainer={dialogPortalContainer}
                selectedID={onProfileTabChange ? requestedAppID : requestedAppID || appSettingsID}
                addAppID={requestedAddAppID}
                onSelect={selectAppSettings}
              >
                {showMCPServers ? toolConnectors : null}
              </AgentAppsPanel>
            ) : showMCPServers && visibleActiveProfileTab === "mcp" ? (
              toolConnectors
            ) : null}
          </div>
        ) : null}
      </div>
      {detailSkill || detailMCP ? (
        <AgentResourceDetails
          key={`${item?.id}:${resourceDetail?.kind}:${resourceDetail?.name}`}
          agentID={String(item?.id || "")}
          skill={detailSkill}
          server={detailMCP}
          t={t}
          portalContainer={dialogPortalContainer}
          busy={resourceMutationBusy || Boolean(appsController.busyID)}
          error={resourceError}
          onRetry={onRetryResource}
          canToggle={canToggleResources && Boolean(onSetResourceEnabled)}
          onClose={() => setResourceDetail(null)}
          onToggle={() => {
            if (resourceDetail)
              void onSetResourceEnabled?.(
                resourceDetail.kind,
                resourceDetail.name,
                detailSkill ? detailSkill.enabled === false : detailMCP?.config.enabled === false,
              );
          }}
          onDelete={() => {
            if (detailSkill) {
              setSkillPendingDelete(detailSkill);
              setDeleteSkillDialogOpen(true);
            } else if (detailMCP) {
              setMCPPendingDelete(detailMCP);
              setDeleteMCPDialogOpen(true);
            }
          }}
        />
      ) : null}
      <DialogRoot open={recreateTarget !== null} onOpenChange={(open) => !open && setRecreateTarget(null)}>
        <DialogContent portalContainer={dialogPortalContainer}>
          <DialogHeader>
            <div>
              <DialogTitle>
                {t("agentRecreateConfirmTitle", { name: recreateTarget?.name || recreateTarget?.id || "" })}
              </DialogTitle>
              <DialogDescription>{t("agentRecreateConfirmMessage")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogFooter>
            <Button variant="secondaryGray" onClick={() => setRecreateTarget(null)}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              disabled={!recreateTarget || saving || busyKey.startsWith(`${recreateTarget.id}:`)}
              onClick={() => {
                if (!recreateTarget) return;
                const target = recreateTarget;
                setRecreateTarget(null);
                void onRecreate(target);
              }}
            >
              {t("agentRecreate")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      {addSkillsDialogOpen && onAddSkills ? (
        <AgentResourcePicker
          key={`${item.id}:skill`}
          agentID={String(item.id)}
          kind="skill"
          authenticated={resourceAuthenticated}
          error={skillAddError}
          onAdd={onAddSkills}
          onClose={() => setAddSkillsDialogOpen(false)}
          onLogin={onRequireOpenCSGAuth}
          portalContainer={dialogPortalContainer}
          t={t}
        />
      ) : null}
      {addKnowledgeDialogOpen && onInstallMCPServers ? (
        <AgentResourcePicker
          key={`${item.id}:knowledge`}
          agentID={String(item.id)}
          kind="knowledge"
          authenticated={resourceAuthenticated}
          error={mcpAddError}
          onAdd={onInstallMCPServers}
          onClose={() => setAddKnowledgeDialogOpen(false)}
          onLogin={onRequireOpenCSGAuth}
          portalContainer={dialogPortalContainer}
          t={t}
        />
      ) : null}
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
          className={`${agentSkillsStyles["agent-skills-dialog"]} ${agentSkillsStyles["agent-skill-delete-dialog"]}`}
          overlayClassName={agentSkillsStyles["agent-skill-delete-backdrop"]}
          portalContainer={dialogPortalContainer}
        >
          <DialogHeader className={agentSkillsStyles["agent-skills-dialog-header"]}>
            <div className={agentSkillsStyles["agent-skills-dialog-copy"]}>
              <DialogTitle>{t("agentDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("agentDeleteSkillConfirmMessage", { name: skillPendingDelete?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {skillDeleteError ? (
            <div
              className={`${agentSkillsStyles["agent-skills-dialog-body"]} ${agentSkillsStyles["agent-skill-delete-dialog-body"]}`}
            >
              <InlineError>{skillDeleteError}</InlineError>
            </div>
          ) : null}
          <div className={agentSkillsStyles["agent-skills-dialog-actions"]}>
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
              {t("confirm")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
      {addMCPDialogOpen && onInstallMCPServers ? (
        <AgentResourcePicker
          key={`${item.id}:mcp`}
          agentID={String(item.id)}
          kind="mcp"
          authenticated={resourceAuthenticated}
          error={mcpAddError}
          onAdd={onInstallMCPServers}
          onClose={() => setAddMCPDialogOpen(false)}
          onLogin={onRequireOpenCSGAuth}
          portalContainer={dialogPortalContainer}
          t={t}
        />
      ) : null}
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
          className={`${agentSkillsStyles["agent-skills-dialog"]} ${agentSkillsStyles["agent-skill-delete-dialog"]}`}
          overlayClassName={agentSkillsStyles["agent-skill-delete-backdrop"]}
          portalContainer={dialogPortalContainer}
        >
          <DialogHeader className={agentSkillsStyles["agent-skills-dialog-header"]}>
            <div className={agentSkillsStyles["agent-skills-dialog-copy"]}>
              <DialogTitle>{t("agentDeleteMCP")}</DialogTitle>
              <DialogDescription>
                {t("agentDeleteMCPConfirmMessage", {
                  name: mcpPendingDelete ? mcpServerDisplayName(mcpPendingDelete) : "",
                })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {mcpDeleteError ? <InlineError>{mcpDeleteError}</InlineError> : null}
          <div className={agentSkillsStyles["agent-skills-dialog-actions"]}>
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
              loadingLabel={t("confirm")}
              disabled={mcpDeleteBusy}
              onClick={handleDeleteMCPConfirm}
            >
              {t("confirm")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
      <DialogRoot open={publishTarget !== null} onOpenChange={(open) => (!open ? setPublishTarget(null) : undefined)}>
        <DialogContent className="agent-publish-dialog" portalContainer={dialogPortalContainer}>
          <DialogHeader>
            <div>
              <DialogTitle>
                {t(publishTarget === "local" ? "agentSaveLocalTemplate" : "agentPublishTemplateTitle")}
              </DialogTitle>
              <DialogDescription>
                {publishTarget !== "local"
                  ? t("agentPublishTemplateCommunitySubtitle")
                  : t("agentPublishTemplateLocalSubtitle")}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogBody className={agentPublicationStyles["agent-publish-dialog-body"]}>
            <section
              className={agentPublicationStyles["agent-publish-section"]}
              aria-label={t("agentPublishTemplateDetails")}
            >
              <h3>{t("agentPublishTemplateDetails")}</h3>
              <div className={agentPublicationStyles["agent-publish-name-fields"]}>
                <Field
                  className={agentPublicationStyles["agent-publish-field"]}
                  label={t("agentPublishTemplateName")}
                  hint={t(linkedTemplate ? "agentPublishTemplateNameReadonlyHint" : "agentPublishTemplateNameHint")}
                  error={publishTemplateNameError || undefined}
                >
                  <TextInput
                    size="sm"
                    value={publishTemplateName}
                    readOnly={Boolean(linkedTemplate)}
                    maxLength={TEMPLATE_NAME_MAX_LENGTH}
                    aria-label={t("agentPublishTemplateName")}
                    onChange={(event) => {
                      setPublishTemplateName(event.target.value);
                      setPublishTemplateNameError("");
                      setPublishAttempted(false);
                    }}
                  />
                </Field>
                <Field
                  className={agentPublicationStyles["agent-publish-field"]}
                  label={t("agentPublishTemplateDisplayName")}
                  hint={t("agentPublishTemplateDisplayNameHint")}
                >
                  <TextInput
                    size="sm"
                    value={publishTemplateDisplayName}
                    maxLength={32}
                    aria-label={t("agentPublishTemplateDisplayName")}
                    onChange={(event) => setPublishTemplateDisplayName(event.target.value)}
                  />
                </Field>
              </div>
              <Field
                className={agentPublicationStyles["agent-publish-field"]}
                label={t("agentPublishTemplateDescription")}
              >
                <TextArea
                  className={agentPublicationStyles["agent-publish-description"]}
                  rows={3}
                  value={publishTemplateDescription}
                  aria-label={t("agentPublishTemplateDescription")}
                  onChange={(event) => setPublishTemplateDescription(event.target.value)}
                />
              </Field>
            </section>
            <section
              className={agentPublicationStyles["agent-publish-section"]}
              aria-label={t("agentPublishTemplateContents")}
            >
              <h3>{t("agentPublishTemplateContents")}</h3>
              {linkedTemplate?.revision ? (
                <p className={agentPublicationStyles["agent-publish-version-note"]}>
                  {t("agentTemplateNextVersion", {
                    version: (linkedTemplate.latest_revision || linkedTemplate.revision) + 1,
                  })}
                </p>
              ) : null}
              <Field className={agentPublicationStyles["agent-publish-field"]} label={t("templateCommitMessage")}>
                <TextArea
                  className={agentPublicationStyles["agent-publish-commit-message"]}
                  rows={2}
                  value={publishCommitMessage}
                  aria-label={t("templateCommitMessage")}
                  onChange={(event) => setPublishCommitMessage(event.target.value)}
                />
              </Field>
              {supportsTemplateMemory ? (
                <label className={agentPublicationStyles["agent-publish-memory-option"]}>
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
            </section>
            {publishTarget === "official" ? (
              <p className={agentPublicationStyles["agent-publish-hub-note"]}>
                <UploadCloud size={16} aria-hidden="true" />
                <span>{t("agentTemplateHubGuidance")}</span>
              </p>
            ) : null}
            {publishTarget === "official" && publishDisabled ? (
              <p role="status" className="text-sm text-[var(--muted)]">
                {t("agentPublishLoginRequired")}
              </p>
            ) : null}
            {publishAttempted && publishError ? (
              <InlineError className={agentPublicationStyles["agent-publish-error"]}>{publishError}</InlineError>
            ) : null}
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
            ) : publishDisabled ? (
              <Button
                variant="primary"
                size="md"
                disabled={publishBusy || !onPublishLogin}
                onClick={() => {
                  setPublishTarget(null);
                  onPublishLogin?.();
                }}
              >
                {t("agentTemplateLoginToPublish")}
              </Button>
            ) : (
              <>
                <Button
                  variant="primary"
                  size="md"
                  loading={publishBusy}
                  loadingLabel={t("agentPublishing")}
                  disabled={publishBusy}
                  onClick={() => void submitPublishTemplate("official")}
                >
                  {t("agentSaveAndPublishTemplate")}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </EntityPane>
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

  return (
    <section
      id="agent-profile-runtime"
      className={`profile-section ${profileFormLayoutStyles["profile-section"]} ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
    >
      <div className="profile-section-heading">
        <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
          {t("profileRuntimeSection")}
        </div>
        <p className="profile-section-description">{t("profileRuntimeSectionDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div
          className={`profile-grid-compact ${profileFormLayoutStyles["profile-grid-compact"]} agent-page-form-content`}
        >
          <Field label={t("profileRuntimeKind")} className={"col-span-full"}>
            <TextInput value={formatRuntimeKindLabel(draft.runtime_kind || runtimeKind, t)} readOnly disabled />
          </Field>
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
  kind?: "mcp" | "knowledge";
  embedded?: boolean;
  toggleBusyName: string;
  continuation?: ResourceContinuation;
  listError?: string;
  onOpenDetail: (item: MCPServer) => void;
  onToggle?: (item: MCPServer) => Promise<void>;
  mutationBusy: boolean;
  hasManagedApps?: boolean;
  addBusy: boolean;
  addError: string;
  deleteError: string;
  onOpenAddMCP: () => void;
  onUpdateMCP?: (server: MCPServer) => Promise<boolean> | boolean;
  servers: readonly MCPServer[];
  sourceBusyNames: ReadonlySet<string>;
  sourceUnavailableNames: ReadonlySet<string>;
  sourceSyncBusyName: string;
  updateAvailableNames: ReadonlySet<string>;
  t: TranslateFn;
};

function AgentMCPPanel({
  kind = "mcp",
  embedded = false,
  toggleBusyName,
  continuation,
  listError,
  onOpenDetail,
  onToggle,
  mutationBusy,
  hasManagedApps = false,
  addBusy,
  addError,
  deleteError,
  onOpenAddMCP,
  onUpdateMCP,
  servers,
  sourceBusyNames,
  sourceUnavailableNames,
  sourceSyncBusyName,
  updateAvailableNames,
  t,
}: AgentMCPPanelProps) {
  return (
    <section
      id={kind === "knowledge" ? "agent-profile-knowledge" : "agent-profile-mcp"}
      className={
        embedded
          ? `${agentSkillsStyles["agent-skills-section"]} agent-mcp-section`
          : `profile-section ${profileFormLayoutStyles["profile-section"]} ${agentSkillsStyles["agent-skills-section"]} agent-mcp-section ${agentDetailPaneStyles["agent-profile-scroll-target"]}`
      }
    >
      <div className={agentSkillsStyles["agent-skills-summary-heading"]}>
        <div className="profile-section-heading">
          <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
            {t(kind === "knowledge" ? "profileKnowledgeTitle" : "connectorsTools")}
          </div>
          <p className="profile-section-description">
            {t(kind === "knowledge" ? "profileKnowledgeHint" : "profileMCPHint")}
          </p>
        </div>
        <div className={agentSkillsStyles["agent-skills-summary-actions"]}>
          <span className={agentSkillsStyles["agent-skills-summary-count"]}>
            {t(kind === "knowledge" ? "profileKnowledgeCount" : "agentMCPCount", {
              count: continuation?.total ?? servers.length,
            })}
          </span>
          {servers.length > 0 ? (
            <Tooltip content={t(kind === "knowledge" ? "agentKnowledgeAdd" : "agentMCPAdd")}>
              <span>
                <Button
                  className="agent-skill-add-button"
                  variant="secondaryGray"
                  size="sm"
                  aria-label={t(kind === "knowledge" ? "agentKnowledgeAdd" : "agentMCPAdd")}
                  disabled={mutationBusy || addBusy}
                  onClick={onOpenAddMCP}
                >
                  <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
                </Button>
              </span>
            </Tooltip>
          ) : null}
        </div>
      </div>
      {addError ? <InlineError>{addError}</InlineError> : null}
      {deleteError ? <InlineError>{deleteError}</InlineError> : null}
      {listError ? (
        <InlineError role="alert">
          {listError}{" "}
          {continuation ? (
            <Button size="sm" onClick={continuation.retry}>
              {t("retry")}
            </Button>
          ) : null}
        </InlineError>
      ) : null}
      {!continuation?.loading && !listError && !servers.length ? (
        <div className="agent-skills-summary-empty">
          <span className={agentSkillsStyles["agent-skills-summary-icon"]} aria-hidden="true">
            {kind === "knowledge" ? <BookOpen size={18} /> : <Server size={18} strokeWidth={1.8} />}
          </span>
          <div className={agentSkillsStyles["agent-skills-summary-empty-copy"]}>
            <strong>
              {t(
                kind === "knowledge" ? "profileKnowledgeEmpty" : hasManagedApps ? "appManualMCPEmpty" : "agentMCPEmpty",
              )}
            </strong>
            <p>{t(kind === "knowledge" ? "profileKnowledgeEmptyHint" : "agentMCPEmptyHint")}</p>
          </div>
          <Button variant="secondaryGray" size="sm" disabled={mutationBusy || addBusy} onClick={onOpenAddMCP}>
            <Plus aria-hidden="true" size={15} strokeWidth={2.2} />
            {t(kind === "knowledge" ? "agentKnowledgeAdd" : "agentMCPAdd")}
          </Button>
        </div>
      ) : null}
      {servers.length ? (
        <ResourceList>
          {servers.map((server) => {
            const sourceUnavailable = sourceUnavailableNames.has(server.name);
            const updateAvailable = updateAvailableNames.has(server.name);
            const syncBusy = sourceSyncBusyName === server.name;
            return (
              <ResourceListCard
                key={server.name}
                title={mcpServerDisplayName(server)}
                description={server.description}
                icon={
                  <ResourceFeaturedIcon>
                    {kind === "knowledge" ? <BookOpen size={16} /> : <SidebarMcpIcon size={16} />}
                  </ResourceFeaturedIcon>
                }
                onOpen={() => onOpenDetail(server)}
                badge={
                  <>
                    {sourceUnavailable ? (
                      <span>{t("agentKnowledgeMCPSourceDeleted")}</span>
                    ) : updateAvailable ? (
                      <span>{t("agentKnowledgeMCPUpdateAvailable")}</span>
                    ) : null}
                  </>
                }
                actions={
                  <>
                    <ResourceStatus status={server.config.enabled === false ? "disabled" : "enabled"}>
                      {t(server.config.enabled === false ? "appStatusDisabled" : "profileResourceEnabled")}
                    </ResourceStatus>
                    {updateAvailable && !sourceUnavailable && onUpdateMCP ? (
                      <Button
                        size="sm"
                        loading={syncBusy}
                        disabled={mutationBusy || sourceBusyNames.has(server.name)}
                        onClick={() => void onUpdateMCP(server)}
                        aria-label={t("agentMCPUpdateConfig")}
                      >
                        <RefreshCw size={14} />
                      </Button>
                    ) : null}
                    {toggleBusyName === server.name ? <span role="status">{t("resourceUpdating")}</span> : null}
                    <Switch
                      aria-busy={toggleBusyName === server.name}
                      className={agentSkillsStyles["agent-skill-card-switch-sm"]}
                      aria-label={mcpServerDisplayName(server)}
                      checked={server.config.enabled !== false}
                      disabled={mutationBusy || !onToggle}
                      onCheckedChange={() => void onToggle?.(server)}
                    />
                  </>
                }
              />
            );
          })}
        </ResourceList>
      ) : null}
      <ResourceLoadMore continuation={continuation} showSummary={false} t={t} />
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
    <div id="agent-profile-model" className={agentDetailPaneStyles["agent-profile-scroll-target"]}>
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
  const renderedMemory = useMemo(() => renderMarkdown(document?.content || ""), [document?.content]);

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
    <div
      id="agent-profile-memory"
      className={`${agentMemoryStyles["agent-memory-tab"]} ${agentDetailPaneStyles["agent-profile-tab-panel"]} ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
    >
      <section
        className={`profile-section ${profileFormLayoutStyles["profile-section"]} ${agentSkillsStyles["agent-skills-section"]} agent-memory-settings-section`}
      >
        <div className="agent-memory-section-heading">
          <div className="profile-section-heading">
            <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
              {t("agentMemoryTitle")}
            </div>
            <p className="profile-section-description">{t("agentMemoryDescription")}</p>
          </div>
          <div className="agent-memory-setting-actions">
            <span className={agentMemoryStyles["agent-memory-status"]}>
              {document?.enabled ? t("statusEnabled") : t("statusDisabled")}
            </span>
            <Switch
              size="sm"
              checked={document?.enabled ?? false}
              disabled={!document || loading || updating}
              aria-label={t("agentMemoryEnabled")}
              onCheckedChange={(checked) => void setMemoryEnabled(checked)}
            />
          </div>
        </div>
        <p className={agentMemoryStyles["agent-memory-toggle-hint"]}>{t("agentMemoryToggleHint")}</p>
      </section>

      <section
        className={`profile-section ${profileFormLayoutStyles["profile-section"]} ${agentSkillsStyles["agent-skills-section"]} agent-memory-document-section`}
      >
        <div className="agent-memory-section-heading">
          <div className="profile-section-heading">
            <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
              {document?.name || t("agentMemoryDocumentTitle")}
            </div>
            <p className="profile-section-description">{t("agentMemoryDocumentDescription")}</p>
            {document?.location ? (
              <code className={agentMemoryStyles["agent-memory-location"]}>{document.location}</code>
            ) : null}
          </div>
          <div className={agentSkillsStyles["agent-skills-summary-actions"]}>
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

        {loadError ? <InlineError>{loadError}</InlineError> : null}
        {loading ? (
          <div className="agent-skills-summary-empty agent-memory-summary-empty" role="status">
            <span className={agentSkillsStyles["agent-skills-summary-icon"]} aria-hidden="true">
              <CircleDashed size={18} strokeWidth={1.8} />
            </span>
            <div className={agentSkillsStyles["agent-skills-summary-empty-copy"]}>
              <strong>{t("agentMemoryLoading")}</strong>
            </div>
          </div>
        ) : document?.ready ? (
          <div className="agent-section-form agent-memory-document-shell">
            <ClipboardCopyButton
              className="agent-memory-copy-button"
              copiedIcon={<Check aria-hidden="true" size={16} strokeWidth={2.2} />}
              copiedLabel={t("copiedToClipboard")}
              icon={<Copy aria-hidden="true" size={16} strokeWidth={2.2} />}
              iconOnly
              label={t("copyToClipboard")}
              text={document.content || ""}
            />
            <div
              className={`${agentMemoryStyles["agent-memory-document"]} message-content`}
              role="region"
              aria-label={t("agentMemoryDocumentLabel")}
              dangerouslySetInnerHTML={{ __html: renderedMemory }}
            />
          </div>
        ) : (
          <div className="agent-skills-summary-empty agent-memory-summary-empty">
            <span className={agentSkillsStyles["agent-skills-summary-icon"]} aria-hidden="true">
              <FileCode2 size={18} strokeWidth={1.8} />
            </span>
            <div className={agentSkillsStyles["agent-skills-summary-empty-copy"]}>
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
      className={`profile-section ${profileFormLayoutStyles["profile-section"]} agent-instructions-section ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
    >
      <div className={agentInstructionsStyles["agent-instructions-header"]}>
        <div className="profile-section-heading">
          <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
            {t("agentInstructions")}
          </div>
          <p className="profile-section-description">
            {mode === "default" ? t("agentInstructionsDefaultHint") : t("agentInstructionsAdvancedHint")}
          </p>
        </div>
        <SegmentedControl
          value={mode}
          onChange={setMode}
          label={t("agentInstructionsViewMode")}
          size="sm"
          options={[
            { value: "default", label: t("agentInstructionsDefaultMode") },
            { value: "advanced", label: t("agentInstructionsAdvancedMode") },
          ]}
        />
      </div>
      <div className={agentInstructionsStyles["agent-instructions-content"]}>
        <div className={`profile-grid-compact ${profileFormLayoutStyles["profile-grid-compact"]}`}>
          <Field className={`span-2 ${profileFormLayoutStyles["span-2"]}`}>
            <TextArea
              aria-label={mode === "advanced" ? t("agentInstructionsEffective") : t("agentInstructions")}
              className={`compact-textarea ${agentInstructionsStyles["agent-instructions-editor"]}`}
              value={mode === "advanced" ? effective : draft.instructions || ""}
              onInput={(event) =>
                mode === "advanced"
                  ? setEffective(event.currentTarget.value)
                  : updateDraft({ instructions: event.currentTarget.value })
              }
              placeholder={t("agentInstructionsPlaceholder")}
            />
          </Field>
        </div>
      </div>
      {advancedError ? <InlineError>{advancedError}</InlineError> : null}
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
  toggleBusyName: string;
  continuation?: ResourceContinuation;
  onOpenDetail: (item: SlashSkillOption) => void;
  onToggle?: (item: SlashSkillOption) => Promise<void>;
  mutationBusy: boolean;
  onOpenAddSkills: () => void;
  skillAddBusy: boolean;
  skillAddError: string;
  skillDeleteError: string;
  skills: readonly SlashSkillOption[];
  skillsError: string;
  skillsLoading: boolean;
  t: TranslateFn;
};

function AgentSkillsPanel({
  toggleBusyName,
  continuation,
  onOpenDetail,
  onToggle,
  mutationBusy,
  onOpenAddSkills,
  skillAddBusy,
  skillAddError,
  skillDeleteError,
  skills,
  skillsError,
  skillsLoading,
  t,
}: AgentSkillsPanelProps) {
  return (
    <section
      id="agent-profile-skills"
      className={`profile-section ${profileFormLayoutStyles["profile-section"]} ${agentSkillsStyles["agent-skills-section"]} ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
    >
      <div className={agentSkillsStyles["agent-skills-summary-heading"]}>
        <div className="profile-section-heading">
          <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
            {t("agentSkillsTitle")}
          </div>
          <p className="profile-section-description">{t("agentSkillsDescription")}</p>
        </div>
        <div className={agentSkillsStyles["agent-skills-summary-actions"]}>
          <span className={agentSkillsStyles["agent-skills-summary-count"]}>
            {t("agentSkillsCount", { count: continuation?.total ?? skills.length })}
          </span>
          <Tooltip content={t("agentSkillAdd")}>
            <span>
              <Button
                className="agent-skill-add-button"
                variant="secondaryGray"
                size="sm"
                aria-label={t("agentSkillAdd")}
                disabled={mutationBusy || skillAddBusy}
                onClick={onOpenAddSkills}
              >
                <Plus aria-hidden="true" size={16} strokeWidth={2.2} />
              </Button>
            </span>
          </Tooltip>
        </div>
      </div>
      {skillsError ? (
        <DismissibleInlineError
          className={`${agentSkillsStyles["agent-skills-error"]}`}
          messageKey={skillsError}
          closeLabel={t("close")}
        >
          <span role="alert">{skillsError}</span>
          {continuation ? (
            <Button size="sm" onClick={continuation.retry}>
              {t("retry")}
            </Button>
          ) : null}
        </DismissibleInlineError>
      ) : null}
      {skillAddError ? <InlineError>{skillAddError}</InlineError> : null}
      {skillDeleteError ? <InlineError>{skillDeleteError}</InlineError> : null}
      {skillsLoading ? (
        <div className="agent-skills-summary-empty">
          <span className={agentSkillsStyles["agent-skills-summary-icon"]} aria-hidden="true">
            <FileCode2 size={18} strokeWidth={1.8} />
          </span>
          <div>
            <strong>{t("agentSkillsLoading")}</strong>
          </div>
        </div>
      ) : null}
      {!skillsLoading && !skills.length ? (
        <div className="agent-skills-summary-empty">
          <span className={agentSkillsStyles["agent-skills-summary-icon"]} aria-hidden="true">
            <FileCode2 size={18} strokeWidth={1.8} />
          </span>
          <div className={agentSkillsStyles["agent-skills-summary-empty-copy"]}>
            <strong>{t("agentSkillsEmpty")}</strong>
            <p>{t("agentSkillsEmptyHint")}</p>
          </div>
          <Button variant="secondaryGray" size="sm" disabled={mutationBusy || skillAddBusy} onClick={onOpenAddSkills}>
            <Plus aria-hidden="true" size={15} strokeWidth={2.2} />
            {t("agentSkillAdd")}
          </Button>
        </div>
      ) : null}
      {!skillsLoading && skills.length ? (
        <div className={`hub-skill-card-grid ${agentDetailPaneStyles["agent-profile-skill-card-grid"]}`}>
          {skills.map((skill) => (
            <ResourceListCard
              key={skill.name}
              title={skill.name}
              description={skill.description}
              icon={<AgentSkillFeaturedIcon />}
              onOpen={() => onOpenDetail(skill)}
              actions={
                <>
                  <ResourceStatus status={skill.enabled === false ? "disabled" : "enabled"}>
                    {skill.enabled === false ? t("appStatusDisabled") : t("resourcesSkillEnabled")}
                  </ResourceStatus>
                  <Switch
                    aria-busy={toggleBusyName === skill.name}
                    className={agentSkillsStyles["agent-skill-card-switch-sm"]}
                    aria-label={skill.name}
                    checked={skill.enabled !== false}
                    disabled={mutationBusy || !onToggle}
                    onCheckedChange={() => void onToggle?.(skill)}
                  />
                </>
              }
            />
          ))}
        </div>
      ) : null}
      <ResourceLoadMore continuation={continuation} showSummary={false} t={t} />
    </section>
  );
}

function AgentSkillFeaturedIcon() {
  return (
    <ResourceFeaturedIcon>
      <SidebarPuzzlePiece02Icon size={16} aria-hidden="true" />
    </ResourceFeaturedIcon>
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
    <section
      id="agent-profile-advanced"
      className={`profile-section ${profileFormLayoutStyles["profile-section"]} agent-advanced-section ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
    >
      <div className="profile-section-heading">
        <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
          {t("profileAdvanced")}
        </div>
        <p className="profile-section-description">{t("profileAdvancedDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div
          className={`profile-advanced-grid ${profileFormLayoutStyles["profile-advanced-grid"]} agent-page-form-content`}
        >
          {!isNotifierRuntimeDraftOnAgentPage(draft, item) ? (
            <Field label={t("profileRequestOptions")} className={"profile-json-field"}>
              <TextArea
                className="compact-json"
                value={draft.requestOptionsText}
                onInput={(event) => updateDraft({ requestOptionsText: event.currentTarget.value })}
              />
            </Field>
          ) : null}
          <EnvKeyValueEditor
            className="profile-env-field"
            rows={draft.envRows}
            t={t}
            onChange={(rows) => updateDraft({ envRows: rows })}
          />
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
  const disconnectLabel = t("feishuDisconnect");
  const canDisconnect = connected && Boolean(onDisconnectFeishu);
  const connectURL = String(pendingRegistration?.connect_url || "").trim();

  return (
    <section
      id="agent-profile-channels"
      className={`profile-section ${profileFormLayoutStyles["profile-section"]} ${agentChannelsStyles["agent-channels-section"]} ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
      aria-label={t("agentChannelsTitle")}
    >
      <div className="profile-section-heading">
        <h2
          id="agent-channels-title"
          className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]} agent-channels-title`}
        >
          {t("agentChannelsTitle")}
        </h2>
        <p className="profile-section-description">{t("agentChannelsDescription")}</p>
      </div>
      <div className={agentChannelsStyles["agent-channel-row"]}>
        <span className={agentChannelsStyles["agent-channel-icon"]} aria-hidden="true">
          <img src="icons/feishu.png" alt="" />
        </span>
        <span className={agentChannelsStyles["agent-channel-main"]}>
          <span className={agentChannelsStyles["agent-channel-name"]}>{t("feishuChannelName")}</span>
          <span
            className={`${agentChannelsStyles["agent-channel-status"]} ${connected ? "connected" : pending ? "pending" : ""}`.trim()}
          >
            {statusIcon}
            {statusLabel}
          </span>
          {pending ? (
            <span className={agentChannelsStyles["agent-channel-detail"]}>{t("feishuPendingDetail")}</span>
          ) : null}
        </span>
        <span className={agentChannelsStyles["agent-channel-actions"]}>
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
              loadingLabel={disconnectLabel}
              disabled={!canDisconnect || actionBusy}
              onClick={() => onDisconnectFeishu?.(item)}
            >
              <Unlink2 aria-hidden="true" size={15} strokeWidth={2} />
              {disconnectLabel}
            </Button>
          ) : null}
        </span>
      </div>
    </section>
  );
}

type AgentActionsMenuProps = {
  relayPublication?: RelayPublicationController;
  activeRoom?: IMConversation | null;
  busy: boolean;
  canPublishLocal: boolean;
  canPublishCommunity: boolean;
  incomplete: boolean;
  isManager: boolean;
  item: AgentLike;
  onDelete: AgentDetailPaneProps["onDelete"];
  onInvite: AgentActionHandler;
  onPublish?: (target: AgentTemplatePublishTarget) => VoidOrPromise;
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
  onInvite,
  onDelete,
  onPublish,
  relayPublication,
}: AgentActionsMenuProps) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const hasOtherActions = SHOW_AGENT_LIFECYCLE_ACTIONS || canPublishLocal || !isManager;

  const renderMenu = (relayAction?: React.ReactNode) => (
    <DropdownMenuRoot>
      <DropdownMenuTrigger asChild>
        <Button
          ref={triggerRef}
          variant="secondaryGray"
          size="md"
          className={agentDetailPaneStyles["agent-actions-menu-trigger"]}
        >
          <MoreHorizontal aria-hidden="true" size={18} strokeWidth={2} />
          <span>{t("agentMoreActions")}</span>
          {upgradeNeeded ? (
            <span className={agentDetailPaneStyles["agent-actions-alert-dot"]} aria-hidden="true"></span>
          ) : null}
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
        {SHOW_AGENT_LIFECYCLE_ACTIONS && activeRoom && !isManager ? (
          <DropdownMenuItem disabled={busy} onSelect={() => onInvite(item)}>
            <UserPlus aria-hidden="true" size={15} strokeWidth={2} />
            <span>{t("inviteToRoom")}</span>
          </DropdownMenuItem>
        ) : null}
        {relayAction}
        {canPublishLocal ? (
          <>
            <DropdownMenuItem disabled={publishBusy} onSelect={() => onPublish?.("local")}>
              <Save aria-hidden="true" size={15} strokeWidth={2} />
              <span>{publishBusy ? t("agentPublishing") : t("agentSaveLocalTemplate")}</span>
            </DropdownMenuItem>
            {canPublishCommunity ? (
              <DropdownMenuItem
                disabled={publishBusy}
                title={publishDisabled ? t("agentPublishLoginRequired") : undefined}
                onSelect={() => onPublish?.("official")}
              >
                <UploadCloud aria-hidden="true" size={15} strokeWidth={2} />
                <span>{publishBusy ? t("agentPublishing") : t("agentSaveAndPublishTemplate")}</span>
              </DropdownMenuItem>
            ) : null}
          </>
        ) : null}
        {!isManager ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              danger={isNotificationBotAgent(item)}
              disabled={busy}
              onSelect={() => onDelete(item, triggerRef.current)}
            >
              {isNotificationBotAgent(item) ? (
                <Trash2 aria-hidden="true" size={15} strokeWidth={2} />
              ) : (
                <Archive aria-hidden="true" size={15} strokeWidth={2} />
              )}
              <span>{t(isNotificationBotAgent(item) ? "agentDelete" : "agentArchive")}</span>
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenuRoot>
  );
  return relayPublication ? (
    <RelayPublication
      key={String(item.id)}
      controller={relayPublication}
      t={t}
      renderAction={(action) => (hasOtherActions || action ? renderMenu(action) : null)}
    />
  ) : (
    hasOtherActions && renderMenu()
  );
}
