import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker } from "react-router-dom";
import { errorMessage } from "@/api/client";
import { loginCLIProxyProviderRequest } from "@/api/cliproxy";
import {
  batchAddAgentSkillsRequest,
  createBotRequest,
  createManagerAgentRequest,
  createNotificationBotRequest,
  deleteAgentRequest,
  deleteAgentSkillRequest,
  deleteBotRequest,
  deleteFeishuParticipantRequest,
  fetchAgent,
  fetchAgentProfile,
  fetchAgentProfileDefaults,
  fetchAgentSkills,
  fetchAgentSkillsFile,
  finalizeFeishuRegistrationRequest,
  patchNotificationBotRequest,
  runAgentActionRequest,
  startFeishuRegistrationRequest,
  updateAgentRequest,
} from "@/api/agents";
import type { AgentUpdatePayload, FeishuRegistration, FetchAgentsOptions } from "@/api/agents";
import { patchCsgclawUserRequest } from "@/api/participants";
import { publishAgentTemplateRequest } from "@/api/hub";
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
import {
  applyTemplateToDraft,
  advanceAgentProgress,
  agentDraftWithRuntimeFieldsFromAgent,
  agentPageLLMProfileChanged,
  agentRuntimePollSettled,
  agentToDraft,
  isAgentProfileDraftComplete,
  isAgentProfileMarkedComplete,
  availableManagerRebuildRuntimeOptions,
  collectManagerTemplateVariants,
  composeLegacyRuntimeKind,
  defaultManagerRebuildImageForRuntime,
  defaultWorkerImageForRuntime,
  draftRuntimeOptionsForSave,
  draftToProfileComparePayload,
  draftToProfile,
  ensureNotifierPullSubscriptionDraft,
  feishuAgentParticipant,
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
  normalizeRuntimeKind,
  profileSelectorFromDraft,
  providerNeedsAuth,
  resolvedNotifierWebhookOrigin,
  resolveRuntimeSelection,
  resolveAgentChannelUserID,
  shouldWaitForManagerRuntimeAfterProfileSave,
  startAgentCreateProgress,
} from "@/models/agents";
import type {
  AgentCreateProgressState,
  AgentDraft,
  AgentLike,
  AgentProfileLike,
  AgentTemplateLike,
  RuntimeKind,
} from "@/models/agents";
import { isDirectConversation, localIdentitiesMatch, upsertUserInData } from "@/models/conversations";
import { displayTeam } from "@/models/tasks";
import type { WorkspaceTeam } from "@/models/tasks";
import { modelProviderOptionsFromCatalog, providerNameForProviderID } from "@/models/modelProviders";
import type { ModelProviderOption } from "@/models/modelProviders";
import { WorkspacePaneTypes } from "@/models/routing";
import { skillDescriptionFromMarkdown, skillOptionsFromWorkspace } from "@/models/slashCommands";
import { useCLIProxyAuthStatuses } from "./useCLIProxyAuthStatuses";
import { workspaceQueryKeys } from "./workspaceQueries";
import type { MessageAction, MessageActionError, MessageLike } from "@/components/business/MessageContent/types";
import type { IMConversation, IMUser, TranslateFn } from "@/models/conversations";
import type { UseAgentControllerArgs } from "./types";

type ManagerRebuildOptions = {
  runtimeKind?: RuntimeKind;
};

type AgentModalMode = "create" | "edit";
type AgentAction = "delete" | "recreate" | "start" | "stop" | "upgrade";

type FeishuPendingRegistration = FeishuRegistration & {
  agent_id: string;
  registration_id: string;
};

type AgentWithProfile = {
  agent: AgentLike;
  profile?: AgentProfileLike | null;
};

type AgentPageNoticeTone = "info" | "warning" | "success";
type FeishuActionKind = "connect" | "disconnect" | "finalize";

const AGENT_RUNTIME_SYNC_INTERVAL_MS = 2_000;
const AGENT_RUNTIME_SYNC_TIMEOUT_MS = 120_000;
const FEISHU_CHANNEL_ACTION = "feishu";
const FEISHU_REGISTRATION_DEFAULT_POLL_SECONDS = 3;
const FEISHU_REGISTRATION_MIN_POLL_SECONDS = 1;
const FEISHU_REGISTRATION_MAX_POLL_SECONDS = 30;

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

