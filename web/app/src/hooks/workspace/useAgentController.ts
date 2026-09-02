import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker } from "react-router-dom";
import { apiErrorBillingURL, errorMessage as apiErrorMessage, type ApiError } from "@/api/client";
import { loginCLIProxyProviderRequest } from "@/api/cliproxy";
import {
  batchAddAgentMCPServersRequest,
  batchDeleteAgentMCPServersRequest,
  batchAddAgentSkillsRequest,
  createBotRequest,
  createManagerAgentRequest,
  createNotificationBotRequest,
  deleteAgentLikeRequest,
  deleteAgentSkillRequest,
  deleteBotRequest,
  deleteFeishuParticipantRequest,
  fetchAgent,
  fetchAgentProfile,
  fetchAgentProfileDefaults,
  fetchAgentMCPServers,
  fetchAgentSkills,
  fetchAgentSkillsFile,
  finalizeFeishuRegistrationRequest,
  initAgentLarkCLIRequest,
  patchNotificationBotRequest,
  runAgentActionRequest,
  startFeishuRegistrationRequest,
  updateAgentRequest,
} from "@/api/agents";
import type {
  AgentUpdatePayload,
  FeishuRegistration,
  FeishuRegistrationFinalizeResult,
  FetchAgentsOptions,
} from "@/api/agents";
import { fetchAgentMCPServerSourceStatus, syncAgentMCPServerSource } from "@/api/mcp";
import { patchCsgclawUserRequest } from "@/api/participants";
import { publishAgentTemplateRequest, type AgentTemplatePublishTarget } from "@/api/hub";
import { HubTemplateErrorCodes, hubTemplateErrorCode, upsertHubTemplateReviewState } from "@/models/hubWorkspace";
import type { HubTemplate } from "@/models/hubWorkspace";
import { createUserRequest, joinAgentToRoomRequest } from "@/api/im";
import { fetchSkills } from "@/api/skills";
import { createTeamRequest, deleteTeamRequest, fetchTeams, updateTeamRequest } from "@/api/tasks";
import type { CreateTeamPayload } from "@/api/tasks";
import {
  BOT_CREATE_KIND_NOTIFICATION,
  BOT_CREATE_KIND_WORKER,
  BOT_TYPE_NORMAL,
  BOT_TYPE_NOTIFICATION,
  DEFAULT_RUNTIME_KIND,
  MANAGER_AGENT_ID,
  MANAGER_AGENT_ROLE,
  WORKER_AGENT_ROLE,
} from "@/shared/constants/agents";
import { ACTION_REBUILD_MANAGER } from "@/shared/constants/messages";
import { selectUnusedAgentAvatar } from "@/shared/avatarOptions";
import { FEISHU_REGISTRATIONS_STORAGE_KEY } from "@/shared/storage/keys";
import { localizeAPIError } from "@/shared/i18n";
import { LAST_CREATED_AGENT_MODEL_STORAGE_KEY } from "@/shared/storage/keys";
import {
  applyTemplateToDraft,
  advanceAgentProgress,
  agentOfflineReasonLabel,
  agentRuntimeKind,
  agentRuntimeState,
  agentDraftMissingRequiredEnv,
  agentDraftWithRuntimeFieldsFromAgent,
  agentPageLLMProfileChanged,
  agentRuntimePollSettled,
  agentToDraft,
  isAgentProfileDraftComplete,
  isAgentProfileMarkedComplete,
  composeLegacyRuntimeKind,
  defaultWorkerImageForRuntime,
  draftMCPServersForSave,
  draftRuntimeOptionsForSave,
  draftToProfileComparePayload,
  draftToProfile,
  ensureNotifierPullSubscriptionDraft,
  feishuAgentParticipant,
  isAgentRuntimeUnavailable,
  isAgentRunning,
  isManagerAgent,
  isNotificationBotAgent,
  isNotificationBotDraftContext,
  isNotifierRuntimeDraft,
  isNotifierRuntimeDraftOnAgentPage,
  notifierFormIsComplete,
  mergeAgentIntoList,
  normalizeAuthProviderName,
  normalizeRuntimeName,
  normalizeTemplateSelection,
  partitionWorkspaceAgentItems,
  pickDefaultAgentTemplate,
  resolveAgentAvatarSource,
  resolveAgentAvatarUserID,
  normalizeRuntimeKind,
  profileSelectorFromDraft,
  providerNeedsAuth,
  resolvedNotifierWebhookOrigin,
  resolveRuntimeSelection,
  resolveAgentChannelUserID,
  shouldWaitForManagerRuntimeAfterProfileSave,
  startAgentCreateProgress,
  workerSelectableTemplates,
} from "@/models/agents";
import type {
  AgentCreateProgressState,
  AgentDraft,
  AgentLike,
  AgentProfileLike,
  AgentTemplateLike,
  RuntimeBootstrapConfig,
} from "@/models/agents";
import { isDirectConversation, localIdentitiesMatch, upsertUserInData } from "@/models/conversations";
import { mcpManagedKnowledgeBaseSource, mcpServersFromMap } from "@/models/mcp";
import type { MCPServerSourceStatus } from "@/models/mcp";
import { displayTeam, teamMemberIDs } from "@/models/tasks";
import type { WorkspaceTeam } from "@/models/tasks";
import {
  modelProviderCatalogForAgentAvailability,
  modelProviderCatalogWithModels,
  modelProviderOptionsFromCatalog,
  providerNameForProviderID,
} from "@/models/modelProviders";
import type { ModelProviderOption } from "@/models/modelProviders";
import { WorkspacePaneTypes } from "@/models/routing";
import { skillDescriptionFromMarkdown, skillOptionsFromWorkspace } from "@/models/slashCommands";
import { useCLIProxyAuthStatuses } from "./useCLIProxyAuthStatuses";
import { workspaceQueryKeys } from "./workspaceQueries";
import type { MessageAction, MessageActionFeedback, MessageLike } from "@/components/business/MessageContent/types";
import type { IMConversation, IMUser } from "@/models/conversations";
import type { UseAgentControllerArgs } from "./types";
import { useProfileModelOptions } from "./useProfileModelOptions";

const SANDBOX_RUNTIME_REFRESH_INTERVAL_MS = 3000;

type AgentModalMode = "create" | "edit";
type AgentAction = "delete" | "recreate" | "start" | "stop" | "upgrade";

type FeishuPendingRegistration = FeishuRegistration & {
  agent_id: string;
  registration_id: string;
};

function runtimeChoicesFromBootstrapConfig(config: RuntimeBootstrapConfig | null | undefined) {
  return Array.isArray(config?.worker_runtime_choices) ? config.worker_runtime_choices : [];
}

function hasUnavailableSandboxRuntimeChoice(config: RuntimeBootstrapConfig | null | undefined): boolean {
  return runtimeChoicesFromBootstrapConfig(config).some((item) => item?.sandbox_enabled && item?.installed === false);
}

type AgentCreateMode = "template" | "custom";

type LastCreatedAgentModelPreference = {
  modelID: string;
  providerID: string;
};

type AgentWithProfile = {
  agent: AgentLike;
  profile?: AgentProfileLike | null;
};

type AgentPageDraftData = {
  agent: AgentLike;
  draft: AgentDraft;
};

type AgentPageNoticeTone = "info" | "warning" | "success";
type AgentPageNoticeState = {
  message: string;
  tone: AgentPageNoticeTone;
};
type LarkCLIDialogState = {
  kind: "message" | "install";
  message: string;
  title: string;
};
type AgentActionBusyEntry = {
  busyKey: string;
  visible: boolean;
};
type AgentActionBusyState = Record<string, AgentActionBusyEntry>;
type FeishuActionKind = "connect" | "disconnect" | "finalize" | "lark-cli";

export function feishuRegistrationFinalizeNotice(result: FeishuRegistrationFinalizeResult | null | undefined): {
  kind: "success" | "unavailable" | "bind_failed" | "error" | "warnings";
  warnings: string[];
} {
  const code = String(result?.lark_cli_error?.code || "").trim();
  if (code === "lark_cli_unavailable") {
    return { kind: "unavailable", warnings: [] };
  }
  if (code === "lark_cli_bind_failed") {
    return { kind: "bind_failed", warnings: [] };
  }
  if (code) {
    return { kind: "error", warnings: [] };
  }
  const warnings = (result?.warnings || []).map((warning) => String(warning || "").trim()).filter(Boolean);
  return { kind: warnings.length > 0 ? "warnings" : "success", warnings };
}

const AGENT_RUNTIME_SYNC_INTERVAL_MS = 2_000;
const AGENT_RUNTIME_SYNC_TIMEOUT_MS = 120_000;
const GLOBAL_AGENT_PAGE_NOTICE_KEY = "__global__";

function cloneMCPServersForDraft(servers: AgentDraft["mcpServers"]): AgentDraft["mcpServers"] {
  return servers && typeof servers === "object" ? { ...servers } : servers;
}
const FEISHU_CHANNEL_ACTION = "feishu";
const FEISHU_REGISTRATION_DEFAULT_POLL_SECONDS = 3;
const FEISHU_REGISTRATION_MIN_POLL_SECONDS = 1;
const FEISHU_REGISTRATION_MAX_POLL_SECONDS = 30;
const AGENT_CREATE_NAME_RETRY_LIMIT = 20;
const noopRefreshWorkspaceModelProviders = async (): Promise<null> => null;
const noopSelectModelProvider = (): void => undefined;

function feishuActionKey(agentID: string, action: FeishuActionKind): string {
  return `${agentID}:${FEISHU_CHANNEL_ACTION}:${action}`;
}

function feishuRegistrationExpired(registration: FeishuRegistration | null | undefined, now = Date.now()): boolean {
  const expiresAt = Date.parse(String(registration?.expires_at || ""));
  return Number.isFinite(expiresAt) && expiresAt <= now;
}

function feishuRegistrationFinalizeClearsPending(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const status = Number((error as { status?: unknown }).status);
  return status === 404 || status === 410;
}

function normalizeFeishuPendingRegistration(
  registration: FeishuRegistration | null | undefined,
  fallbackAgentID: string,
): FeishuPendingRegistration | null {
  const registrationID = String(registration?.registration_id || "").trim();
  const agentID = String(registration?.agent_id || fallbackAgentID || "").trim();
  if (!registrationID || !agentID || feishuRegistrationExpired(registration)) {
    return null;
  }
  return {
    ...registration,
    agent_id: agentID,
    registration_id: registrationID,
  };
}

function feishuRegistrationPollDelayMs(registration: FeishuRegistration | null | undefined): number {
  const rawSeconds = Number(registration?.next_poll_seconds);
  const seconds = Number.isFinite(rawSeconds) && rawSeconds > 0 ? rawSeconds : FEISHU_REGISTRATION_DEFAULT_POLL_SECONDS;
  return Math.min(FEISHU_REGISTRATION_MAX_POLL_SECONDS, Math.max(FEISHU_REGISTRATION_MIN_POLL_SECONDS, seconds)) * 1000;
}

function isAgentNameConflictError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const status = Number((error as { status?: unknown }).status);
  if (status === 409) {
    return true;
  }
  const message = String((error as { message?: unknown }).message || "")
    .trim()
    .toLowerCase();
  return message.includes("agent name") && message.includes("already exists");
}

function splitAgentNameSuffix(name: string): { baseName: string; nextIndex: number } {
  const trimmed = String(name || "").trim();
  const match = trimmed.match(/^(.*?)-(\d+)$/);
  if (!match) {
    return { baseName: trimmed, nextIndex: 2 };
  }
  return {
    baseName: match[1] || trimmed,
    nextIndex: Number.parseInt(match[2] || "1", 10) + 1,
  };
}

function nextAvailableAgentName(name: string, existingNames: Iterable<string>): string {
  const normalizedExisting = new Set(
    Array.from(existingNames, (item) =>
      String(item || "")
        .trim()
        .toLowerCase(),
    ).filter(Boolean),
  );
  const trimmed = String(name || "").trim();
  if (!trimmed) {
    return "";
  }
  if (!normalizedExisting.has(trimmed.toLowerCase())) {
    return trimmed;
  }
  const { baseName, nextIndex } = splitAgentNameSuffix(trimmed);
  const safeBaseName = baseName.trim() || trimmed;
  for (let index = nextIndex; index < nextIndex + AGENT_CREATE_NAME_RETRY_LIMIT; index += 1) {
    const candidate = `${safeBaseName}-${index}`;
    if (!normalizedExisting.has(candidate.toLowerCase())) {
      return candidate;
    }
  }
  return `${safeBaseName}-${Date.now()}`;
}

function pruneFeishuPendingRegistrations(
  registrations: Record<string, FeishuPendingRegistration>,
): Record<string, FeishuPendingRegistration> {
  const next: Record<string, FeishuPendingRegistration> = {};
  Object.entries(registrations).forEach(([agentID, registration]) => {
    const normalized = normalizeFeishuPendingRegistration(registration, agentID);
    if (normalized) {
      next[normalized.agent_id] = normalized;
    }
  });
  return next;
}

function loadFeishuPendingRegistrations(): Record<string, FeishuPendingRegistration> {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const raw = window.localStorage.getItem(FEISHU_REGISTRATIONS_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    const decoded = JSON.parse(raw);
    if (!decoded || typeof decoded !== "object" || Array.isArray(decoded)) {
      saveFeishuPendingRegistrations({});
      return {};
    }
    const pruned = pruneFeishuPendingRegistrations(decoded as Record<string, FeishuPendingRegistration>);
    saveFeishuPendingRegistrations(pruned);
    return pruned;
  } catch {
    saveFeishuPendingRegistrations({});
    return {};
  }
}

function saveFeishuPendingRegistrations(registrations: Record<string, FeishuPendingRegistration>): void {
  if (typeof window === "undefined") {
    return;
  }
  const pruned = pruneFeishuPendingRegistrations(registrations);
  try {
    if (Object.keys(pruned).length === 0) {
      window.localStorage.removeItem(FEISHU_REGISTRATIONS_STORAGE_KEY);
      return;
    }
    window.localStorage.setItem(FEISHU_REGISTRATIONS_STORAGE_KEY, JSON.stringify(pruned));
  } catch {
    // Persistence is best-effort; the in-memory state still drives the current tab.
  }
}