function draftWithModelProviderFallback(draft: AgentDraft, options: readonly ModelProviderOption[]): AgentDraft {
  const providerID = String(draft.model_provider_id || "").trim();
  const modelID = String(draft.model_id || "").trim();
  if (providerID && modelID) {
    return draft;
  }
  const option = options.find((item) => {
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
    return draft;
  }
  const nextProviderID = providerID || option.providerID;
  return {
    ...draft,
    provider: providerNameForProviderID(nextProviderID),
    model_provider_id: nextProviderID,
    model_id: modelID || option.modelID,
  };
}

export function shouldReturnToAgentOverviewAfterAgentMissing(
  activePane: { type?: string; id?: string | undefined } | null | undefined,
) {
  return activePane?.type === WorkspacePaneTypes.agent;
}

export function useAgentController({
  activeConversationId,
  activePane,
  agents,
  agentsLoaded,
  agentsQuery,
  bootstrapConfig,
  data,
  hubTemplates,
  locale,
  managerProfile,
  modelProviders = null,
  modelProvidersLoaded = false,
  refreshHubTemplates,
  refreshWorkspaceAgents,
  refreshWorkspaceBootstrap,
  refreshWorkspaceBootstrapConfig,
  refreshWorkspaceManagerProfile,
  refreshWorkspaceModelProviders = async () => null,
  rooms,
  selectAgent,
  selectComputer,
  selectConversation,
  selectHub,
  selectModelProvider = () => {},
  setAgentsData,
  setBootstrapData,
  setSelectedHubTemplateId,
  t,
}: UseAgentControllerArgs) {
  const queryClient = useQueryClient();
  const [cliproxyAuthBusy, setCLIProxyAuthBusy] = useState("");
  const [agentsError, setAgentsError] = useState("");
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [showManagerRebuildModal, setShowManagerRebuildModal] = useState(false);
  const [managerRebuildError, setManagerRebuildError] = useState("");
  const [managerRebuildRuntimeKind, setManagerRebuildRuntimeKind] = useState<RuntimeKind>(DEFAULT_RUNTIME_KIND);
  const [managerRebuildImage, setManagerRebuildImage] = useState("");
  const [agentModalMode, setAgentModalMode] = useState<AgentModalMode>("create");
  const [agentCreateBotKind, setAgentCreateBotKind] = useState(BOT_CREATE_KIND_WORKER);
  const [editingAgent, setEditingAgent] = useState<AgentLike | null>(null);
  const [agentDraft, setAgentDraft] = useState<AgentDraft | null>(null);
  const [agentBusy, setAgentBusy] = useState(false);
  const [agentError, setAgentError] = useState("");
  const [agentProgress, setAgentProgress] = useState<AgentCreateProgressState | null>(null);
  const [agentActionBusy, setAgentActionBusy] = useState("");
  const [messageActionBusy] = useState("");
  const [messageActionError, setMessageActionError] = useState<MessageActionError>({ key: "", message: "" });
  const [agentPageDraft, setAgentPageDraft] = useState<AgentDraft | null>(null);
  const [agentPageSavedDraft, setAgentPageSavedDraft] = useState<AgentDraft | null>(null);
  const [agentPageBusy, setAgentPageBusy] = useState(false);
  const [agentPagePublishBusy, setAgentPagePublishBusy] = useState(false);
  const [agentPageError, setAgentPageError] = useState("");
  const [agentSkillAddBusy, setAgentSkillAddBusy] = useState(false);
  const [agentSkillAddError, setAgentSkillAddError] = useState("");
  const [agentSkillDeleteBusy, setAgentSkillDeleteBusy] = useState(false);
  const [agentSkillDeleteError, setAgentSkillDeleteError] = useState("");
  const [agentPageNotice, setAgentPageNotice] = useState("");
  const [agentPageNoticeTone, setAgentPageNoticeTone] = useState<AgentPageNoticeTone>("warning");
  const agentPageNoticeTimerRef = useRef<number | null>(null);
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
  const agentPageHasUnsavedChanges = Boolean(
    agentPageDraft && agentPageSavedDraft && JSON.stringify(agentPageDraft) !== JSON.stringify(agentPageSavedDraft),
  );
  const agentPageNavigationBlocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      agentPageHasUnsavedChanges && currentLocation.pathname !== nextLocation.pathname,
  );
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
    const userID = resolveAgentChannelUserID(item);
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
  const runningAgentCount = agentItems.filter(isAgentRunning).length;
  const notifierWebhookPublicOrigin = useMemo(() => resolvedNotifierWebhookOrigin(bootstrapConfig), [bootstrapConfig]);
  const createParticipantAvatarSources = useMemo(
    () => [...agentItems, ...(data?.users ?? [])],
    [agentItems, data?.users],
  );
  const selectedAgentForPage = useMemo(() => {
    if (activePane.type !== WorkspacePaneTypes.agent) {
      return null;
    }
    return agentItems.find((item) => item.id === activePane.id) ?? null;
  }, [agentItems, activePane]);
  const selectedAgentForPageDraftSignature = useMemo(() => {
    if (!selectedAgentForPage) {
      return "";
    }
    return JSON.stringify({
      id: selectedAgentForPage.id || "",
      name: selectedAgentForPage.name || "",
      description: selectedAgentForPage.description || "",
      instructions: selectedAgentForPage.instructions || "",
      profile: profileSelectorFromDraft(agentToDraft(selectedAgentForPage)),
      profile_complete:
        selectedAgentForPage.profile_complete ?? selectedAgentForPage.agent_profile?.profile_complete ?? null,
      provider: selectedAgentForPage.provider || selectedAgentForPage.agent_profile?.provider || "",
      model_provider_id:
        selectedAgentForPage.model_provider_id || selectedAgentForPage.agent_profile?.model_provider_id || "",
      model_id: selectedAgentForPage.model_id || selectedAgentForPage.agent_profile?.model_id || "",
      reasoning_effort:
        selectedAgentForPage.reasoning_effort || selectedAgentForPage.agent_profile?.reasoning_effort || "",
      enable_fast_mode:
        selectedAgentForPage.enable_fast_mode ?? selectedAgentForPage.agent_profile?.enable_fast_mode ?? false,
    });
  }, [selectedAgentForPage]);
  const selectedFeishuPendingRegistration = useMemo(() => {
    const agentID = String(selectedAgentForPage?.id || "").trim();
    if (!agentID) {
      return null;
    }
    return normalizeFeishuPendingRegistration(feishuPendingRegistrations[agentID], agentID);
  }, [feishuPendingRegistrations, selectedAgentForPage?.id]);
  const skillsAgentID = selectedAgentForPage?.id || "";
  const globalSkillsQuery = useQuery({
    queryKey: workspaceQueryKeys.skills(),
    queryFn: async () => {
      const payload = await fetchSkills();
      return Array.isArray(payload) ? payload : [];
    },
  });
  const agentSkillsQuery = useQuery({
    queryKey: workspaceQueryKeys.agentSkills(skillsAgentID),
    queryFn: async () => {
      const skillsListing = await fetchAgentSkills(skillsAgentID);
      const skills = skillOptionsFromWorkspace(skillsListing.entries || []);
      return Promise.all(
        skills.map(async (skill) => {
          try {
            const file = await fetchAgentSkillsFile(skillsAgentID, `${skill.name}/SKILL.md`);
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
    enabled: Boolean(skillsAgentID),
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
  const activeConversation = useMemo(
    () => data?.rooms.find((item) => item.id === activeConversationId) ?? null,
    [data, activeConversationId],
  );

  const managerTemplateVariants = collectManagerTemplateVariants(hubTemplates);
  const managerRuntimeOptions = availableManagerRebuildRuntimeOptions(
    managerTemplateVariants,
    bootstrapConfig,
    managerAgent?.runtime_kind ?? undefined,
  );
  const agentsDisplayError =
    agentsError || (agentsQuery.isError ? errorMessage(agentsQuery.error, t("agentActionFailed")) : "");
  const teamsQuery = useQuery({
    queryKey: workspaceQueryKeys.teams(),
    queryFn: fetchTeams,
  });

  const agentModelOptions = useMemo(() => modelProviderOptionsFromCatalog(modelProviders), [modelProviders]);
  const agentPageModelOptions = useMemo(() => modelProviderOptionsFromCatalog(modelProviders), [modelProviders]);
  const agentModelBusy = Boolean(showAgentModal && !modelProvidersLoaded);
  const agentPageModelBusy = Boolean(activePane.type === WorkspacePaneTypes.agent && !modelProvidersLoaded);
  const agentPageModelError = "";
  const resetAgentModels = useCallback(() => {
    void refreshWorkspaceModelProviders();
  }, [refreshWorkspaceModelProviders]);
  const resetAgentPageModels = useCallback(() => {
    void refreshWorkspaceModelProviders();
  }, [refreshWorkspaceModelProviders]);
  const { cliproxyAuthStatuses, setCLIProxyAuthStatus } = useCLIProxyAuthStatuses(
    [
      managerProfile?.provider,
      isNotifierRuntimeDraft(agentDraft) ? "" : agentDraft?.provider,
      isNotifierRuntimeDraft(agentPageDraft) ? "" : agentPageDraft?.provider,
    ],
    t,
  );

  const progressBusy = agentBusy || agentActionBusy === `${MANAGER_AGENT_ID}:recreate`;

  const clearAgentPageNotice = useCallback(() => {
    if (agentPageNoticeTimerRef.current !== null) {
      window.clearTimeout(agentPageNoticeTimerRef.current);
      agentPageNoticeTimerRef.current = null;
    }
    setAgentPageNotice("");
    setAgentPageNoticeTone("warning");
  }, []);

  const showAgentPageNotice = useCallback((message: string, tone: AgentPageNoticeTone = "warning") => {
    if (agentPageNoticeTimerRef.current !== null) {
      window.clearTimeout(agentPageNoticeTimerRef.current);
    }
    setAgentPageNotice(message);
    setAgentPageNoticeTone(tone);
    agentPageNoticeTimerRef.current = window.setTimeout(() => {
      setAgentPageNotice("");
      setAgentPageNoticeTone("warning");
      agentPageNoticeTimerRef.current = null;
    }, 5000);
  }, []);

  useEffect(
    () => () => {
      if (agentPageNoticeTimerRef.current !== null) {
        window.clearTimeout(agentPageNoticeTimerRef.current);
      }
    },
    [],
  );

  function agentOperationUsesPageError(item: AgentLike | null | undefined): boolean {
    return Boolean(item?.id && activePane.type === WorkspacePaneTypes.agent && activePane.id === item.id);
  }

  function clearAgentOperationError(item: AgentLike | null | undefined): void {
    if (agentOperationUsesPageError(item)) {
      setAgentPageError("");
      return;
    }
    setAgentsError("");
  }

  function setAgentOperationError(item: AgentLike | null | undefined, message: string): void {
    if (agentOperationUsesPageError(item)) {
      setAgentPageError(message);
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
    if (!managerProfileIncomplete) {
      clearAgentPageNotice();
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
    showAgentPageNotice(t("profileIncompleteRedirectNotice"));
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
      return;
    }
    if (window.confirm(t("agentUnsavedChangesWarning"))) {
      agentPageNavigationBlocker.proceed();
    } else {
      agentPageNavigationBlocker.reset();
    }
  }, [agentPageNavigationBlocker, t]);

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
  }, [skillsAgentID]);

  useEffect(() => {
    if (!selectedAgentForPage) {
      setAgentPageDraft(null);
      setAgentPageSavedDraft(null);
      setAgentPageError("");
      setAgentPagePublishBusy(false);
      return;
    }
    if (agentPageHasUnsavedChanges) {
      return;
    }
    loadAgentPageDraft(selectedAgentForPage);
  }, [agentPageHasUnsavedChanges, selectedAgentForPage?.id, selectedAgentForPageDraftSignature]);

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

  function openManagerRebuildModal(item: AgentLike | null | undefined = managerAgent) {
    const initialRuntimeKind = normalizeRuntimeKind(
      item?.runtime_kind || managerAgent?.runtime_kind || bootstrapConfig?.runtime_kind || managerRebuildRuntimeKind,
    );
    const fallbackRuntimeKind = managerRuntimeOptions[0]?.value || DEFAULT_RUNTIME_KIND;
    const resolvedRuntimeKind = managerRuntimeOptions.some((option) => option.value === initialRuntimeKind)
      ? initialRuntimeKind
      : fallbackRuntimeKind;
    const currentImage = String(item?.image ?? managerAgent?.image ?? "").trim();
    const resolvedImage = defaultManagerRebuildImageForRuntime(
      managerTemplateVariants,
      resolvedRuntimeKind,
      bootstrapConfig,
      currentImage,
    );
    setManagerRebuildRuntimeKind(resolvedRuntimeKind);
    setManagerRebuildImage(resolvedImage);
    setManagerRebuildError("");
    setShowManagerRebuildModal(true);
  }

  function updateManagerRebuildRuntimeKind(runtimeKind: string): void {
    const nextRuntimeKind = normalizeRuntimeKind(runtimeKind);
    setManagerRebuildRuntimeKind(nextRuntimeKind);
    setManagerRebuildError("");
    setManagerRebuildImage(
      defaultManagerRebuildImageForRuntime(
        managerTemplateVariants,
        nextRuntimeKind,
        bootstrapConfig,
        managerAgent?.image || "",
      ),
    );
  }

  async function requestManagerRebuild(options: ManagerRebuildOptions = {}): Promise<void> {
    const runtimeKind = normalizeRuntimeKind(
      options.runtimeKind ||
        managerAgent?.runtime_kind ||
        bootstrapConfig?.runtime_kind ||
        managerRuntimeOptions[0]?.value,
    );
    const rebuiltAgent = await createManagerAgentRequest({
      runtime_kind: runtimeKind,
    });
    await refreshAgentsWithUpdatedAgent(rebuiltAgent);
    await syncAgentStateUntilRunning(MANAGER_AGENT_ID);
    await refreshManagerProfile();
    await refreshWorkspaceBootstrapConfig();
  }

  async function rebuildManagerFromBrowser(options: ManagerRebuildOptions = {}): Promise<boolean> {
    setAgentActionBusy(`${MANAGER_AGENT_ID}:recreate`);
    setManagerRebuildError("");
    const runtimeKind = normalizeRuntimeKind(
      options.runtimeKind ||
        managerAgent?.runtime_kind ||
        bootstrapConfig?.runtime_kind ||
        managerRuntimeOptions[0]?.value,
    );
    setAgentProgress(startAgentCreateProgress(runtimeKind));
    try {
      await requestManagerRebuild(options);
      setAgentProgress((current) =>
        current
          ? { ...current, percent: 100, status: "done", index: Math.max(0, (current.steps?.length || 1) - 1) }
          : current,
      );
      return true;
    } catch (err) {
      setAgentProgress((current) => (current ? { ...current, status: "failed" } : current));
      setManagerRebuildError(errorMessage(err, t("agentActionFailed")));
      return false;
    } finally {
      setAgentActionBusy("");
    }
  }

  async function confirmManagerRebuild(): Promise<void> {
    if (agentActionBusy) {
      return;
    }
    const selectedRuntimeKind = normalizeRuntimeKind(
      managerRebuildRuntimeKind || managerAgent?.runtime_kind || bootstrapConfig?.runtime_kind,
    );
    setMessageActionError({ key: "", message: "" });
    const rebuilt = await rebuildManagerFromBrowser({ runtimeKind: selectedRuntimeKind });
    if (rebuilt) {
      setShowManagerRebuildModal(false);
      setAgentProgress(null);
    }
  }

  async function handleMessageAction(action: MessageAction | null | undefined, _message?: MessageLike | null) {
    if (!action || action.id !== ACTION_REBUILD_MANAGER) {
      return;
    }
    if (messageActionBusy || agentActionBusy) {
      return;
    }
    setMessageActionError({ key: "", message: "" });
    openManagerRebuildModal(managerAgent);
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
    if (activePane.type === WorkspacePaneTypes.agent && activePane.id === agentID) {
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

  async function fetchAgentWithProfile(item: AgentLike | null | undefined): Promise<AgentWithProfile> {
    const id = String(item?.id ?? "").trim();
    if (!id) {
      return { agent: item || {}, profile: item?.agent_profile };
    }
    let agent: AgentLike = item || {};
    try {
      agent = { ...agent, ...(await fetchAgent(id)) };
    } catch (_) {
      // Keep the channel bot list item when the full agent endpoint is unavailable.
    }
    let profile = agent?.agent_profile;
    try {
      profile = await fetchAgentProfile(id);
    } catch (_) {
      // Keep the profile embedded in the full agent record or list item.
    }
    return { agent, profile };
  }

  async function agentDraftFromItem(item: AgentLike): Promise<AgentDraft> {
    if (isNotificationBotAgent(item)) {
      return ensureNotifierPullSubscriptionDraft(agentToDraft(item));
    }
    const { agent, profile } = await fetchAgentWithProfile(item);
    const base = agentToDraft({ ...agent, agent_profile: profile });
    const runtimeKind = normalizeRuntimeKind(agent?.runtime_kind || item?.runtime_kind || base.runtime_kind);
    return ensureNotifierPullSubscriptionDraft({
      ...base,
      runtime_kind: runtimeKind || base.runtime_kind,
      bot_type: BOT_TYPE_NORMAL,
    });
  }

  async function openCreateNotificationParticipantModal(): Promise<void> {
    setAgentModalMode("create");
    setAgentCreateBotKind(BOT_CREATE_KIND_NOTIFICATION);
    setEditingAgent(null);
    setAgentError("");
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
    setEditingAgent(null);
    setAgentError("");
    setAgentProgress(null);
    resetAgentModels();
    const runtimeChoices = Array.isArray(bootstrapConfig?.worker_runtime_choices)
      ? bootstrapConfig.worker_runtime_choices
      : [];
    const codexAvailable = runtimeChoices.some(
      (item) => !item?.sandbox_enabled && normalizeRuntimeName(item?.name) === "codex" && item?.installed !== false,
    );
    let preferredRuntimeKind =
      normalizeRuntimeKind(bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "") || DEFAULT_RUNTIME_KIND;
    if (preferredRuntimeKind === "codex" && !codexAvailable) {
      preferredRuntimeKind =
        normalizeRuntimeKind(
          composeLegacyRuntimeKind(
            normalizeRuntimeName(runtimeChoices.find((item) => item?.sandbox_enabled)?.name || "picoclaw"),
            true,
          ),
        ) || DEFAULT_RUNTIME_KIND;
    }
    const selectedTemplate =
      template === undefined
        ? pickDefaultAgentTemplate(hubTemplates, preferredRuntimeKind, bootstrapConfig)
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
          bootstrapConfig,
          managerAgent?.image || "",
        ),
        runtime_name: initialRuntime.runtime_name,
        sandbox_enabled: initialRuntime.sandbox_enabled,
        runtime_kind: initialRuntime.runtime_kind,
        bot_type: BOT_TYPE_NORMAL,
        agent_profile: defaults,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
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
          bootstrapConfig,
          managerAgent?.image || "",
        ),
        runtime_name: initialRuntime.runtime_name,
        sandbox_enabled: initialRuntime.sandbox_enabled,
        runtime_kind: initialRuntime.runtime_kind,
        bot_type: BOT_TYPE_NORMAL,
        agent_profile: managerProfile,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      draft = draftWithModelProviderFallback(draft, agentModelOptions);
      setAgentDraft(draft);
      setShowAgentModal(true);
    }
  }

  function openCreateTeamModal(): void {
    const firstAgentID = createTeamCandidateIDs[0] || "";
    setEditingTeam(null);
    setCreateTeamTitle("");
    setCreateTeamMemberIDs(firstAgentID ? [firstAgentID] : []);
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
    setCreateTeamMemberIDs([...(item.member_agent_ids ?? [])]);
    setTeamActionError("");
    setShowCreateTeamModal(true);
  }

  async function openEditAgentModal(item: AgentLike): Promise<void> {
    setAgentModalMode("edit");
    setAgentCreateBotKind(isNotificationBotAgent(item) ? BOT_CREATE_KIND_NOTIFICATION : BOT_CREATE_KIND_WORKER);
    setEditingAgent(item);
    setAgentError("");
    setAgentProgress(null);
    resetAgentModels();
    try {
      const draft = await agentDraftFromItem(item);
      setAgentDraft(draft);
      setShowAgentModal(true);
    } catch (err) {
      setAgentError(errorMessage(err, t("agentActionFailed")));
    }
  }

  async function loadAgentPageDraft(item: AgentLike | null | undefined): Promise<void> {
    if (!item?.id) {
      return;
    }
    setAgentPageError("");
    resetAgentPageModels();
    try {
      const draft = await agentDraftFromItem(item);
      setAgentPageDraft(draft);
      setAgentPageSavedDraft(draft);
    } catch (err) {
      setAgentPageError(errorMessage(err, t("agentActionFailed")));
      const draft = ensureNotifierPullSubscriptionDraft(agentToDraft(item));
      setAgentPageDraft(draft);
      setAgentPageSavedDraft(draft);
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
  ): boolean {
    return Boolean(saved?.id && saved.id !== MANAGER_AGENT_ID && profileChanged && !runtimeOptionsChanged);
  }

  async function saveAgentPage(): Promise<void> {
    const draftToSave = agentPageDraft;
    if (!draftToSave || !selectedAgentForPage?.id) {
      return;
    }
    setAgentPageBusy(true);
    setAgentPageError("");
    try {
      const draft = ensureNotifierPullSubscriptionDraft(draftToSave);
      if (isNotifierRuntimeDraftOnAgentPage(draftToSave, selectedAgentForPage)) {
        if (!notifierFormIsComplete(draftToSave, selectedAgentForPage)) {
          setAgentPageError(t("profileSaveIncompleteError"));
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
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        await refreshAgentSkills(saved.id || selectedAgentForPage.id);
        const savedDraft = agentToDraft(saved);
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
      const profileChanged = profilePayloadForCompare(draftToSave) !== profilePayloadForCompare(agentPageSavedDraft);
      const runtimeOptionsChanged =
        runtimeOptionsPayloadForCompare(draftToSave) !== runtimeOptionsPayloadForCompare(agentPageSavedDraft);
      const hasProfileOrRuntimeChange = profileChanged || (runtimeOptionsChanged && hasObjectValues(runtimeOptions));

      const payload = agentPageBaseUpdatePayload(draftToSave);
      if (profileChanged) {
        payload.agent_profile = profile;
        payload.profile = profileSelectorFromDraft(draft);
      }
      if (runtimeOptionsChanged) {
        payload.runtime_options = runtimeOptions || {};
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
        const nextDraft = await agentDraftFromItem(savedMetaOnly);
        setAgentPageDraft(nextDraft);
        setAgentPageSavedDraft(nextDraft);
        return;
      }
      const llmProfileChanged = agentPageLLMProfileChanged(draftToSave, agentPageSavedDraft);
      if (llmProfileChanged && !isAgentProfileDraftComplete(draftToSave)) {
        setAgentPageError(t("profileSaveIncompleteError"));
        return;
      }
      debugAgentPageSavePayload("full", payload);
      const managerBeforeSave = selectedAgentForPage;
      const profileIncompleteBeforeSave = !isAgentProfileMarkedComplete(agentPageSavedDraft);
      const saved = await updateAgentRequest(selectedAgentForPage.id, payload);
      await saveLinkedAgentUserAvatar(selectedAgentForPage, draft.avatar);
      if (canApplyAgentPageProfileSaveImmediately(saved, profileChanged, runtimeOptionsChanged)) {
        applyAgentListUpdate(saved);
        const savedDraft = agentToDraft(saved);
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
      const savedDraft = await agentDraftFromItem(saved);
      setAgentPageDraft(savedDraft);
      setAgentPageSavedDraft(savedDraft);
      if (
        profileChanged &&
        saved.id === MANAGER_AGENT_ID &&
        !isAgentProfileMarkedComplete(saved) &&
        !isAgentProfileMarkedComplete(savedDraft)
      ) {
        setAgentPageError(t("profileSaveIncompleteError"));
        showAgentPageNotice(t("profileSetupIncompleteAfterSave"));
      }
    } catch (err) {
      setAgentPageError(errorMessage(err, t("agentActionFailed")));
    } finally {
      setAgentPageBusy(false);
    }
  }

  async function publishAgentPage(): Promise<void> {
    if (!selectedAgentForPage?.id || agentPagePublishBusy) {
      return;
    }
    setAgentPagePublishBusy(true);
    setAgentPageError("");
    try {
      const published = await publishAgentTemplateRequest(selectedAgentForPage.id);
      await refreshHubTemplates();
      if (published?.id) {
        setSelectedHubTemplateId(published.id);
      }
      selectHub();
    } catch (err) {
      setAgentPageError(errorMessage(err, t("agentActionFailed")));
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
      if (isCreate) {
        if (runtimeOptions) {
          payload.runtime_options = runtimeOptions;
        }
      } else if (runtimeOptionsChanged) {
        payload.runtime_options = runtimeOptions || {};
      }
      const saved = isCreate
        ? await createBotRequest(payload)
        : await updateAgentRequest(editingAgentID, {
            name: payload.name,
            description: payload.description,
            instructions: payload.instructions,
            agent_profile: payload.agent_profile,
            profile: payload.profile,
            ...(payload.runtime_options !== undefined ? { runtime_options: payload.runtime_options } : {}),
          });
      await saveLinkedAgentUserAvatar(saved?.participants?.length ? saved : editingAgent || saved, agentDraft.avatar);
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
    } finally {
      setAgentBusy(false);
    }
  }

  async function runAgentAction(item: AgentLike | null | undefined, action: AgentAction): Promise<void> {
    if (!item?.id || agentActionBusy) {
      return;
    }
    if (
      isNotificationBotAgent(item) &&
      (action === "recreate" || action === "start" || action === "stop" || action === "upgrade")
    ) {
      return;
    }
    if (action === "recreate" && isManagerAgent(item)) {
      openManagerRebuildModal(item);
      return;
    }
    if (action === "delete" && !window.confirm(agentDeleteConfirmationMessage(item, t))) {
      return;
    }
    setAgentActionBusy(`${item.id}:${action}`);
    clearAgentOperationError(item);
    try {
      let updatedAgent: AgentLike | null = null;
      if (action === "delete") {
        await deleteAgentRequest(item.id);
      } else {
        updatedAgent = await runAgentActionRequest(item.id, action);
      }
      await refreshAgentsWithUpdatedAgent(updatedAgent);
      if (action === "delete") {
        await refreshAgentSkills(item.id);
      }
      if (item.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
        if (action === "recreate" || action === "start") {
          await syncAgentStateUntilRunning(MANAGER_AGENT_ID);
        }
      }
    } catch (err) {
      setAgentOperationError(item, errorMessage(err, t("agentActionFailed")));
    } finally {
      setAgentActionBusy("");
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
      if (!agentID || !registrationID || feishuAutoFinalizeActiveRef.current.has(registrationID)) {
        return;
      }
      const background = Boolean(options.background);
      const busyKey = feishuActionKey(agentID, "finalize");
      feishuAutoFinalizeActiveRef.current.add(registrationID);
      if (!background) {
        setAgentActionBusy(busyKey);
        setAgentPageError("");
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
            showAgentPageNotice(t("feishuConnectPending"));
          }
          return;
        }
        updateFeishuPendingRegistrations((current) => {
          const next = { ...current };
          delete next[agentID];
          return next;
        });
        await refreshAgentStateRef.current(agentID);
        showAgentPageNotice(t("feishuConnectConfigured"), "success");
      } catch (err) {
        if (feishuRegistrationFinalizeClearsPending(err)) {
          updateFeishuPendingRegistrations((current) => {
            const next = { ...current };
            delete next[agentID];
            return next;
          });
        }
        if (!background) {
          setAgentPageError(errorMessage(err, t("feishuConnectFailed")));
        }
      } finally {
        feishuAutoFinalizeActiveRef.current.delete(registrationID);
        if (!background) {
          setAgentActionBusy((current) => (current === busyKey ? "" : current));
        }
      }
    },
    [showAgentPageNotice, t, updateFeishuPendingRegistrations],
  );

  useEffect(() => {
    const timers: number[] = [];
    Object.entries(feishuPendingRegistrations).forEach(([agentID, registration]) => {
      const pending = normalizeFeishuPendingRegistration(registration, agentID);
      if (!pending || agentActionBusy || feishuAutoFinalizeActiveRef.current.has(pending.registration_id)) {
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
  }, [agentActionBusy, completeFeishuPendingRegistration, feishuPendingRegistrations]);

  const finalizeVisibleFeishuPendingRegistrations = useCallback(() => {
    if (agentActionBusy) {
      return;
    }
    Object.entries(feishuPendingRegistrations).forEach(([agentID, registration]) => {
      const pending = normalizeFeishuPendingRegistration(registration, agentID);
      if (!pending || feishuAutoFinalizeActiveRef.current.has(pending.registration_id)) {
        return;
      }
      void completeFeishuPendingRegistration(pending, { background: true });
    });
  }, [agentActionBusy, completeFeishuPendingRegistration, feishuPendingRegistrations]);

  useEffect(() => {
    window.addEventListener("focus", finalizeVisibleFeishuPendingRegistrations);
    return () => {
      window.removeEventListener("focus", finalizeVisibleFeishuPendingRegistrations);
    };
  }, [finalizeVisibleFeishuPendingRegistrations]);

  async function startFeishuConnect(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    if (!agentID || agentActionBusy) {
      return;
    }
    setAgentActionBusy(feishuActionKey(agentID, "connect"));
    setAgentPageError("");
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
      showAgentPageNotice(t("feishuConnectStarted"), "info");
    } catch (err) {
      setAgentPageError(errorMessage(err, t("feishuConnectFailed")));
    } finally {
      setAgentActionBusy("");
    }
  }

  async function finalizeFeishuConnect(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    const pending = normalizeFeishuPendingRegistration(feishuPendingRegistrations[agentID], agentID);
    if (!agentID || !pending || agentActionBusy) {
      return;
    }
    await completeFeishuPendingRegistration(pending, { showPendingNotice: true });
  }

  async function disconnectFeishu(item: AgentLike | null | undefined): Promise<void> {
    const agentID = String(item?.id || "").trim();
    const participantID = String(feishuAgentParticipant(item)?.id || "").trim();
    if (!agentID || !participantID || agentActionBusy) {
      return;
    }
    const busyKey = feishuActionKey(agentID, "disconnect");
    setAgentActionBusy(busyKey);
    setAgentPageError("");
    try {
      await deleteFeishuParticipantRequest(participantID);
      updateFeishuPendingRegistrations((current) => {
        const next = { ...current };
        delete next[agentID];
        return next;
      });
      await refreshAgentStateRef.current(agentID);
      showAgentPageNotice(t("feishuDisconnectConfigured"), "success");
    } catch (err) {
      setAgentPageError(errorMessage(err, t("feishuDisconnectFailed")));
    } finally {
      setAgentActionBusy((current) => (current === busyKey ? "" : current));
    }
  }

  async function deletePreviewBot(item: AgentLike | null | undefined) {
    if (!item?.id || agentActionBusy) {
      return false;
    }
    if (!window.confirm(`${t("agentDelete")} ${item.name}?`)) {
      return false;
    }
    setAgentActionBusy(`${item.id}:delete-bot`);
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
      setAgentActionBusy("");
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
        member_agent_ids: createTeamMemberIDs,
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
    const teamTitle = item ? displayTeam(item) : teamID;
    if (!window.confirm(t("teamDeleteConfirm", { title: teamTitle }))) {
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
      if (!skillsAgentID || agentSkillAddBusy) {
        return false;
      }
      const names = skillNames.map((name) => String(name || "").trim()).filter(Boolean);
      if (!names.length) {
        return false;
      }
      setAgentSkillAddBusy(true);
      setAgentSkillAddError("");
      try {
        await batchAddAgentSkillsRequest(skillsAgentID, names);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(skillsAgentID) });
        return true;
      } catch (err) {
        setAgentSkillAddError(errorMessage(err, t("agentSkillAddFailed")));
        return false;
      } finally {
        setAgentSkillAddBusy(false);
      }
    },
    [agentSkillAddBusy, queryClient, skillsAgentID, t],
  );

  const deleteAgentSkill = useCallback(
    async (skill: { name?: string | null } | string | null | undefined) => {
      if (!skillsAgentID || agentSkillDeleteBusy) {
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
        await deleteAgentSkillRequest(skillsAgentID, name);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(skillsAgentID) });
        return true;
      } catch (err) {
        setAgentSkillDeleteError(errorMessage(err, t("agentSkillDeleteFailed")));
        return false;
      } finally {
        setAgentSkillDeleteBusy(false);
      }
    },
    [agentSkillDeleteBusy, queryClient, skillsAgentID, t],
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

  return {
    agentActionBusy,
    agentItems,
    agentsDisplayError,
    cliproxyAuthBusy,
    cliproxyAuthStatuses,
    deletePreviewBot,
    handleMessageAction,
    loginCLIProxyProvider,
    managerAgent,
    managerProfileIncomplete,
    messageActionBusy,
    messageActionError,
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
      busyKey: agentActionBusy,
      error: "",
      draft: agentPageDraft,
      savedDraft: agentPageSavedDraft,
      hasUnsavedChanges: agentPageHasUnsavedChanges,
      models: agentPageModelOptions.map((option) => option.modelID),
      modelOptions: agentPageModelOptions,
      modelProviders,
      modelBusy: agentPageModelBusy,
      modelError: agentPageModelError,
      saving: agentPageBusy,
      publishBusy: agentPagePublishBusy,
      saveError: agentPageError,
      notice: agentPageNotice,
      noticeTone: agentPageNoticeTone,
      feishuConnectBusy: agentActionBusy.includes(`:${FEISHU_CHANNEL_ACTION}:`) ? agentActionBusy : "",
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
      skills: agentSkillsQuery.data ?? [],
      skillsLoading: agentSkillsQuery.isFetching,
      skillsError: agentSkillsError,
      workspaceSupported: Boolean(selectedAgentForPage),
      onDraftChange: setAgentPageDraft,
      onSave: saveAgentPage,
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
      onAddSkills: batchAddAgentSkills,
      onDeleteSkill: deleteAgentSkill,
      teamActionBusy,
      teamActionError,
      onCreateTeam: createAgentTeam,
    },
    computerViewProps: {
      t,
      agents: agentItems,
      activeAgentID: activePane.type === WorkspacePaneTypes.agent ? activePane.id : "",
      busyKey: agentActionBusy,
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
            onAgentCreateBotKindChange: setAgentCreateBotKind,
            editingAgent,
            agentDraft,
            onAgentDraftChange: setAgentDraft,
            onAgentModelsReset: resetAgentModels,
            hubTemplates,
            bootstrapConfig,
            managerAgent,
            agentModels: agentModelOptions.map((option) => option.modelID),
            agentModelOptions,
            modelProviders,
            agentModelBusy,
            authStatuses: cliproxyAuthStatuses,
            authBusyProvider: cliproxyAuthBusy,
            notifierWebhookPublicOrigin,
            onProviderLogin: loginCLIProxyProvider,
            agentError,
            agentProgress,
            agentBusy,
            onClose: () => setShowAgentModal(false),
            onSave: saveAgent,
          }
        : null,
    managerRebuildModalProps: showManagerRebuildModal
      ? {
          t,
          runtimeOptions: managerRuntimeOptions,
          runtimeKind: managerRebuildRuntimeKind,
          image: managerRebuildImage,
          busy: agentActionBusy === `${MANAGER_AGENT_ID}:recreate`,
          error: managerRebuildError,
          progress: agentProgress,
          onRuntimeKindChange: updateManagerRebuildRuntimeKind,
          onClose: () => {
            setShowManagerRebuildModal(false);
            setManagerRebuildError("");
            setAgentProgress(null);
          },
          onConfirm: confirmManagerRebuild,
        }
      : null,
    createTeamModalProps: showCreateTeamModal
      ? {
          t,
          mode: editingTeam ? ("edit" as const) : ("create" as const),
          candidates: createTeamCandidates,
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

function csgclawParticipantIDForAgent(item: AgentLike): string {
  const participant = item.participants?.find(
    (candidate) => String(candidate?.channel || "").trim() === "csgclaw" && String(candidate?.id || "").trim(),
  );
  return String(participant?.id || item.id || "").trim();
}

function agentDeleteConfirmationMessage(item: AgentLike, t: TranslateFn): string {
  const name = String(item.name || item.id || "").trim();
  const message = t("agentDeleteConfirmMessage", { name });
  const channels = agentDeleteBoundChannels(item);
  if (channels.length === 0) {
    return message;
  }
  return [
    message,
    "",
    t("agentDeleteBoundChannels", { channels: channels.join(", ") }),
    "",
    t("agentDeleteCascadeNote"),
  ].join("\n");
}

function agentDeleteBoundChannels(item: AgentLike): string[] {
  const agentID = String(item.id || "").trim();
  const channels = new Set<string>();
  for (const participant of item.participants || []) {
    const participantID = String(participant?.id || "").trim();
    if (!participantID) {
      continue;
    }
    const participantAgentID = String(participant?.agent_id || "").trim();
    if (participantAgentID && agentID && participantAgentID !== agentID) {
      continue;
    }
    const channel = String(participant?.channel || "")
      .trim()
      .toLowerCase();
    if (!channel || channel === "csgclaw") {
      continue;
    }
    channels.add(agentDeleteChannelLabel(channel));
  }
  return Array.from(channels).sort((left, right) => left.localeCompare(right));
}

function agentDeleteChannelLabel(channel: string): string {
  if (channel === "feishu") {
    return "Feishu";
  }
  return channel.replace(/(^|[-_\s]+)(\w)/g, (_, separator: string, value: string) => {
    return `${separator ? " " : ""}${value.toUpperCase()}`;
  });
}