function loadLastCreatedAgentModelPreference(): LastCreatedAgentModelPreference | null {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    const raw = window.localStorage.getItem(LAST_CREATED_AGENT_MODEL_STORAGE_KEY);
    if (!raw) {
      return null;
    }
    const decoded = JSON.parse(raw);
    const providerID = String((decoded as { providerID?: unknown })?.providerID || "").trim();
    const modelID = String((decoded as { modelID?: unknown })?.modelID || "").trim();
    if (!providerID || !modelID) {
      return null;
    }
    return { providerID, modelID };
  } catch {
    return null;
  }
}

function saveLastCreatedAgentModelPreference(draft: Partial<AgentDraft> | null | undefined): void {
  if (typeof window === "undefined") {
    return;
  }
  const providerID = String(draft?.model_provider_id || "").trim();
  const modelID = String(draft?.model_id || "").trim();
  if (!providerID || !modelID) {
    return;
  }
  try {
    window.localStorage.setItem(
      LAST_CREATED_AGENT_MODEL_STORAGE_KEY,
      JSON.stringify({
        providerID,
        modelID,
      }),
    );
  } catch {
    // Best-effort only; fallback defaults still work.
  }
}

function draftWithModelProviderFallback(draft: AgentDraft, options: readonly ModelProviderOption[]): AgentDraft {
  const preference = loadLastCreatedAgentModelPreference();
  if (preference?.providerID && preference?.modelID) {
    const exactMatch = options.find(
      (item) => item.providerID === preference.providerID && item.modelID === preference.modelID,
    );
    if (exactMatch) {
      return {
        ...draft,
        provider: providerNameForProviderID(preference.providerID),
        model_provider_id: preference.providerID,
        model_id: preference.modelID,
      };
    }
    const providerMatch = options.find((item) => item.providerID === preference.providerID && item.modelID);
    if (providerMatch) {
      return {
        ...draft,
        provider: providerNameForProviderID(preference.providerID),
        model_provider_id: preference.providerID,
        model_id: providerMatch.modelID,
      };
    }
  }
  const providerID = String(draft.model_provider_id || "").trim();
  const modelID = String(draft.model_id || "").trim();
  if (providerID && modelID && options.some((item) => item.providerID === providerID && item.modelID === modelID)) {
    return draft;
  }
  let option = options.find((item) => {
    if (!item.providerID || !item.modelID) {
      return false;
    }
    if (providerID) {
      return item.providerID === providerID;
    }
    if (modelID) {
      return item.modelID === modelID;
    }
    return true;
  });
  if (!option) {
    option = options.find((item) => item.providerID && item.modelID);
  }
  if (!option) {
    return draft;
  }
  return {
    ...draft,
    provider: providerNameForProviderID(option.providerID),
    model_provider_id: option.providerID,
    model_id: option.modelID,
  };
}

export function shouldReturnToAgentOverviewAfterAgentMissing(
  activePane: { type?: string; id?: string | undefined } | null | undefined,
) {
  return activePane?.type === WorkspacePaneTypes.agent;
}

export function agentSelectionAfterDelete(
  items: AgentLike[],
  deletedAgentID: string,
  managerAgentID = MANAGER_AGENT_ID,
): AgentLike | null {
  const deletedIndex = items.findIndex((item) => item.id === deletedAgentID);
  const ordinaryAgents = items.filter(
    (item) => item.id !== managerAgentID && !isManagerAgent(item) && !isNotificationBotAgent(item),
  );
  const nextOrdinaryAgent =
    items.slice(Math.max(0, deletedIndex + 1)).find((item) => ordinaryAgents.some((agent) => agent.id === item.id)) ??
    items
      .slice(0, Math.max(0, deletedIndex))
      .reverse()
      .find((item) => ordinaryAgents.some((agent) => agent.id === item.id));

  return nextOrdinaryAgent ?? items.find((item) => item.id === managerAgentID || isManagerAgent(item)) ?? null;
}

export function useAgentController({
  activeConversationId,
  activePane,
  agents,
  agentsLoaded,
  agentsQuery,
  bootstrapConfig,
  data,
  catalogMCPServers = [],
  catalogMCPServersError = "",
  catalogMCPServersLoading = false,
  hubTemplates,
  locale,
  managerProfile,
  modelProviders = null,
  modelProvidersLoaded = false,
  openCSGAuthenticated = false,
  onAgentDeleted,
  profileDetailAgentID = "",
  refreshMCPServers = async () => null,
  refreshHubTemplates,
  refreshWorkspaceAgents,
  refreshWorkspaceBootstrap,
  refreshWorkspaceBootstrapConfig,
  refreshWorkspaceManagerProfile,
  refreshWorkspaceModelProviders = noopRefreshWorkspaceModelProviders,
  rooms,
  navigatePane,
  selectAgent,
  selectComputer,
  selectConversation,
  selectHub,
  selectModelProvider = noopSelectModelProvider,
  setAgentsData,
  setBootstrapData,
  setHubPublishError = () => undefined,
  setSelectedHubTemplateId,
  t,
}: UseAgentControllerArgs) {
  const errorMessage = useCallback(
    (error: unknown, fallback = "") => localizeAPIError(error, t, apiErrorMessage(error, fallback) || fallback),
    [t],
  );
  const queryClient = useQueryClient();
  const [cliproxyAuthBusy, setCLIProxyAuthBusy] = useState("");
  const [agentsError, setAgentsError] = useState("");
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [agentModalMode, setAgentModalMode] = useState<AgentModalMode>("create");
  const [agentCreateBotKind, setAgentCreateBotKind] = useState(BOT_CREATE_KIND_WORKER);
  const [agentCreateMode, setAgentCreateMode] = useState<AgentCreateMode>("template");
  const [editingAgent, setEditingAgent] = useState<AgentLike | null>(null);
  const [agentDraft, setAgentDraft] = useState<AgentDraft | null>(null);
  const [agentModalBootstrapConfig, setAgentModalBootstrapConfig] = useState<RuntimeBootstrapConfig | null>(null);
  const [agentBusy, setAgentBusy] = useState(false);
  const [agentError, setAgentError] = useState("");
  const [agentBillingURL, setAgentBillingURL] = useState("");
  const [agentProgress, setAgentProgress] = useState<AgentCreateProgressState | null>(null);
  const [agentActionBusyByAgent, setAgentActionBusyByAgent] = useState<AgentActionBusyState>({});
  const [larkCLIDialog, setLarkCLIDialog] = useState<LarkCLIDialogState | null>(null);
  const agentActionBusyByAgentRef = useRef<AgentActionBusyState>({});
  const [messageActionBusy, setMessageActionBusy] = useState("");
  const [messageActionFeedback, setMessageActionFeedback] = useState<MessageActionFeedback>({
    key: "",
    message: "",
  });
  const [agentPageDraft, setAgentPageDraft] = useState<AgentDraft | null>(null);
  const [agentPageSavedDraft, setAgentPageSavedDraft] = useState<AgentDraft | null>(null);
  const [agentPageBusy, setAgentPageBusy] = useState(false);
  const [agentPagePublishBusy, setAgentPagePublishBusy] = useState(false);
  const [agentPagePublishError, setAgentPagePublishError] = useState("");
  const [agentPageError, setAgentPageError] = useState("");
  const [agentPageBillingURL, setAgentPageBillingURL] = useState("");
  function setAgentPageSaveError(message: string, billingURL = ""): void {
    setAgentPageError(message);
    setAgentPageBillingURL(billingURL);
  }
  const [agentSkillAddBusy, setAgentSkillAddBusy] = useState(false);
  const [agentSkillAddError, setAgentSkillAddError] = useState("");
  const [agentSkillDeleteBusy, setAgentSkillDeleteBusy] = useState(false);
  const [agentSkillDeleteError, setAgentSkillDeleteError] = useState("");
  const [agentMCPAddBusy, setAgentMCPAddBusy] = useState(false);
  const [agentMCPAddError, setAgentMCPAddError] = useState("");
  const [agentMCPDeleteBusy, setAgentMCPDeleteBusy] = useState(false);
  const [agentMCPDeleteError, setAgentMCPDeleteError] = useState("");
  const [agentMCPSourceSyncBusyName, setAgentMCPSourceSyncBusyName] = useState("");
  const [agentPageNotices, setAgentPageNotices] = useState<Record<string, AgentPageNoticeState>>({});
  const agentPageNoticeTimersRef = useRef<Record<string, number>>({});
  const agentPageDraftLoadSeqRef = useRef(0);
  const agentPageDraftRequestRef = useRef(0);
  const agentPageNavigationConfirmedRef = useRef(false);
  const [feishuPendingRegistrations, setFeishuPendingRegistrations] = useState<
    Record<string, FeishuPendingRegistration>
  >(() => loadFeishuPendingRegistrations());
  const feishuAutoFinalizeActiveRef = useRef<Set<string>>(new Set());
  const refreshAgentStateRef = useRef<(agentID: string) => Promise<AgentLike | null>>(async () => null);
  const [teamActionBusy, setTeamActionBusy] = useState(false);
  const [teamActionError, setTeamActionError] = useState("");
  const [showCreateTeamModal, setShowCreateTeamModal] = useState(false);
  const [editingTeam, setEditingTeam] = useState<WorkspaceTeam | null>(null);
  const [createTeamTitle, setCreateTeamTitle] = useState("");
  const [createTeamMemberIDs, setCreateTeamMemberIDs] = useState<string[]>([]);
  const claimAgentAction = useCallback((agentID: string, busyKey: string, visible = true): boolean => {
    const normalizedAgentID = String(agentID || "").trim();
    const normalizedBusyKey = String(busyKey || "").trim();
    if (!normalizedAgentID || !normalizedBusyKey || agentActionBusyByAgentRef.current[normalizedAgentID]) {
      return false;
    }
    const next = {
      ...agentActionBusyByAgentRef.current,
      [normalizedAgentID]: { busyKey: normalizedBusyKey, visible },
    };
    agentActionBusyByAgentRef.current = next;
    setAgentActionBusyByAgent(next);
    return true;
  }, []);
  const releaseAgentAction = useCallback((agentID: string, busyKey: string): void => {
    const normalizedAgentID = String(agentID || "").trim();
    if (!normalizedAgentID || agentActionBusyByAgentRef.current[normalizedAgentID]?.busyKey !== busyKey) {
      return;
    }
    const next = { ...agentActionBusyByAgentRef.current };
    delete next[normalizedAgentID];
    agentActionBusyByAgentRef.current = next;
    setAgentActionBusyByAgent(next);
  }, []);
  const isAgentActionBusy = useCallback(
    (agentID: string | null | undefined): boolean =>
      Boolean(agentActionBusyByAgentRef.current[String(agentID || "").trim()]),
    [],
  );
  const agentActionBusyKeys = useMemo(
    () =>
      Object.values(agentActionBusyByAgent)
        .filter((entry) => entry.visible)
        .map((entry) => entry.busyKey),
    [agentActionBusyByAgent],
  );
  const agentPageHasUnsavedChanges = Boolean(
    agentPageDraft && agentPageSavedDraft && JSON.stringify(agentPageDraft) !== JSON.stringify(agentPageSavedDraft),
  );
  const agentPageNavigationBlocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      agentPageHasUnsavedChanges && currentLocation.pathname !== nextLocation.pathname,
  );
  const managerCodexRuntimeUnavailable = bootstrapConfig?.manager_runtime?.installed === false;
  const managerAgentRuntimeUnavailable = isAgentRuntimeUnavailable(managerProfile);
  const managerRuntimeUnavailable = managerCodexRuntimeUnavailable || managerAgentRuntimeUnavailable;
  const managerRuntimeWarning = managerRuntimeUnavailable
    ? managerCodexRuntimeUnavailable
      ? String(
          bootstrapConfig?.manager_runtime?.message ||
            bootstrapConfig?.manager_runtime?.install_guidance ||
            t("managerCodexMissingWarning"),
        )
      : t("runtimeSandboxUnavailable", {
          reason: agentOfflineReasonLabel(agentRuntimeState(managerProfile), t),
        })
    : "";
  const managerProfileIncomplete = managerProfile && managerProfile.profile_complete === false;
  const usersById = useMemo(() => {
    const result = new Map<string, IMUser>();
    data?.users.forEach((user) => result.set(user.id, user));
    return result;
  }, [data?.users]);
  const agentItems = useMemo(
    () =>
      agents.map((item) => ({
        ...item,
        avatar: resolveAgentAvatarSource(item, usersById),
      })),
    [agents, usersById],
  );

  async function saveLinkedAgentUserAvatar(
    item: AgentLike | null | undefined,
    avatar: string | null | undefined,
  ): Promise<void> {
    const userID = resolveAgentAvatarUserID(item, usersById);
    const nextAvatar = String(avatar || "").trim();
    if (!userID || !nextAvatar) {
      return;
    }
    const existing = usersById.get(userID);
    if (String(existing?.avatar || "").trim() === nextAvatar) {
      return;
    }
    const updated = await patchCsgclawUserRequest(userID, { avatar: nextAvatar });
    setBootstrapData((current) => {
      const currentUser = current?.users.find((candidate) => candidate.id === updated.id) ?? existing ?? null;
      return upsertUserInData(current, {
        ...(currentUser ?? { id: updated.id || userID, name: updated.name || item?.name || userID }),
        ...updated,
        avatar: String(updated.avatar || nextAvatar).trim() || nextAvatar,
        participants: updated.participants ?? currentUser?.participants,
      });
    });
  }

  const managerAgent = agentItems.find((item) => item.role === MANAGER_AGENT_ROLE || item.id === MANAGER_AGENT_ID);
  const { workerAgentItems, notificationAgentItems } = partitionWorkspaceAgentItems(agentItems, MANAGER_AGENT_ID);
  const createTeamCandidates = useMemo(
    () => [...workerAgentItems, ...notificationAgentItems].filter((item) => Boolean(item?.id)),
    [notificationAgentItems, workerAgentItems],
  );
  const createTeamCandidateIDs = useMemo(
    () => createTeamCandidates.map((item) => String(item.id)),
    [createTeamCandidates],
  );
  const managerTeamMemberID = matchingTeamCandidateID(managerAgent?.id || MANAGER_AGENT_ID, createTeamCandidateIDs);
  const lockedTeamMemberID = matchingTeamCandidateID(
    editingTeam?.lead_agent_id || managerTeamMemberID,
    createTeamCandidateIDs,
  );
  const runningAgentCount = agentItems.filter(isAgentRunning).length;
  const notifierWebhookPublicOrigin = useMemo(() => resolvedNotifierWebhookOrigin(bootstrapConfig), [bootstrapConfig]);
  const createParticipantAvatarSources = useMemo(
    () => [...agentItems, ...(data?.users ?? [])],
    [agentItems, data?.users],
  );
  const selectedAgentForPage = useMemo(() => {
    const selectedAgentID =
      activePane.type === WorkspacePaneTypes.agent ? String(activePane.id || "").trim() : profileDetailAgentID.trim();
    if (!selectedAgentID) {
      return null;
    }
    return agentItems.find((item) => item.id === selectedAgentID) ?? null;
  }, [agentItems, activePane.id, activePane.type, profileDetailAgentID]);
  const selectedAgentForPageDraftSignature = useMemo(() => {
    if (!selectedAgentForPage) {
      return "";
    }
    const profile = selectedAgentForPage.agent_profile;
    const modelProviderID =
      selectedAgentForPage.model_provider_id || selectedAgentForPage.agent_profile?.model_provider_id || "";
    const modelID = selectedAgentForPage.model_id || selectedAgentForPage.agent_profile?.model_id || "";
    return JSON.stringify({
      id: selectedAgentForPage.id || "",
      name: selectedAgentForPage.name || "",
      description: selectedAgentForPage.description || "",
      instructions: selectedAgentForPage.instructions || "",
      profile: modelProviderID
        ? profileSelectorFromDraft({ model_provider_id: modelProviderID, model_id: modelID })
        : "",
      profile_complete: selectedAgentForPage.profile_complete ?? profile?.profile_complete ?? null,
      provider: selectedAgentForPage.provider || profile?.provider || "",
      model_provider_id: modelProviderID,
      model_id: modelID,
      reasoning_effort: selectedAgentForPage.reasoning_effort || profile?.reasoning_effort || "",
      enable_fast_mode: selectedAgentForPage.enable_fast_mode ?? profile?.enable_fast_mode ?? false,
    });
  }, [selectedAgentForPage]);
  const selectedAgentForPageRef = useRef(selectedAgentForPage);
  selectedAgentForPageRef.current = selectedAgentForPage;
  const selectedFeishuPendingRegistration = useMemo(() => {
    const agentID = String(selectedAgentForPage?.id || "").trim();
    if (!agentID) {
      return null;
    }
    return normalizeFeishuPendingRegistration(feishuPendingRegistrations[agentID], agentID);
  }, [feishuPendingRegistrations, selectedAgentForPage?.id]);
  const agentDetailAgentID = selectedAgentForPage?.id || "";
  const globalSkillsQuery = useQuery({
    queryKey: workspaceQueryKeys.skills(),
    queryFn: async () => {
      const payload = await fetchSkills();
      return Array.isArray(payload) ? payload : [];
    },
  });
  const agentSkillsQuery = useQuery({
    queryKey: workspaceQueryKeys.agentSkills(agentDetailAgentID),
    queryFn: async () => {
      const skillsListing = await fetchAgentSkills(agentDetailAgentID);
      const skills = skillOptionsFromWorkspace(skillsListing.entries || []);
      return Promise.all(
        skills.map(async (skill) => {
          try {
            const file = await fetchAgentSkillsFile(agentDetailAgentID, `${skill.name}/SKILL.md`);
            return {
              ...skill,
              description: skillDescriptionFromMarkdown(file.content || "") || skill.description,
            };
          } catch {
            return skill;
          }
        }),
      );
    },
    enabled: Boolean(agentDetailAgentID),
  });
  const agentMCPServersQuery = useQuery({
    queryKey: workspaceQueryKeys.agentMCPServers(agentDetailAgentID),
    queryFn: () => fetchAgentMCPServers(agentDetailAgentID),
    enabled: Boolean(agentDetailAgentID),
  });
  const agentSkillsError = agentSkillsQuery.error
    ? errorMessage(agentSkillsQuery.error, t("agentSkillsLoadFailed"))
    : "";
  const agentSkillCandidates = useMemo(() => {
    const currentSkillNames = new Set((agentSkillsQuery.data ?? []).map((skill) => String(skill?.name || "").trim()));
    return (globalSkillsQuery.data ?? []).filter((skill) => {
      const name = String(skill?.name || "").trim();
      return Boolean(name) && !currentSkillNames.has(name);
    });
  }, [agentSkillsQuery.data, globalSkillsQuery.data]);
  const agentSkillCandidatesError = globalSkillsQuery.error
    ? errorMessage(globalSkillsQuery.error, t("agentSkillsLoadFailed"))
    : "";
  const agentMCPServers = useMemo(() => {
    return mcpServersFromMap(agentMCPServersQuery.data?.servers);
  }, [agentMCPServersQuery.data]);
  const agentMCPCandidates = useMemo(() => {
    const currentNames = new Set(agentMCPServers.map((server) => server.name));
    return catalogMCPServers.filter((server) => server.name && !currentNames.has(server.name));
  }, [agentMCPServers, catalogMCPServers]);
  const agentManagedMCPServers = useMemo(
    () => agentMCPServers.filter((server) => Boolean(mcpManagedKnowledgeBaseSource(server.config))),
    [agentMCPServers],
  );
  const agentMCPSourceQueries = useQueries({
    queries: agentManagedMCPServers.map((server) => ({
      queryKey: workspaceQueryKeys.agentMCPServerSource(agentDetailAgentID, server.name),
      queryFn: () => fetchAgentMCPServerSourceStatus(agentDetailAgentID, server.name),
      enabled: Boolean(agentDetailAgentID),
      retry: false,
    })),
  });
  const agentMCPSourceStatusByName = useMemo(() => {
    return Object.fromEntries(
      agentManagedMCPServers.flatMap((server, index) => {
        const status = agentMCPSourceQueries[index]?.data;
        return status ? [[server.name, status]] : [];
      }),
    ) as Record<string, MCPServerSourceStatus>;
  }, [agentMCPSourceQueries, agentManagedMCPServers]);
  const agentMCPSourceBusyNames = useMemo(
    () =>
      new Set(
        agentManagedMCPServers
          .filter((_, index) => agentMCPSourceQueries[index]?.isFetching)
          .map((server) => server.name),
      ),
    [agentMCPSourceQueries, agentManagedMCPServers],
  );
  const agentMCPSourceUnavailableNames = useMemo(
    () =>
      new Set(
        agentManagedMCPServers
          .filter((server) => agentMCPSourceStatusByName[server.name]?.sourceAvailable === false)
          .map((server) => server.name),
      ),
    [agentMCPSourceStatusByName, agentManagedMCPServers],
  );
  const agentMCPUpdateAvailableNames = useMemo(() => {
    return new Set(
      agentMCPServers
        .filter((server) => {
          const sourceStatus = agentMCPSourceStatusByName[server.name];
          return sourceStatus?.sourceAvailable !== false && sourceStatus?.updateAvailable;
        })
        .map((server) => server.name),
    );
  }, [agentMCPServers, agentMCPSourceStatusByName]);
  const activeConversation = useMemo(
    () => data?.rooms.find((item) => item.id === activeConversationId) ?? null,
    [data, activeConversationId],
  );

  const agentsDisplayError =
    agentsError || (agentsQuery.isError ? errorMessage(agentsQuery.error, t("agentActionFailed")) : "");
  const teamsQuery = useQuery({
    queryKey: workspaceQueryKeys.teams(),
    queryFn: fetchTeams,
  });

  const codexModelProviderAvailable = bootstrapConfig?.manager_runtime?.installed !== false;
  const agentModelProviders = useMemo(
    () =>
      modelProviderCatalogForAgentAvailability(modelProviders, {
        codexAvailable: codexModelProviderAvailable,
      }),
    [codexModelProviderAvailable, modelProviders],
  );
  const agentModelOptions = useMemo(() => modelProviderOptionsFromCatalog(agentModelProviders), [agentModelProviders]);
  const {
    models: agentPageDiscoveredModels,
    modelBusy: agentPageModelProbeBusy,
    modelError: agentPageModelError,
    retryModels: retryAgentPageModels,
  } = useProfileModelOptions({
    draft: agentPageDraft,
    enabled: Boolean(selectedAgentForPage),
    onDraftChange: setAgentPageDraft,
  });
  const agentPageModelProviders = useMemo(
    () =>
      modelProviderCatalogWithModels(
        agentModelProviders,
        String(agentPageDraft?.model_provider_id || ""),
        agentPageDiscoveredModels,
      ),
    [agentModelProviders, agentPageDiscoveredModels, agentPageDraft?.model_provider_id],
  );
  const agentPageModelOptions = useMemo(
    () => modelProviderOptionsFromCatalog(agentPageModelProviders),
    [agentPageModelProviders],
  );
  const agentModelBusy = Boolean(showAgentModal && !modelProvidersLoaded);
  const agentPageModelBusy = Boolean(selectedAgentForPage && (!modelProvidersLoaded || agentPageModelProbeBusy));
  const resetAgentModels = useCallback(() => {
    void refreshWorkspaceModelProviders();
  }, [refreshWorkspaceModelProviders]);
  const resetAgentPageModels = useCallback(() => {
    void refreshWorkspaceModelProviders();
  }, [refreshWorkspaceModelProviders]);
  const fetchAgentWithProfile = useCallback(async (item: AgentLike | null | undefined): Promise<AgentWithProfile> => {
    const id = String(item?.id ?? "").trim();
    if (!id) {
      return { agent: item || {}, profile: item?.agent_profile };
    }
    let agent: AgentLike = item || {};
    const [fetchedAgent, fetchedProfile] = await Promise.all([
      Promise.resolve(fetchAgent(id)).catch(() => null),
      Promise.resolve(fetchAgentProfile(id)).catch(() => null),
    ]);
    if (fetchedAgent && String(fetchedAgent.id ?? "").trim() === id) {
      agent = { ...agent, ...fetchedAgent };
    }
    const profile = fetchedProfile ?? agent?.agent_profile;
    return { agent, profile };
  }, []);
  const agentDraftFromItem = useCallback(
    async (item: AgentLike): Promise<AgentPageDraftData> => {
      if (isNotificationBotAgent(item)) {
        return { agent: item, draft: ensureNotifierPullSubscriptionDraft(agentToDraft(item)) };
      }
      const [{ agent, profile }, mcpServersView] = await Promise.all([
        fetchAgentWithProfile(item),
        fetchAgentMCPServers(String(item.id || "").trim()),
      ]);
      const base = agentToDraft({ ...agent, agent_profile: profile });
      const runtimeKind = normalizeRuntimeKind(agent?.runtime_kind || item?.runtime_kind || base.runtime_kind);
      return {
        agent,
        draft: ensureNotifierPullSubscriptionDraft({
          ...base,
          mcpServers: cloneMCPServersForDraft(mcpServersView.servers),
          runtime_kind: runtimeKind || base.runtime_kind,
          bot_type: BOT_TYPE_NORMAL,
        }),
      };
    },
    [fetchAgentWithProfile],
  );
  const loadAgentPageDraft = useCallback(
    async (item: AgentLike | null | undefined, loadSeq: number): Promise<void> => {
      if (!item?.id) {
        return;
      }
      const requestID = agentPageDraftRequestRef.current + 1;
      agentPageDraftRequestRef.current = requestID;
      setAgentPageSaveError("");
      resetAgentPageModels();
      const fallbackDraft = ensureNotifierPullSubscriptionDraft(agentToDraft(item));
      setAgentPageDraft(fallbackDraft);
      setAgentPageSavedDraft(fallbackDraft);
      try {
        const { agent, draft } = await agentDraftFromItem(item);
        if (agentPageDraftLoadSeqRef.current !== loadSeq || agentPageDraftRequestRef.current !== requestID) {
          return;
        }
        // The detail endpoint performs the bounded readiness probe. Publish
        // that result to the shared roster cache as well, so the page and
        // agent list render the same transient runtime observation.
        setAgentsData((current) => mergeAgentIntoList(current, agent));
        setAgentPageDraft(draft);
        setAgentPageSavedDraft(draft);
      } catch (err) {
        if (agentPageDraftLoadSeqRef.current !== loadSeq || agentPageDraftRequestRef.current !== requestID) {
          return;
        }
        setAgentPageSaveError(errorMessage(err, t("agentActionFailed")));
      }
    },
    [agentDraftFromItem, errorMessage, resetAgentPageModels, setAgentsData, t],
  );
  const { cliproxyAuthStatuses, setCLIProxyAuthStatus } = useCLIProxyAuthStatuses(
    [
      managerProfile?.provider,
      isNotifierRuntimeDraft(agentDraft) ? "" : agentDraft?.provider,
      isNotifierRuntimeDraft(agentPageDraft) ? "" : agentPageDraft?.provider,
    ],
    t,
  );

  const progressBusy = agentBusy;

  const clearAgentPageNotice = useCallback((ownerAgentID?: string | null) => {
    const noticeKey =
      ownerAgentID === null
        ? GLOBAL_AGENT_PAGE_NOTICE_KEY
        : String(ownerAgentID || selectedAgentForPageRef.current?.id || GLOBAL_AGENT_PAGE_NOTICE_KEY).trim();
    const timer = agentPageNoticeTimersRef.current[noticeKey];
    if (timer !== undefined) {
      window.clearTimeout(timer);
      delete agentPageNoticeTimersRef.current[noticeKey];
    }
    setAgentPageNotices((current) => {
      if (!(noticeKey in current)) {
        return current;
      }
      const next = { ...current };
      delete next[noticeKey];
      return next;
    });
  }, []);

  const showAgentPageNotice = useCallback(
    (message: string, tone: AgentPageNoticeTone = "warning", durationMs = 5000, ownerAgentID?: string | null) => {
      const noticeKey =
        ownerAgentID === null
          ? GLOBAL_AGENT_PAGE_NOTICE_KEY
          : String(ownerAgentID || selectedAgentForPageRef.current?.id || GLOBAL_AGENT_PAGE_NOTICE_KEY).trim();
      const timer = agentPageNoticeTimersRef.current[noticeKey];
      if (timer !== undefined) {
        window.clearTimeout(timer);
        delete agentPageNoticeTimersRef.current[noticeKey];
      }
      setAgentPageNotices((current) => ({ ...current, [noticeKey]: { message, tone } }));
      if (durationMs <= 0) {
        return;
      }
      agentPageNoticeTimersRef.current[noticeKey] = window.setTimeout(() => {
        setAgentPageNotices((current) => {
          if (!(noticeKey in current)) {
            return current;
          }
          const next = { ...current };
          delete next[noticeKey];
          return next;
        });
        delete agentPageNoticeTimersRef.current[noticeKey];
      }, durationMs);
    },
    [],
  );

  useEffect(
    () => () => {
      Object.values(agentPageNoticeTimersRef.current).forEach((timer) => window.clearTimeout(timer));
      agentPageNoticeTimersRef.current = {};
    },
    [],
  );

  function agentOperationUsesPageError(item: AgentLike | null | undefined): boolean {
    return Boolean(item?.id && selectedAgentForPageRef.current?.id === item.id);
  }

  function clearAgentOperationError(item: AgentLike | null | undefined): void {
    if (agentOperationUsesPageError(item)) {
      setAgentPageSaveError("");
      return;
    }
    setAgentsError("");
  }

  function setAgentOperationError(item: AgentLike | null | undefined, message: string, billingURL = ""): void {
    if (agentOperationUsesPageError(item)) {
      setAgentPageSaveError(message, billingURL);
      return;
    }
    setAgentsError(message);
  }

  useEffect(() => {
    if (!progressBusy || !agentProgress?.steps?.length) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      setAgentProgress((current) => advanceAgentProgress(current));
    }, 1200);
    return () => window.clearInterval(timer);
  }, [progressBusy, agentProgress?.startedAt, agentProgress?.steps?.length]);

  useEffect(() => {
    if (
      !showAgentModal ||
      agentModalMode !== "create" ||
      agentCreateBotKind !== BOT_CREATE_KIND_WORKER ||
      !hasUnavailableSandboxRuntimeChoice(agentModalBootstrapConfig || bootstrapConfig)
    ) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      void refreshWorkspaceBootstrapConfig().then((config) => {
        if (config) {
          setAgentModalBootstrapConfig(config);
        }
      });
    }, SANDBOX_RUNTIME_REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [
    agentCreateBotKind,
    agentModalMode,
    agentModalBootstrapConfig,
    bootstrapConfig,
    refreshWorkspaceBootstrapConfig,
    showAgentModal,
  ]);

  useEffect(() => {
    if (!managerProfileIncomplete) {
      clearAgentPageNotice(null);
      return;
    }
  }, [clearAgentPageNotice, managerProfileIncomplete]);

  useEffect(() => {
    if (!managerProfileIncomplete) {
      return;
    }
    if (activePane.type === WorkspacePaneTypes.agent && activePane.id === MANAGER_AGENT_ID) {
      return;
    }
    showAgentPageNotice(t("profileIncompleteRedirectNotice"), "warning", 5000, null);
    selectAgent({ id: MANAGER_AGENT_ID }, { replace: true });
  }, [activePane.id, activePane.type, managerProfileIncomplete, selectAgent, showAgentPageNotice, t]);

  useEffect(() => {
    if (!activePane || activePane.type !== WorkspacePaneTypes.agent) {
      return;
    }
    if (!agentsLoaded) {
      return;
    }
    if (shouldReturnToAgentOverviewAfterAgentMissing(activePane) && !agents.some((item) => item.id === activePane.id)) {
      selectComputer({ replace: true });
    }
  }, [agents, agentsLoaded, activePane, selectComputer]);

  useEffect(() => {
    if (agentPageNavigationBlocker.state !== "blocked") {
      agentPageNavigationConfirmedRef.current = false;
      return;
    }
    if (agentPageNavigationConfirmedRef.current) {
      return;
    }
    agentPageNavigationConfirmedRef.current = true;
    if (window.confirm(t("agentUnsavedChangesWarning"))) {
      setAgentPageDraft(agentPageSavedDraft);
      setAgentPageSaveError("");
      agentPageNavigationBlocker.proceed();
    } else {
      agentPageNavigationBlocker.reset();
    }
  }, [agentPageNavigationBlocker, agentPageSavedDraft, t]);

  useEffect(() => {
    if (!agentPageHasUnsavedChanges) {
      return undefined;
    }
    function handleBeforeUnload(event: BeforeUnloadEvent) {
      event.preventDefault();
      event.returnValue = "";
    }
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [agentPageHasUnsavedChanges]);

  useEffect(() => {
    setAgentSkillAddError("");
    setAgentSkillDeleteError("");
    setAgentMCPAddError("");
    setAgentMCPDeleteError("");
  }, [agentDetailAgentID]);

  useEffect(() => {
    const selectedAgent = selectedAgentForPageRef.current;
    if (!selectedAgent) {
      agentPageDraftLoadSeqRef.current += 1;
      setAgentPageDraft(null);
      setAgentPageSavedDraft(null);
      setAgentPageSaveError("");
      setAgentPagePublishBusy(false);
      return;
    }
    if (agentPageHasUnsavedChanges) {
      setAgentPageSaveError("");
      return;
    }
    const loadSeq = agentPageDraftLoadSeqRef.current + 1;
    agentPageDraftLoadSeqRef.current = loadSeq;
    const draft = ensureNotifierPullSubscriptionDraft(agentToDraft(selectedAgent));
    setAgentPageDraft(draft);
    setAgentPageSavedDraft(draft);
    void loadAgentPageDraft(selectedAgent, loadSeq);
  }, [agentPageHasUnsavedChanges, loadAgentPageDraft, selectedAgentForPage?.id, selectedAgentForPageDraftSignature]);

  async function refreshManagerProfile(): Promise<void> {
    await refreshWorkspaceManagerProfile();
  }

  async function loginCLIProxyProvider(provider: string | null | undefined): Promise<void> {
    const normalized = normalizeAuthProviderName(provider);
    if (!providerNeedsAuth(normalized) || cliproxyAuthBusy) {
      return;
    }
    setCLIProxyAuthBusy(normalized);
    setCLIProxyAuthStatus(normalized, {
      ...(cliproxyAuthStatuses[normalized] || {}),
      provider: normalized,
      message: t("authConnecting"),
    });
    try {
      const status = await loginCLIProxyProviderRequest(normalized);
      setCLIProxyAuthStatus(normalized, status);
    } catch (err) {
      setCLIProxyAuthStatus(normalized, {
        provider: normalized,
        authenticated: false,
        login_required: true,
        message: errorMessage(err, t("authMissing")),
      });
    } finally {
      setCLIProxyAuthBusy("");
    }
  }

  async function requestManagerRebuild(): Promise<void> {
    if (managerRuntimeUnavailable) {
      throw new Error(managerRuntimeWarning || t("managerCodexMissingWarning"));
    }
    const rebuiltAgent = await createManagerAgentRequest();
    await refreshAgentsWithUpdatedAgent(rebuiltAgent);
    const runningAgent = await syncAgentStateUntilRunning(MANAGER_AGENT_ID);
    if (!isAgentRunning(runningAgent)) {
      throw new Error(t("managerRecreateNotRunning"));
    }
    await refreshManagerProfile();
    await refreshWorkspaceBootstrapConfig();
  }

  async function rebuildManagerFromBrowser(): Promise<{ billingURL: string; message: string } | null> {
    try {
      await requestManagerRebuild();
      return null;
    } catch (err) {
      return {
        billingURL: apiErrorBillingURL(err),
        message: errorMessage(err, t("agentActionFailed")),
      };
    }
  }

  async function handleMessageAction(action: MessageAction | null | undefined, message?: MessageLike | null) {
    if (!action || action.id !== ACTION_REBUILD_MANAGER) {
      return;
    }
    const managerActionAgentID = String(managerAgent?.id || MANAGER_AGENT_ID).trim();
    const busyKey = `${managerActionAgentID}:recreate`;
    if (messageActionBusy || isAgentActionBusy(managerActionAgentID)) {
      return;
    }
    if (!claimAgentAction(managerActionAgentID, busyKey)) {
      return;
    }
    const key = `${message?.id || "message"}:${action.id}`;
    setMessageActionBusy(key);
    setMessageActionFeedback({ key, message: t("managerRecreateInProgress"), tone: "info" });
    try {
      const rebuildError = await rebuildManagerFromBrowser();
      if (rebuildError) {
        setMessageActionFeedback({ key, message: rebuildError.message, tone: "error" });
      } else {
        setMessageActionFeedback({ key, message: t("managerRecreateSucceeded"), tone: "success" });
      }
    } finally {
      releaseAgentAction(managerActionAgentID, busyKey);
      setMessageActionBusy("");
    }
  }

  async function refreshAgents(options: FetchAgentsOptions = {}) {
    try {
      await refreshWorkspaceAgents(options);
      setAgentsError("");
    } catch (err) {
      if (!options.silent) {
        setAgentsError(errorMessage(err, t("agentActionFailed")));
      }
    }
  }

  async function fetchLatestActionAgent(updatedAgent: AgentLike | null | undefined): Promise<AgentLike | null> {
    const id = String(updatedAgent?.id ?? "").trim();
    if (!id) {
      return updatedAgent ?? null;
    }
    try {
      const fetched = await fetchAgent(id, { cacheBust: true });
      return mergeAgentIntoList(updatedAgent ? [updatedAgent] : [], fetched)[0] ?? fetched;
    } catch (_) {
      return updatedAgent ?? null;
    }
  }

  function applyAgentListUpdate(agent: AgentLike | null | undefined) {
    const agentID = String(agent?.id ?? "").trim();
    if (!agentID || !agent) {
      return;
    }
    setAgentsData((current) => mergeAgentIntoList(current, agent));
    if (selectedAgentForPage?.id === agentID) {
      setAgentPageDraft((current) => agentDraftWithRuntimeFieldsFromAgent(current ?? agentToDraft(agent), agent));
      setAgentPageSavedDraft((current) => agentDraftWithRuntimeFieldsFromAgent(current ?? agentToDraft(agent), agent));
    }
  }

  async function refreshAgentState(agentID: string): Promise<AgentLike | null> {
    try {
      const latest = await fetchAgent(agentID, { cacheBust: true });
      applyAgentListUpdate(latest);
      return latest;
    } catch {
      try {
        await refreshWorkspaceAgents({ silent: true });
        const latest = await fetchAgent(agentID);
        applyAgentListUpdate(latest);
        return latest;
      } catch {
        return null;
      }
    }
  }

  useEffect(() => {
    refreshAgentStateRef.current = refreshAgentState;
  });

  async function syncAgentStateUntilRunning(
    agentID: string,
    options: { timeoutMs?: number; intervalMs?: number; acceptStopped?: boolean } = {},
  ): Promise<AgentLike | null> {
    const timeoutMs = options.timeoutMs ?? AGENT_RUNTIME_SYNC_TIMEOUT_MS;
    const intervalMs = options.intervalMs ?? AGENT_RUNTIME_SYNC_INTERVAL_MS;
    const acceptStopped = options.acceptStopped ?? false;
    const deadline = Date.now() + timeoutMs;
    let latest: AgentLike | null = null;
    while (Date.now() < deadline) {
      try {
        latest = await fetchAgent(agentID);
        applyAgentListUpdate(latest);
        if (isAgentRunning(latest)) {
          return latest;
        }
        if (acceptStopped && agentRuntimePollSettled(latest)) {
          return latest;
        }
      } catch {
        // Manager sandbox provisioning can lag behind profile save.
      }
      await new Promise((resolve) => window.setTimeout(resolve, intervalMs));
    }
    try {
      await refreshWorkspaceAgents({ silent: true });
      latest = (await fetchAgent(agentID)) ?? latest;
      applyAgentListUpdate(latest);
    } catch {
      // Best-effort final refresh.
    }
    return latest;
  }

  async function syncManagerRuntimeAfterProfileSave(
    agentBeforeSave: AgentLike | null | undefined,
    profileIncompleteBeforeSave = false,
  ): Promise<void> {
    if (
      shouldWaitForManagerRuntimeAfterProfileSave(agentBeforeSave, {
        profileIncompleteBeforeSave,
      })
    ) {
      await syncAgentStateUntilRunning(MANAGER_AGENT_ID, { acceptStopped: true });
      return;
    }
    await refreshAgentState(MANAGER_AGENT_ID);
  }

  async function refreshAgentsWithUpdatedAgent(updatedAgent: AgentLike | null | undefined): Promise<void> {
    const latestAgent = await fetchLatestActionAgent(updatedAgent);
    await refreshAgents();
    if (latestAgent?.id) {
      applyAgentListUpdate(latestAgent);
      await refreshAgentSkills(latestAgent.id);
    }
  }

  async function refreshAgentSkills(agentID: string | null | undefined): Promise<void> {
    const id = String(agentID ?? "").trim();
    if (!id) {
      return;
    }
    await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(id) });
  }

  async function openCreateNotificationParticipantModal(): Promise<void> {
    setAgentModalMode("create");
    setAgentCreateBotKind(BOT_CREATE_KIND_NOTIFICATION);
    setAgentCreateMode("custom");
    setEditingAgent(null);
    setAgentModalBootstrapConfig(null);
    setAgentError("");
    setAgentBillingURL("");
    setAgentProgress(null);
    resetAgentModels();
    const draft = ensureNotifierPullSubscriptionDraft(
      agentToDraft({
        name: "",
        description: "",
        avatar: selectUnusedAgentAvatar(createParticipantAvatarSources),
        bot_type: BOT_TYPE_NOTIFICATION,
      }),
    );
    setAgentDraft(draft);
    setShowAgentModal(true);
  }

  async function openCreateAgentModal(template: AgentTemplateLike | null | undefined = undefined): Promise<void> {
    setAgentModalMode("create");
    setAgentCreateBotKind(BOT_CREATE_KIND_WORKER);
    setAgentCreateMode("template");
    setEditingAgent(null);
    setAgentError("");
    setAgentBillingURL("");
    setAgentProgress(null);
    resetAgentModels();
    const refreshedBootstrapConfig = await refreshWorkspaceBootstrapConfig();
    const effectiveBootstrapConfig = refreshedBootstrapConfig || bootstrapConfig;
    setAgentModalBootstrapConfig(effectiveBootstrapConfig);
    const runtimeChoices = runtimeChoicesFromBootstrapConfig(effectiveBootstrapConfig);
    const codexAvailable = runtimeChoices.some(
      (item) => !item?.sandbox_enabled && normalizeRuntimeName(item?.name) === "codex" && item?.installed !== false,
    );
    const isCSGHubSandboxProvider =
      String(effectiveBootstrapConfig?.sandbox_provider || "")
        .trim()
        .toLowerCase() === "csghub";
    const createWorkerTemplates = workerSelectableTemplates(hubTemplates).filter(
      (item) => normalizeRuntimeKind(item.runtime_kind) !== "picoclaw_sandbox",
    );
    const preferredSandboxRuntimeName =
      normalizeRuntimeName(
        runtimeChoices.find((item) => item?.sandbox_enabled && normalizeRuntimeName(item?.name) === "openclaw")?.name,
      ) || normalizeRuntimeName(runtimeChoices.find((item) => item?.sandbox_enabled)?.name || "openclaw");
    let preferredRuntimeKind =
      normalizeRuntimeKind(composeLegacyRuntimeKind(preferredSandboxRuntimeName, true)) || "openclaw_sandbox";
    if (isCSGHubSandboxProvider) {
      preferredRuntimeKind = "codex";
    } else if (!runtimeChoices.length && !codexAvailable) {
      preferredRuntimeKind = "openclaw_sandbox";
    }
    const selectedTemplate =
      template === undefined
        ? pickDefaultAgentTemplate(createWorkerTemplates, preferredRuntimeKind, effectiveBootstrapConfig)
        : normalizeTemplateSelection(template);
    try {
      const defaults = await fetchAgentProfileDefaults();
      const initialRuntime = resolveRuntimeSelection({
        runtime_kind: selectedTemplate?.runtime_kind || preferredRuntimeKind,
      });
      let draft = agentToDraft({
        avatar: selectUnusedAgentAvatar(createParticipantAvatarSources),
        image: defaultWorkerImageForRuntime(
          hubTemplates,
          initialRuntime.runtime_kind,
          effectiveBootstrapConfig,
          managerAgent?.image || "",
        ),
        runtime_name: initialRuntime.runtime_name,
        sandbox_enabled: initialRuntime.sandbox_enabled,
        runtime_kind: initialRuntime.runtime_kind,
        bot_type: BOT_TYPE_NORMAL,
        agent_profile: defaults,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, effectiveBootstrapConfig, managerAgent?.image || "");
      draft = draftWithModelProviderFallback(draft, agentModelOptions);
      setAgentDraft(draft);
      setShowAgentModal(true);
    } catch (_) {
      const initialRuntime = resolveRuntimeSelection({
        runtime_kind: selectedTemplate?.runtime_kind || preferredRuntimeKind,
      });
      let draft = agentToDraft({
        avatar: selectUnusedAgentAvatar(createParticipantAvatarSources),
        image: defaultWorkerImageForRuntime(
          hubTemplates,
          initialRuntime.runtime_kind,
          effectiveBootstrapConfig,
          managerAgent?.image || "",
        ),
        runtime_name: initialRuntime.runtime_name,
        sandbox_enabled: initialRuntime.sandbox_enabled,
        runtime_kind: initialRuntime.runtime_kind,
        bot_type: BOT_TYPE_NORMAL,
        agent_profile: managerProfile,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, effectiveBootstrapConfig, managerAgent?.image || "");
      draft = draftWithModelProviderFallback(draft, agentModelOptions);
      setAgentDraft(draft);
      setShowAgentModal(true);
    }
  }

  function openCreateTeamModal(): void {
    setEditingTeam(null);
    setCreateTeamTitle("");
    setCreateTeamMemberIDs(managerTeamMemberID ? [managerTeamMemberID] : []);
    setTeamActionError("");
    setShowCreateTeamModal(true);
  }

  function closeCreateTeamModal(): void {
    setShowCreateTeamModal(false);
    setEditingTeam(null);
    setTeamActionError("");
  }

  function openManageTeamMembers(item: WorkspaceTeam | null | undefined): void {
    if (!item?.id) {
      return;
    }
    setEditingTeam(item);
    setCreateTeamTitle(displayTeam(item));
    setCreateTeamMemberIDs(teamSelectionMemberIDs(item, createTeamCandidateIDs));
    setTeamActionError("");
    setShowCreateTeamModal(true);
  }

  async function openEditAgentModal(item: AgentLike): Promise<void> {
    setAgentModalMode("edit");
    setAgentCreateBotKind(isNotificationBotAgent(item) ? BOT_CREATE_KIND_NOTIFICATION : BOT_CREATE_KIND_WORKER);
    setAgentCreateMode("custom");
    setEditingAgent(null);
    setAgentModalBootstrapConfig(bootstrapConfig);
    setAgentError("");
    setAgentBillingURL("");
    setAgentProgress(null);
    resetAgentModels();
    try {
      const { draft } = await agentDraftFromItem(item);
      setEditingAgent({ ...item, mcpServers: draft.mcpServers });
      setAgentDraft(draft);
      setShowAgentModal(true);
    } catch (err) {
      setAgentError(errorMessage(err, t("agentActionFailed")));
    }
  }

  function normalizeDraftForCompare(draft: AgentDraft | null | undefined): AgentDraft | null {
    if (!draft) {
      return null;
    }
    return ensureNotifierPullSubscriptionDraft(draft);
  }

  function profilePayloadForCompare(draft: AgentDraft | null | undefined): string {
    const normalized = normalizeDraftForCompare(draft);
    if (!normalized) {
      return "";
    }
    return JSON.stringify(
      draftToProfileComparePayload(normalized, {
        name: normalized.name,
        description: normalized.description,
      }),
    );
  }

  function runtimeOptionsPayloadForCompare(draft: AgentDraft | null | undefined): string {
    const normalized = normalizeDraftForCompare(draft);
    if (!normalized) {
      return "";
    }
    const runtimeOptions = draftRuntimeOptionsForSave(normalized, {
      mergeNotifier: false,
    });
    return JSON.stringify(runtimeOptions || {});
  }

  function mcpServersPayloadForCompare(draft: AgentDraft | null | undefined): string {
    const normalized = normalizeDraftForCompare(draft);
    if (!normalized) {
      return "";
    }
    return JSON.stringify({ mcpServers: draftMCPServersForSave(normalized) });
  }

  function hasObjectValues(value: unknown): value is Record<string, unknown> {
    return Boolean(value && typeof value === "object" && !Array.isArray(value) && Object.keys(value).length > 0);
  }

  function debugAgentPageSavePayload(mode: "meta-only" | "full", payload: AgentUpdatePayload): void {
    if (!import.meta.env.DEV) {
      return;
    }
    // Dev-only trace to verify whether avatar-only saves include profile/runtime payloads.
    console.info("[agent-page-save]", {
      agent_id: selectedAgentForPage?.id || "",
      mode,
      payload,
    });
  }

  function agentPageBaseUpdatePayload(draftToSave: AgentDraft): AgentUpdatePayload {
    const payload: AgentUpdatePayload = {
      description: draftToSave.description,
      instructions: draftToSave.instructions,
    };
    const managerDraft =
      isManagerAgent(selectedAgentForPage) ||
      draftToSave.agent_id === MANAGER_AGENT_ID ||
      draftToSave.role === MANAGER_AGENT_ROLE;
    if (!managerDraft) {
      payload.name = draftToSave.name;
    }
    return payload;
  }

  function canApplyAgentPageProfileSaveImmediately(
    saved: AgentLike | null | undefined,
    profileChanged: boolean,
    runtimeOptionsChanged: boolean,
    mcpServersChanged: boolean,
  ): boolean {
    return Boolean(
      saved?.id && saved.id !== MANAGER_AGENT_ID && profileChanged && !runtimeOptionsChanged && !mcpServersChanged,
    );
  }

  async function saveAgentPageMetadata(patch: Pick<Partial<AgentDraft>, "description" | "name">): Promise<void> {
    const agentID = String(selectedAgentForPage?.id ?? "").trim();
    if (!agentID || !agentPageDraft || agentPageBusy) {
      return;
    }
    const payload: AgentUpdatePayload = {};
    const isManagerDraft =
      isManagerAgent(selectedAgentForPage) ||
      agentPageDraft.agent_id === MANAGER_AGENT_ID ||
      agentPageDraft.role === MANAGER_AGENT_ROLE;
    if (patch.name !== undefined && !isManagerDraft) {
      const name = String(patch.name ?? "");
      if (!name.trim()) {
        setAgentPageSaveError(t("profileSaveIncompleteError"));
        return;
      }
      if (name !== String(agentPageSavedDraft?.name ?? "")) {
        payload.name = name;
      }
    }
    if (patch.description !== undefined) {
      const description = String(patch.description ?? "");
      if (description !== String(agentPageSavedDraft?.description ?? "")) {
        payload.description = description;
      }
    }
    if (!Object.keys(payload).length) {
      return;
    }
    setAgentPageBusy(true);
    setAgentPageSaveError("");
    try {
      const saved = await updateAgentRequest(agentID, payload);
      setAgentsData((current) => mergeAgentIntoList(current, saved));
      setAgentPageDraft((current) => {
        if (!current || String(current.agent_id ?? "").trim() !== agentID) {
          return current;
        }
        return {
          ...current,
          ...(payload.name !== undefined ? { name: saved.name || "" } : {}),
          ...(payload.description !== undefined ? { description: saved.description || "" } : {}),
        };
      });
      setAgentPageSavedDraft((current) => {
        if (!current || String(current.agent_id ?? "").trim() !== agentID) {
          return current;
        }
        return {
          ...current,
          ...(payload.name !== undefined ? { name: saved.name || "" } : {}),
          ...(payload.description !== undefined ? { description: saved.description || "" } : {}),
        };
      });
    } catch (err) {
      setAgentPageSaveError(errorMessage(err, t("agentActionFailed")), apiErrorBillingURL(err));
    } finally {
      setAgentPageBusy(false);
    }
  }

  async function saveAgentPage(): Promise<void> {
    const draftToSave = agentPageDraft;
    if (!draftToSave || !selectedAgentForPage?.id) {
      return;
    }
    setAgentPageBusy(true);
    setAgentPageSaveError("");
    try {
      const draft = ensureNotifierPullSubscriptionDraft(draftToSave);
      if (isNotifierRuntimeDraftOnAgentPage(draftToSave, selectedAgentForPage)) {
        if (!notifierFormIsComplete(draftToSave, selectedAgentForPage)) {
          setAgentPageSaveError(t("profileSaveIncompleteError"));
          return;
        }
        const runtimeOptions = draftRuntimeOptionsForSave(draft, { mergeNotifier: true });
        const payload: AgentUpdatePayload = {
          name: draftToSave.name,
          description: draftToSave.description,
          instructions: draftToSave.instructions,
        };
        if (runtimeOptions) {
          payload.runtime_options = runtimeOptions;
        }
        const saved = await patchNotificationBotRequest(selectedAgentForPage.id, payload);
        await saveLinkedAgentUserAvatar(selectedAgentForPage, draft.avatar);
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        await refreshAgentSkills(saved.id || selectedAgentForPage.id);
        const savedDraft = agentToDraft({ ...saved, avatar: draft.avatar });
        setAgentPageDraft(savedDraft);
        setAgentPageSavedDraft(savedDraft);
        return;
      }
      const profile = draftToProfile(draft, {
        name: draftToSave.name,
        description: draftToSave.description,
      });
      const runtimeOptions = draftRuntimeOptionsForSave(draft, {
        mergeNotifier: false,
      });
      const mcpServers = draftMCPServersForSave(draft);
      const profileChanged = profilePayloadForCompare(draftToSave) !== profilePayloadForCompare(agentPageSavedDraft);
      const runtimeOptionsChanged =
        runtimeOptionsPayloadForCompare(draftToSave) !== runtimeOptionsPayloadForCompare(agentPageSavedDraft);
      const mcpServersChanged =
        mcpServersPayloadForCompare(draftToSave) !== mcpServersPayloadForCompare(agentPageSavedDraft);
      const hasProfileOrRuntimeChange =
        profileChanged || (runtimeOptionsChanged && hasObjectValues(runtimeOptions)) || mcpServersChanged;

      const payload = agentPageBaseUpdatePayload(draftToSave);
      if (profileChanged) {
        payload.agent_profile = profile;
        payload.profile = profileSelectorFromDraft(draft);
      }
      if (runtimeOptionsChanged) {
        payload.runtime_options = runtimeOptions || {};
      }
      if (mcpServersChanged && mcpServers !== undefined) {
        payload.mcpServers = mcpServers;
      }
      if (!hasProfileOrRuntimeChange) {
        debugAgentPageSavePayload("meta-only", payload);
        const savedMetaOnly = await updateAgentRequest(selectedAgentForPage.id, payload);
        await saveLinkedAgentUserAvatar(selectedAgentForPage, draft.avatar);
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        if (savedMetaOnly.id === MANAGER_AGENT_ID) {
          await refreshManagerProfile();
        }
        await refreshAgentSkills(savedMetaOnly.id || selectedAgentForPage.id);
        const { draft: nextDraft } = await agentDraftFromItem({ ...savedMetaOnly, avatar: draft.avatar });
        setAgentPageDraft(nextDraft);
        setAgentPageSavedDraft(nextDraft);
        return;
      }
      const llmProfileChanged = agentPageLLMProfileChanged(draftToSave, agentPageSavedDraft);
      if (llmProfileChanged && !isAgentProfileDraftComplete(draftToSave)) {
        setAgentPageSaveError(t("profileSaveIncompleteError"));
        return;
      }
      debugAgentPageSavePayload("full", payload);
      const managerBeforeSave = selectedAgentForPage;
      const profileIncompleteBeforeSave = !isAgentProfileMarkedComplete(agentPageSavedDraft);
      const saved = await updateAgentRequest(selectedAgentForPage.id, payload);
      await saveLinkedAgentUserAvatar(selectedAgentForPage, draft.avatar);
      if (mcpServersChanged) {
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentMCPServers(selectedAgentForPage.id) });
      }
      if (canApplyAgentPageProfileSaveImmediately(saved, profileChanged, runtimeOptionsChanged, mcpServersChanged)) {
        const savedWithAvatar = { ...saved, avatar: draft.avatar };
        applyAgentListUpdate(savedWithAvatar);
        const savedDraft = agentToDraft(savedWithAvatar);
        setAgentPageDraft(savedDraft);
        setAgentPageSavedDraft(savedDraft);
        return;
      }
      await refreshAgentsWithUpdatedAgent(saved);
      if (saved.id === MANAGER_AGENT_ID && profileChanged) {
        void syncManagerRuntimeAfterProfileSave(managerBeforeSave, profileIncompleteBeforeSave);
      }
      await refreshWorkspaceBootstrap();
      if (saved.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
      }
      await refreshAgentSkills(saved.id || selectedAgentForPage.id);
      const { draft: savedDraft } = await agentDraftFromItem({ ...saved, avatar: draft.avatar });
      setAgentPageDraft(savedDraft);
      setAgentPageSavedDraft(savedDraft);
      if (
        profileChanged &&
        saved.id === MANAGER_AGENT_ID &&
        !isAgentProfileMarkedComplete(saved) &&
        !isAgentProfileMarkedComplete(savedDraft)
      ) {
        setAgentPageSaveError(t("profileSaveIncompleteError"));
        showAgentPageNotice(t("profileSetupIncompleteAfterSave"), "warning", 5000, saved.id);
      }
    } catch (err) {
      setAgentPageSaveError(errorMessage(err, t("agentActionFailed")), apiErrorBillingURL(err));
    } finally {
      setAgentPageBusy(false);
    }
  }

  async function publishAgentPage(
    target: AgentTemplatePublishTarget,
    name: string,
    description: string,
    includeMemory: boolean,
  ): Promise<boolean> {
    if (!selectedAgentForPage?.id || agentPagePublishBusy) {
      return false;
    }
    if (target !== "local" && !openCSGAuthenticated) {
      setAgentPageSaveError(t("agentPublishLoginRequired"));
      return false;
    }
    if (target !== "local" && agentRuntimeKind(selectedAgentForPage) !== "codex") {
      return false;
    }
    setAgentPagePublishBusy(true);
    setAgentPagePublishError("");
    setAgentPageSaveError("");
    try {
      const published = await publishAgentTemplateRequest(
        selectedAgentForPage.id,
        target,
        name,
        description,
        includeMemory,
      );
      await refreshHubTemplates();
      if (published?.id) {
        setSelectedHubTemplateId(published.id);
        navigatePane({ type: WorkspacePaneTypes.hub, id: published.id, resourceType: "template" }, rooms);
      } else {
        selectHub();
      }
      return true;
    } catch (err) {
      const errorCode = hubTemplateErrorCode(err);
      const deploySensitiveCheckFailed = errorCode === HubTemplateErrorCodes.reviewFailed;
      const deployReviewPending = errorCode === HubTemplateErrorCodes.reviewPending;
      const publishedTemplateID = String((err as ApiError | null)?.publishedTemplateId ?? "").trim();
      const message = errorMessage(err, t("agentActionFailed"));
      if (target === "official_deploy" && publishedTemplateID) {
        await refreshHubTemplates();
        if (deploySensitiveCheckFailed || deployReviewPending) {
          queryClient.setQueryData<HubTemplate[]>(workspaceQueryKeys.hubTemplates(), (templates) =>
            upsertHubTemplateReviewState(
              templates,
              publishedTemplateID,
              deployReviewPending ? "Pending" : "Fail",
              deployReviewPending ? "" : message,
            ),
          );
        }
        // Publishing succeeded, but deployment did not. Keep the upstream
        // result visible after navigating to the newly published template,
        // including the common case where review is still pending.
        setHubPublishError(message);
        setSelectedHubTemplateId(publishedTemplateID);
        navigatePane({ type: WorkspacePaneTypes.hub, id: publishedTemplateID, resourceType: "template" }, rooms);
        return true;
      }
      setAgentPagePublishError(message);
      setAgentPageSaveError(message);
      return false;
    } finally {
      setAgentPagePublishBusy(false);
    }
  }

  async function saveAgent(): Promise<void> {
    if (!agentDraft) {
      return;
    }
    setAgentBusy(true);
    setAgentError("");
    setAgentBillingURL("");
    const isCreate = agentModalMode === "create";
    const editingAgentID = String(editingAgent?.id ?? "").trim();
    if (!isCreate && !editingAgentID) {
      setAgentError(t("agentActionFailed"));
      setAgentBusy(false);
      return;
    }
    const isNotification = isNotificationBotDraftContext(
      agentDraft,
      editingAgent,
      isCreate ? agentCreateBotKind : undefined,
    );
    const runtimeSelection = resolveRuntimeSelection({
      runtime_kind: agentDraft.runtime_kind,
      runtime_name: agentDraft.runtime_name,
      sandbox_enabled: agentDraft.sandbox_enabled,
    });
    const runtimeKind = normalizeRuntimeKind(runtimeSelection.runtime_kind) || DEFAULT_RUNTIME_KIND;
    setAgentProgress(isCreate ? startAgentCreateProgress(isNotification ? BOT_TYPE_NOTIFICATION : runtimeKind) : null);
    try {
      const draft = ensureNotifierPullSubscriptionDraft(agentDraft);
      if (isCreate && !isNotification && agentDraftMissingRequiredEnv(draft)) {
        setAgentError(t("profileEnvRequiredError"));
        setAgentProgress(null);
        return;
      }
      if (isNotification) {
        const runtimeOptions = draftRuntimeOptionsForSave(draft, { mergeNotifier: true });
        const payload: AgentUpdatePayload = {
          name: agentDraft.name,
          description: agentDraft.description,
          instructions: agentDraft.instructions,
        };
        if (runtimeOptions) {
          payload.runtime_options = runtimeOptions;
        }
        const saved = await (isCreate
          ? createNotificationBotRequest(payload)
          : patchNotificationBotRequest(editingAgentID, payload));
        const avatarOwner = saved?.user_id || saved?.participants?.length ? saved : editingAgent || saved;
        await saveLinkedAgentUserAvatar(avatarOwner, agentDraft.avatar);
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        if (!isCreate) {
          await refreshAgentSkills(editingAgentID);
        }
        if (isCreate) {
          setAgentProgress((current) =>
            current
              ? { ...current, percent: 100, status: "done", index: Math.max(0, (current.steps?.length || 1) - 1) }
              : current,
          );
          selectAgent(saved, { replace: true });
        }
        setShowAgentModal(false);
        setAgentDraft(null);
        setAgentProgress(null);
        return;
      }
      const profile = draftToProfile(draft, {
        name: agentDraft.name,
        description: agentDraft.description,
      });
      const runtimeOptions = draftRuntimeOptionsForSave(draft, {
        mergeNotifier: false,
      });
      const mcpServers = draftMCPServersForSave(draft);
      const payload: AgentUpdatePayload = {
        name: agentDraft.name,
        role: WORKER_AGENT_ROLE,
        description: agentDraft.description,
        instructions: agentDraft.instructions,
        image: runtimeSelection.sandbox_enabled ? agentDraft.image : "",
        runtime_name: runtimeSelection.runtime_name,
        sandbox_enabled: runtimeSelection.sandbox_enabled,
        runtime_kind: runtimeKind,
        from_template: agentDraft.from_template || "",
        agent_profile: profile,
        profile: profileSelectorFromDraft(draft),
      };
      const editingDraftBaseline = editingAgent ? agentToDraft(editingAgent) : null;
      const runtimeOptionsChanged = !isCreate
        ? runtimeOptionsPayloadForCompare(agentDraft) !== runtimeOptionsPayloadForCompare(editingDraftBaseline)
        : Boolean(runtimeOptions);
      const mcpServersChanged = !isCreate
        ? mcpServersPayloadForCompare(agentDraft) !== mcpServersPayloadForCompare(editingDraftBaseline)
        : Boolean(mcpServers);
      if (isCreate) {
        if (runtimeOptions) {
          payload.runtime_options = runtimeOptions;
        }
        if (mcpServers !== undefined && mcpServers !== null) {
          payload.mcpServers = mcpServers;
        }
      } else if (runtimeOptionsChanged) {
        payload.runtime_options = runtimeOptions || {};
      }
      if (!isCreate && mcpServersChanged && mcpServers !== undefined) {
        payload.mcpServers = mcpServers;
      }
      const saved = isCreate
        ? await (async () => {
            const existingNames = agentItems.map((item) => String(item.name || "").trim()).filter(Boolean);
            let createPayload: AgentUpdatePayload = { ...payload };
            for (let attempt = 0; attempt <= AGENT_CREATE_NAME_RETRY_LIMIT; attempt += 1) {
              try {
                return await createBotRequest(createPayload);
              } catch (error) {
                if (!createPayload.from_template || !isAgentNameConflictError(error)) {
                  throw error;
                }
                const nextName = nextAvailableAgentName(String(createPayload.name || ""), existingNames);
                if (!nextName || nextName === createPayload.name) {
                  throw error;
                }
                existingNames.push(nextName);
                createPayload = { ...createPayload, name: nextName };
              }
            }
            return createBotRequest(createPayload);
          })()
        : await updateAgentRequest(editingAgentID, {
            name: payload.name,
            description: payload.description,
            instructions: payload.instructions,
            agent_profile: payload.agent_profile,
            profile: payload.profile,
            ...(payload.runtime_options !== undefined ? { runtime_options: payload.runtime_options } : {}),
            ...(payload.mcpServers !== undefined ? { mcpServers: payload.mcpServers } : {}),
          });
      await saveLinkedAgentUserAvatar(saved?.participants?.length ? saved : editingAgent || saved, agentDraft.avatar);
      if (!isCreate && mcpServersChanged) {
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentMCPServers(editingAgentID) });
      }
      if (isCreate) {
        saveLastCreatedAgentModelPreference(agentDraft);
      }
      await refreshAgents();
      await refreshWorkspaceBootstrap();
      if (saved.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
      }
      await refreshAgentSkills(saved.id || editingAgentID);
      if (isCreate) {
        setAgentProgress((current) =>
          current
            ? { ...current, percent: 100, status: "done", index: Math.max(0, (current.steps?.length || 1) - 1) }
            : current,
        );
        selectAgent(saved, { replace: true });
      }
      setShowAgentModal(false);
      setAgentDraft(null);
      setAgentProgress(null);
    } catch (err) {
      setAgentProgress((current) => (current ? { ...current, status: "failed" } : current));
      setAgentError(errorMessage(err, t("agentActionFailed")));
      setAgentBillingURL(apiErrorBillingURL(err));
    } finally {
      setAgentBusy(false);
    }
  }

  async function runAgentAction(item: AgentLike | null | undefined, action: AgentAction): Promise<void> {
    if (!item?.id || isAgentActionBusy(item.id)) {
      return;
    }
    if (
      isNotificationBotAgent(item) &&
      (action === "recreate" || action === "start" || action === "stop" || action === "upgrade")
    ) {
      return;
    }
    if (action === "recreate" && isManagerAgent(item)) {
      const busyKey = `${item.id}:${action}`;
      if (!claimAgentAction(item.id, busyKey)) {
        return;
      }
      clearAgentOperationError(item);
      showAgentPageNotice(t("managerRecreateInProgress"), "info", 0, item.id);
      try {
        const rebuildError = await rebuildManagerFromBrowser();
        if (rebuildError) {
          clearAgentPageNotice(item.id);
          setAgentOperationError(item, rebuildError.message, rebuildError.billingURL);
        } else {
          showAgentPageNotice(t("managerRecreateSucceeded"), "success", 5000, item.id);
        }
      } finally {
        releaseAgentAction(item.id, busyKey);
      }
      return;
    }
    const busyKey = `${item.id}:${action}`;
    if (!claimAgentAction(item.id, busyKey)) {
      return;
    }
    clearAgentOperationError(item);
    const showRecreateNotice = action === "recreate" && agentOperationUsesPageError(item);
    const recreationNoticeName = String(item.name || item.id).trim();
    if (showRecreateNotice) {
      showAgentPageNotice(t("agentRecreateInProgress", { name: recreationNoticeName }), "info", 0, item.id);
    }
    try {
      let updatedAgent: AgentLike | null = null;
      const deletingNotificationBot = action === "delete" && isNotificationBotAgent(item);
      if (action === "delete") {
        await deleteAgentLikeRequest(item);
        onAgentDeleted?.(item);
        if (activePane.type === WorkspacePaneTypes.agent && activePane.id === item.id) {
          const fallbackAgent = agentSelectionAfterDelete(agentItems, item.id);
          if (fallbackAgent) {
            selectAgent(fallbackAgent, { replace: true });
            showAgentPageNotice(
              t("agentDeletedAndSwitched", {
                deleted: String(item.name || item.id),
                selected: String(fallbackAgent.name || fallbackAgent.id),
              }),
              "success",
              5000,
              fallbackAgent.id,
            );
          }
        }
      } else {
        updatedAgent = await runAgentActionRequest(item.id, action);
      }
      await refreshAgentsWithUpdatedAgent(updatedAgent);
      if (action === "delete" && !deletingNotificationBot) {
        await refreshAgentSkills(item.id);
      }
      if (item.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
        if (action === "recreate" || action === "start") {
          await syncAgentStateUntilRunning(MANAGER_AGENT_ID);
        }
      }
      if (showRecreateNotice) {
        showAgentPageNotice(t("agentRecreateSucceeded", { name: recreationNoticeName }), "success", 5000, item.id);
      }
    } catch (err) {
      if (showRecreateNotice) {
        clearAgentPageNotice(item.id);
      }
      setAgentOperationError(item, errorMessage(err, t("agentActionFailed")), apiErrorBillingURL(err));
    } finally {
      releaseAgentAction(item.id, busyKey);
    }
  }

  const updateFeishuPendingRegistrations = useCallback(
    (updater: (current: Record<string, FeishuPendingRegistration>) => Record<string, FeishuPendingRegistration>) => {
      setFeishuPendingRegistrations((current) => {
        const next = pruneFeishuPendingRegistrations(updater(current));
        saveFeishuPendingRegistrations(next);
        return next;
      });
    },
    [],
  );

  const completeFeishuPendingRegistration = useCallback(
    async (
      pending: FeishuPendingRegistration,
      options: { background?: boolean; showPendingNotice?: boolean } = {},
    ): Promise<void> => {
      const agentID = String(pending.agent_id || "").trim();
      const registrationID = String(pending.registration_id || "").trim();
      if (
        !agentID ||
        !registrationID ||
        isAgentActionBusy(agentID) ||
        feishuAutoFinalizeActiveRef.current.has(registrationID)
      ) {
        return;
      }
      const background = Boolean(options.background);
      const busyKey = feishuActionKey(agentID, "finalize");
      if (!claimAgentAction(agentID, busyKey, !background)) {
        return;
      }
      feishuAutoFinalizeActiveRef.current.add(registrationID);
      if (!background) {
        setAgentPageSaveError("");
      }
      try {
        const result = await finalizeFeishuRegistrationRequest(registrationID);
        if (String(result?.status || "").trim() === "pending") {
          const nextPending = normalizeFeishuPendingRegistration({ ...pending, ...result }, agentID) ?? pending;
          updateFeishuPendingRegistrations((current) => ({
            ...current,
            [agentID]: nextPending,
          }));
          if (options.showPendingNotice) {
            showAgentPageNotice(t("feishuConnectPending"), "warning", 5000, agentID);
          }
          return;
        }
        updateFeishuPendingRegistrations((current) => {
          const next = { ...current };
          delete next[agentID];
          return next;
        });
        await refreshAgentStateRef.current(agentID);
        const finalizeNotice = feishuRegistrationFinalizeNotice(result);
        if (finalizeNotice.kind === "unavailable") {
          showAgentPageNotice(t("feishuConnectConfiguredLarkCLIUnavailable"), "warning", 8000, agentID);
        } else if (finalizeNotice.kind === "bind_failed") {
          showAgentPageNotice(t("feishuConnectConfiguredLarkCLIBindFailed"), "warning", 8000, agentID);
        } else if (finalizeNotice.kind === "error") {
          showAgentPageNotice(t("feishuConnectConfiguredLarkCLIError"), "warning", 8000, agentID);
        } else if (finalizeNotice.kind === "warnings") {
          showAgentPageNotice(
            t("feishuConnectConfiguredWithWarnings", { warnings: finalizeNotice.warnings.join("; ") }),
            "warning",
            8000,
            agentID,
          );
        } else {
          showAgentPageNotice(t("feishuConnectConfigured"), "success", 5000, agentID);
        }
      } catch (err) {
        if (feishuRegistrationFinalizeClearsPending(err)) {
          updateFeishuPendingRegistrations((current) => {
            const next = { ...current };
            delete next[agentID];
            return next;
          });
        }
        if (!background) {
          setAgentPageSaveError(errorMessage(err, t("feishuConnectFailed")));
        }
      } finally {
        feishuAutoFinalizeActiveRef.current.delete(registrationID);
        releaseAgentAction(agentID, busyKey);
      }
    },
    [
      claimAgentAction,
      errorMessage,
      isAgentActionBusy,
      releaseAgentAction,
      showAgentPageNotice,
      t,
      updateFeishuPendingRegistrations,
    ],
  );

  useEffect(() => {
    const timers: number[] = [];
    Object.entries(feishuPendingRegistrations).forEach(([agentID, registration]) => {
      const pending = normalizeFeishuPendingRegistration(registration, agentID);
      if (!pending || isAgentActionBusy(agentID) || feishuAutoFinalizeActiveRef.current.has(pending.registration_id)) {
        return;
      }
      const timer = window.setTimeout(() => {
        void completeFeishuPendingRegistration(pending, { background: true });
      }, feishuRegistrationPollDelayMs(pending));
      timers.push(timer);
    });
    return () => {
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [agentActionBusyByAgent, completeFeishuPendingRegistration, feishuPendingRegistrations, isAgentActionBusy]);

  const finalizeVisibleFeishuPendingRegistrations = useCallback(() => {
    Object.entries(feishuPendingRegistrations).forEach(([agentID, registration]) => {
      const pending = normalizeFeishuPendingRegistration(registration, agentID);
      if (!pending || isAgentActionBusy(agentID) || feishuAutoFinalizeActiveRef.current.has(pending.registration_id)) {
        return;
      }
      void completeFeishuPendingRegistration(pending, { background: true });
    });
  }, [completeFeishuPendingRegistration, feishuPendingRegistrations, isAgentActionBusy]);

  useEffect(() => {
    window.addEventListener("focus", finalizeVisibleFeishuPendingRegistrations);
    return () => {
      window.removeEventListener("focus", finalizeVisibleFeishuPendingRegistrations);
    };
  }, [finalizeVisibleFeishuPendingRegistrations]);

  async function startFeishuConnect(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    if (!agentID || isAgentActionBusy(agentID)) {
      return;
    }
    const busyKey = feishuActionKey(agentID, "connect");
    if (!claimAgentAction(agentID, busyKey)) {
      return;
    }
    setAgentPageSaveError("");
    try {
      const registration = await startFeishuRegistrationRequest(agentID);
      const pending = normalizeFeishuPendingRegistration(registration, agentID);
      if (!pending) {
        throw new Error(t("feishuConnectFailed"));
      }
      updateFeishuPendingRegistrations((current) => ({
        ...current,
        [pending.agent_id]: pending,
      }));
      const connectURL = String(pending.connect_url || "").trim();
      if (connectURL) {
        window.open(connectURL, "_blank", "noopener,noreferrer");
      }
      showAgentPageNotice(t("feishuConnectStarted"), "info", 5000, agentID);
    } catch (err) {
      setAgentPageSaveError(errorMessage(err, t("feishuConnectFailed")));
    } finally {
      releaseAgentAction(agentID, busyKey);
    }
  }

  async function finalizeFeishuConnect(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    const pending = normalizeFeishuPendingRegistration(feishuPendingRegistrations[agentID], agentID);
    if (!agentID || !pending || isAgentActionBusy(agentID)) {
      return;
    }
    await completeFeishuPendingRegistration(pending, { showPendingNotice: true });
  }

  async function disconnectFeishu(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    const participantID = String(feishuAgentParticipant(item)?.id || "").trim();
    if (!agentID || !participantID || isAgentActionBusy(agentID)) {
      return;
    }
    const busyKey = feishuActionKey(agentID, "disconnect");
    if (!claimAgentAction(agentID, busyKey)) {
      return;
    }
    setAgentPageSaveError("");
    try {
      await deleteFeishuParticipantRequest(participantID);
      updateFeishuPendingRegistrations((current) => {
        const next = { ...current };
        delete next[agentID];
        return next;
      });
      await refreshAgentStateRef.current(agentID);
      showAgentPageNotice(t("feishuDisconnectConfigured"), "success", 5000, agentID);
    } catch (err) {
      setAgentPageSaveError(errorMessage(err, t("feishuDisconnectFailed")));
    } finally {
      releaseAgentAction(agentID, busyKey);
    }
  }

  async function initAgentLarkCLI(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    if (!agentID || isAgentActionBusy(agentID)) {
      return;
    }
    const busyKey = feishuActionKey(agentID, "lark-cli");
    if (!claimAgentAction(agentID, busyKey)) {
      return;
    }
    setAgentPageSaveError("");
    try {
      const result = await initAgentLarkCLIRequest(agentID);
      await refreshAgentStateRef.current(agentID);
      showAgentPageNotice(t("larkCLIConfigured"), "success", 5000, agentID);
      if (result?.restart_error) {
        setAgentPageSaveError(`${t("larkCLIRestartFailed")} ${result.restart_error}`);
      }
    } catch (err) {
      const apiError = err as ApiError | null;
      if (apiError?.code === "feishu_bot_not_configured") {
        setLarkCLIDialog({
          kind: "message",
          title: t("larkCLINoFeishuBotTitle"),
          message: t("larkCLINoFeishuBotMessage"),
        });
      } else if (apiError?.code === "lark_cli_unavailable") {
        showLarkCLIInstall(item);
      } else {
        setAgentPageSaveError(errorMessage(err, t("larkCLIInitFailed")));
      }
    } finally {
      releaseAgentAction(agentID, busyKey);
    }
  }

  function showLarkCLIInstall(item: AgentLike | null | undefined): void {
    if (!String(item?.id || "").trim()) {
      return;
    }
    setLarkCLIDialog({
      kind: "install",
      title: t("larkCLIInstallTitle"),
      message: t("larkCLIInstallMessage"),
    });
  }

  async function deletePreviewBot(item: AgentLike | null | undefined) {
    if (!item?.id || isAgentActionBusy(item.id)) {
      return false;
    }
    if (!window.confirm(`${t("agentDelete")} ${item.name}?`)) {
      return false;
    }
    const busyKey = `${item.id}:delete-bot`;
    if (!claimAgentAction(item.id, busyKey)) {
      return false;
    }
    clearAgentOperationError(item);
    try {
      await deleteBotRequest(csgclawParticipantIDForAgent(item));
      await refreshAgents();
      await refreshWorkspaceBootstrap();
      if (item.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
      }
      return true;
    } catch (err) {
      setAgentOperationError(item, errorMessage(err, t("agentActionFailed")));
      return false;
    } finally {
      releaseAgentAction(item.id, busyKey);
    }
  }

  async function inviteAgentToRoom(item: AgentLike | null | undefined, options: { silent?: boolean } = {}) {
    if (!activeConversation || isDirectConversation(activeConversation) || !data?.current_user_id || !item?.id) {
      return;
    }
    if (!options.silent) {
      clearAgentOperationError(item);
    }
    try {
      await joinAgentToRoomRequest({
        agent_id: item.id,
        room_id: activeConversation.id,
        inviter_id: data.current_user_id,
        locale,
      });
      await refreshWorkspaceBootstrap();
    } catch (err) {
      if (!options.silent) {
        setAgentOperationError(item, errorMessage(err, t("agentActionFailed")));
      }
    }
  }

  async function createAgentTeam(payload: CreateTeamPayload): Promise<void> {
    if (teamActionBusy) {
      return;
    }
    setTeamActionBusy(true);
    setTeamActionError("");
    try {
      await createTeamRequest(payload);
      await teamsQuery.refetch();
      await refreshWorkspaceBootstrap();
    } catch (err) {
      setTeamActionError(errorMessage(err, t("teamActionFailed")));
      throw err;
    } finally {
      setTeamActionBusy(false);
    }
  }

  async function createTeam(): Promise<void> {
    await createAgentTeam({
      title: createTeamTitle.trim() || t("teamNewFallbackTitle"),
      lead_agent_id: MANAGER_AGENT_ID,
      member_agent_ids: createTeamMemberIDs,
    });
    closeCreateTeamModal();
  }

  async function saveTeamMembers(): Promise<void> {
    if (!editingTeam?.id || teamActionBusy) {
      return;
    }
    setTeamActionBusy(true);
    setTeamActionError("");
    try {
      await updateTeamRequest(editingTeam.id, {
        member_agent_ids: createTeamMemberIDs.filter(
          (memberID) => !localIdentitiesMatch(memberID, editingTeam.lead_agent_id),
        ),
      });
      await teamsQuery.refetch();
      await refreshWorkspaceBootstrap();
      closeCreateTeamModal();
    } catch (err) {
      setTeamActionError(errorMessage(err, t("teamActionFailed")));
      throw err;
    } finally {
      setTeamActionBusy(false);
    }
  }

  async function deleteTeam(item: WorkspaceTeam | null | undefined): Promise<boolean> {
    const teamID = String(item?.id || "").trim();
    if (!teamID || teamActionBusy) {
      return false;
    }
    setTeamActionBusy(true);
    setTeamActionError("");
    try {
      await deleteTeamRequest(teamID);
      await teamsQuery.refetch();
      await refreshWorkspaceBootstrap();
      if (activePane.type === WorkspacePaneTypes.team && activePane.id === teamID) {
        selectComputer({ replace: true });
      }
      return true;
    } catch (err) {
      setTeamActionError(errorMessage(err, t("teamActionFailed")));
      return false;
    } finally {
      setTeamActionBusy(false);
    }
  }

  const batchAddAgentSkills = useCallback(
    async (skillNames: string[]) => {
      if (!agentDetailAgentID || agentSkillAddBusy) {
        return false;
      }
      const names = skillNames.map((name) => String(name || "").trim()).filter(Boolean);
      if (!names.length) {
        return false;
      }
      setAgentSkillAddBusy(true);
      setAgentSkillAddError("");
      try {
        await batchAddAgentSkillsRequest(agentDetailAgentID, names);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(agentDetailAgentID) });
        return true;
      } catch (err) {
        setAgentSkillAddError(errorMessage(err, t("agentSkillAddFailed")));
        return false;
      } finally {
        setAgentSkillAddBusy(false);
      }
    },
    [agentSkillAddBusy, errorMessage, queryClient, agentDetailAgentID, t],
  );

  const deleteAgentSkill = useCallback(
    async (skill: { name?: string | null } | string | null | undefined) => {
      if (!agentDetailAgentID || agentSkillDeleteBusy) {
        return false;
      }
      const rawName = typeof skill === "string" ? skill : String(skill?.name || "");
      const name = rawName.trim();
      if (!name) {
        return false;
      }
      setAgentSkillDeleteBusy(true);
      setAgentSkillDeleteError("");
      try {
        await deleteAgentSkillRequest(agentDetailAgentID, name);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(agentDetailAgentID) });
        return true;
      } catch (err) {
        setAgentSkillDeleteError(errorMessage(err, t("agentSkillDeleteFailed")));
        return false;
      } finally {
        setAgentSkillDeleteBusy(false);
      }
    },
    [agentSkillDeleteBusy, errorMessage, queryClient, agentDetailAgentID, t],
  );

  const installAgentMCPServers = useCallback(
    async (serverNames: string[]) => {
      if (!agentDetailAgentID || agentMCPAddBusy) {
        return false;
      }
      const names = serverNames.map((name) => String(name || "").trim()).filter(Boolean);
      if (!names.length) {
        return false;
      }
      setAgentMCPAddBusy(true);
      setAgentMCPAddError("");
      setAgentMCPDeleteError("");
      try {
        const view = await batchAddAgentMCPServersRequest(agentDetailAgentID, names);
        const mcpServers = cloneMCPServersForDraft(view.servers);
        setAgentPageDraft((current) => (current ? { ...current, mcpServers } : current));
        setAgentPageSavedDraft((current) => (current ? { ...current, mcpServers } : current));
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentMCPServers(agentDetailAgentID) }),
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agents() }),
        ]);
        return true;
      } catch (err) {
        setAgentMCPAddError(errorMessage(err, t("agentActionFailed")));
        return false;
      } finally {
        setAgentMCPAddBusy(false);
      }
    },
    [agentMCPAddBusy, errorMessage, queryClient, agentDetailAgentID, t],
  );

  const syncAgentMCPServerSnapshot = useCallback(
    async (server: { name?: string | null } | string | null | undefined) => {
      const name = (typeof server === "string" ? server : String(server?.name || "")).trim();
      if (!agentDetailAgentID || !name || agentMCPSourceSyncBusyName) {
        return false;
      }
      setAgentMCPSourceSyncBusyName(name);
      setAgentMCPAddError("");
      setAgentMCPDeleteError("");
      try {
        const result = await syncAgentMCPServerSource(agentDetailAgentID, name);
        const mcpServers = cloneMCPServersForDraft(result.agent.servers);
        queryClient.setQueryData(workspaceQueryKeys.agentMCPServers(agentDetailAgentID), result.agent);
        queryClient.setQueryData(workspaceQueryKeys.agentMCPServerSource(agentDetailAgentID, name), result.source);
        setAgentPageDraft((current) => (current ? { ...current, mcpServers } : current));
        setAgentPageSavedDraft((current) => (current ? { ...current, mcpServers } : current));
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agents() }),
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() }),
        ]);
        return true;
      } catch (err) {
        setAgentMCPAddError(errorMessage(err, t("agentMCPSourceSyncFailed")));
        return false;
      } finally {
        setAgentMCPSourceSyncBusyName("");
      }
    },
    [agentDetailAgentID, agentMCPSourceSyncBusyName, errorMessage, queryClient, t],
  );

  const deleteAgentMCPServer = useCallback(
    async (server: { name?: string | null } | string | null | undefined) => {
      if (!agentDetailAgentID || agentMCPDeleteBusy) {
        return false;
      }
      const rawName = typeof server === "string" ? server : String(server?.name || "");
      const name = rawName.trim();
      if (!name) {
        return false;
      }
      setAgentMCPDeleteBusy(true);
      setAgentMCPAddError("");
      setAgentMCPDeleteError("");
      try {
        const view = await batchDeleteAgentMCPServersRequest(agentDetailAgentID, [name]);
        const mcpServers = cloneMCPServersForDraft(view.servers);
        setAgentPageDraft((current) => (current ? { ...current, mcpServers } : current));
        setAgentPageSavedDraft((current) => (current ? { ...current, mcpServers } : current));
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentMCPServers(agentDetailAgentID) }),
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agents() }),
        ]);
        return true;
      } catch (err) {
        setAgentMCPDeleteError(errorMessage(err, t("agentActionFailed")));
        return false;
      } finally {
        setAgentMCPDeleteBusy(false);
      }
    },
    [agentMCPDeleteBusy, errorMessage, queryClient, agentDetailAgentID, t],
  );

  function directConversationForUser(
    userID: string | null | undefined,
    roomList: IMConversation[] = rooms,
    currentUserID: string | null | undefined = data?.current_user_id,
  ): IMConversation | null {
    if (!userID || !currentUserID) {
      return null;
    }
    return (
      roomList.find(
        (room) =>
          isDirectConversation(room) &&
          room.members.some((memberID) => localIdentitiesMatch(memberID, currentUserID)) &&
          room.members.some((memberID) => localIdentitiesMatch(memberID, userID)),
      ) ?? null
    );
  }

  async function openAgentDirectMessage(item: AgentLike | null | undefined): Promise<void> {
    const channelUserID = resolveAgentChannelUserID(item);
    if (!channelUserID || !data?.current_user_id) {
      return;
    }

    clearAgentOperationError(item);
    try {
      let nextData = null;
      let direct = directConversationForUser(channelUserID);
      if (!direct) {
        await createUserRequest({
          id: channelUserID,
          name: String(item?.name || channelUserID),
          role: item?.role || WORKER_AGENT_ROLE,
        });
        nextData = await refreshWorkspaceBootstrap();
        direct = directConversationForUser(
          channelUserID,
          nextData?.rooms ?? rooms,
          nextData?.current_user_id ?? data.current_user_id,
        );
      }

      if (!direct) {
        setAgentOperationError(item, t("agentActionFailed"));
        return;
      }
      const nextRooms = nextData?.rooms ?? rooms;
      selectConversation(direct.id, { rooms: nextRooms });
    } catch (err) {
      setAgentOperationError(item, errorMessage(err, t("agentActionFailed")));
    }
  }

  const selectedAgentAction = selectedAgentForPage?.id ? agentActionBusyByAgent[selectedAgentForPage.id] : undefined;
  const selectedAgentActionBusy = selectedAgentAction?.visible ? selectedAgentAction.busyKey : "";
  const selectedAgentOwnedNotice = selectedAgentForPage?.id ? agentPageNotices[selectedAgentForPage.id] : undefined;
  const selectedAgentPageNotice = selectedAgentOwnedNotice || agentPageNotices[GLOBAL_AGENT_PAGE_NOTICE_KEY];

  return {
    agentActionBusy: selectedAgentActionBusy,
    agentActionBusyKeys,
    agentItems,
    agentsDisplayError,
    cliproxyAuthBusy,
    cliproxyAuthStatuses,
    deletePreviewBot,
    handleMessageAction,
    loginCLIProxyProvider,
    managerAgent,
    managerProfileIncomplete,
    managerRuntimeUnavailable,
    managerRuntimeWarning,
    messageActionBusy,
    messageActionFeedback,
    openAgentDirectMessage,
    notificationAgentItems,
    openCreateAgentModal,
    openCreateTeamModal,
    openManageTeamMembers,
    openCreateNotificationParticipantModal,
    openEditAgentModal,
    runningAgentCount,
    runAgentAction,
    refreshAgentState,
    selectModelProvider,
    selectedAgentForPage,
    teams: teamsQuery.data ?? [],
    teamsLoading: teamsQuery.isLoading,
    deleteTeam,
    workerAgentItems,
    notifierWebhookPublicOrigin,
    agentViewProps: {
      item: selectedAgentForPage,
      t,
      locale,
      busyKey: selectedAgentActionBusy,
      error: "",
      draft: agentPageDraft,
      savedDraft: agentPageSavedDraft,
      hasUnsavedChanges: agentPageHasUnsavedChanges,
      models: agentPageModelOptions.map((option) => option.modelID),
      modelOptions: agentPageModelOptions,
      modelProviders: agentPageModelProviders,
      modelBusy: agentPageModelBusy,
      modelError: agentPageModelError,
      onRetryModels: retryAgentPageModels,
      saving: agentPageBusy,
      publishBusy: agentPagePublishBusy,
      publishDisabled: !openCSGAuthenticated,
      publishError: agentPagePublishError,
      saveError: agentPageError,
      saveBillingURL: agentPageBillingURL,
      notice: selectedAgentPageNotice?.message || "",
      noticeTone: selectedAgentPageNotice?.tone || "warning",
      onDismissNotice: () => clearAgentPageNotice(selectedAgentOwnedNotice ? selectedAgentForPage?.id : null),
      larkCLIDialog,
      feishuConnectBusy: selectedAgentActionBusy.includes(`:${FEISHU_CHANNEL_ACTION}:`) ? selectedAgentActionBusy : "",
      feishuPendingRegistration: selectedFeishuPendingRegistration,
      authStatuses: cliproxyAuthStatuses,
      authBusyProvider: cliproxyAuthBusy,
      notifierWebhookPublicOrigin,
      skillCandidates: agentSkillCandidates,
      skillCandidatesLoading: globalSkillsQuery.isFetching,
      skillCandidatesError: agentSkillCandidatesError,
      skillAddBusy: agentSkillAddBusy,
      skillAddError: agentSkillAddError,
      skillDeleteBusy: agentSkillDeleteBusy,
      skillDeleteError: agentSkillDeleteError,
      mcpCandidates: agentMCPCandidates,
      mcpCandidatesLoading: catalogMCPServersLoading,
      mcpCandidatesError: catalogMCPServersError,
      mcpServers: agentMCPServers,
      mcpUpdateAvailableNames: agentMCPUpdateAvailableNames,
      mcpSourceBusyNames: agentMCPSourceBusyNames,
      mcpSourceUnavailableNames: agentMCPSourceUnavailableNames,
      mcpSourceSyncBusyName: agentMCPSourceSyncBusyName,
      mcpAddBusy: agentMCPAddBusy,
      mcpAddError: agentMCPAddError,
      mcpDeleteBusy: agentMCPDeleteBusy,
      mcpDeleteError: agentMCPDeleteError,
      skills: agentSkillsQuery.data ?? [],
      skillsLoading: agentSkillsQuery.isFetching,
      skillsError: agentSkillsError,
      workspaceSupported: Boolean(selectedAgentForPage),
      directoryPickerAvailable: bootstrapConfig?.directory_picker_available !== false,
      onDraftChange: setAgentPageDraft,
      onSave: saveAgentPage,
      onMetadataSave: saveAgentPageMetadata,
      onMemoryChange: async () => {
        const agentID = String(selectedAgentForPage?.id ?? "").trim();
        if (agentID) {
          await refreshAgentState(agentID);
        }
      },
      onPublish: publishAgentPage,
      onProviderLogin: loginCLIProxyProvider,
      onStart: (item: AgentLike | null | undefined) => runAgentAction(item, "start"),
      onStop: (item: AgentLike | null | undefined) => runAgentAction(item, "stop"),
      onRecreate: (item: AgentLike | null | undefined) => runAgentAction(item, "recreate"),
      onUpgrade: (item: AgentLike | null | undefined) => runAgentAction(item, "upgrade"),
      onDelete: (item: AgentLike | null | undefined) => runAgentAction(item, "delete"),
      onInvite: inviteAgentToRoom,
      onOpenDM: openAgentDirectMessage,
      onStartFeishuConnect: startFeishuConnect,
      onFinalizeFeishuConnect: finalizeFeishuConnect,
      onDisconnectFeishu: disconnectFeishu,
      onInitLarkCLI: initAgentLarkCLI,
      onShowLarkCLIInstall: showLarkCLIInstall,
      onDismissLarkCLIDialog: () => setLarkCLIDialog(null),
      onAddSkills: batchAddAgentSkills,
      onDeleteSkill: deleteAgentSkill,
      onInstallMCPServers: installAgentMCPServers,
      onUpdateMCPServer: syncAgentMCPServerSnapshot,
      onDeleteMCPServer: deleteAgentMCPServer,
      onRetryMCPServers: refreshMCPServers,
      teamActionBusy,
      teamActionError,
      onCreateTeam: createAgentTeam,
    },
    computerViewProps: {
      t,
      agents: agentItems,
      activeAgentID: activePane.type === WorkspacePaneTypes.agent ? activePane.id : "",
      busyKeys: agentActionBusyKeys,
      onCreateAgent: openCreateAgentModal,
      onStartAgent: (item: AgentLike | null | undefined) => runAgentAction(item, "start"),
    },
    agentProfileModalProps:
      showAgentModal && agentDraft
        ? {
            t,
            locale,
            agentModalMode,
            agentCreateBotKind,
            agentCreateMode,
            onAgentCreateModeChange: setAgentCreateMode,
            onAgentCreateBotKindChange: setAgentCreateBotKind,
            editingAgent,
            agentDraft,
            onAgentDraftChange: setAgentDraft,
            onAgentModelsReset: resetAgentModels,
            hubTemplates,
            bootstrapConfig: agentModalBootstrapConfig || bootstrapConfig,
            managerAgent,
            agentModels: agentModelOptions.map((option) => option.modelID),
            agentModelOptions,
            modelProviders: agentModelProviders,
            agentModelBusy,
            authStatuses: cliproxyAuthStatuses,
            authBusyProvider: cliproxyAuthBusy,
            notifierWebhookPublicOrigin,
            onProviderLogin: loginCLIProxyProvider,
            agentError,
            agentBillingURL,
            agentProgress,
            agentBusy,
            onClose: () => setShowAgentModal(false),
            onSave: saveAgent,
          }
        : null,
    createTeamModalProps: showCreateTeamModal
      ? {
          t,
          mode: editingTeam ? ("edit" as const) : ("create" as const),
          candidates: createTeamCandidates,
          lockedTeamMemberIDs: lockedTeamMemberID ? [lockedTeamMemberID] : [],
          teamTitle: createTeamTitle,
          onTeamTitleChange: setCreateTeamTitle,
          teamMemberIDs: createTeamMemberIDs,
          onTeamMemberIDsChange: setCreateTeamMemberIDs,
          submitError: teamActionError,
          teamActionBusy,
          onClose: closeCreateTeamModal,
          onCreate: editingTeam ? saveTeamMembers : createTeam,
        }
      : null,
  };
}

function matchingTeamCandidateID(identity: string | null | undefined, candidateIDs: readonly string[]): string {
  const normalizedIdentity = String(identity || "").trim();
  if (!normalizedIdentity) {
    return "";
  }
  return (
    candidateIDs.find((candidateID) => localIdentitiesMatch(candidateID, normalizedIdentity)) || normalizedIdentity
  );
}

function teamSelectionMemberIDs(team: WorkspaceTeam, candidateIDs: readonly string[]): string[] {
  return Array.from(
    new Set(
      teamMemberIDs(team)
        .map((memberID) => matchingTeamCandidateID(memberID, candidateIDs))
        .filter(Boolean),
    ),
  );
}

function csgclawParticipantIDForAgent(item: AgentLike): string {
  const participant = item.participants?.find(
    (candidate) => String(candidate?.channel || "").trim() === "csgclaw" && String(candidate?.id || "").trim(),
  );
  return String(participant?.id || item.id || "").trim();
}
