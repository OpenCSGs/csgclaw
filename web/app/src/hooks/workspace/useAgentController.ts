import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker } from "react-router-dom";
import { errorMessage } from "@/api/client";
import { loginCLIProxyProviderRequest } from "@/api/cliproxy";
import {
  createBotRequest,
  createManagerAgentRequest,
  createNotificationBotRequest,
  deleteBotRequest,
  fetchAgent,
  fetchAgentProfile,
  fetchAgentProfileDefaults,
  patchNotificationBotRequest,
  runAgentActionRequest,
  updateAgentRequest,
} from "@/api/agents";
import type { AgentUpdatePayload, FetchAgentsOptions } from "@/api/agents";
import { publishAgentTemplateRequest } from "@/api/hub";
import { createUserRequest, inviteRoomUsersRequest, joinAgentToRoomRequest } from "@/api/im";
import { createTeamRequest, fetchTeams } from "@/api/tasks";
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
import { firstWorkspaceFilePath, hasWorkspaceFilePath } from "@/models/workspace";
import {
  applyTemplateToDraft,
  advanceAgentProgress,
  agentDraftWithRuntimeFieldsFromAgent,
  agentRuntimePollSettled,
  agentToDraft,
  availableManagerRebuildRuntimeOptions,
  collectManagerTemplateVariants,
  defaultManagerRebuildImageForRuntime,
  draftNotifierRuntimeOptionsForSave,
  draftToProfile,
  ensureNotifierPullSubscriptionDraft,
  isAgentRunning,
  isManagerAgent,
  isNotificationBotAgent,
  isNotificationBotDraftContext,
  isNotifierRuntimeDraft,
  isNotifierRuntimeDraftOnAgentPage,
  mergeAgentIntoList,
  normalizeAuthProviderName,
  partitionWorkspaceAgentItems,
  resolveAgentAvatarSource,
  normalizeRuntimeKind,
  normalizeTemplateSelection,
  pickDefaultAgentTemplate,
  providerNeedsAuth,
  resolvedNotifierWebhookOrigin,
  resolveAgentChannelUserID,
  runtimeImageForKind,
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
import { isDirectConversation, resolveRoomInviterID } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";
import { useCLIProxyAuthStatuses } from "./useCLIProxyAuthStatuses";
import { useProfileModelOptions } from "./useProfileModelOptions";
import {
  useWorkspaceAgentWorkspaceFileQuery,
  useWorkspaceAgentWorkspaceQuery,
  workspaceQueryKeys,
} from "./workspaceQueries";
import type { MessageAction, MessageActionError, MessageLike } from "@/components/business/MessageContent/types";
import type { IMConversation } from "@/models/conversations";
import type { UseAgentControllerArgs } from "./types";

type ManagerRebuildOptions = {
  runtimeKind?: RuntimeKind;
};

type AgentModalMode = "create" | "edit";
type AgentAction = "delete" | "recreate" | "start" | "stop" | "upgrade";

type AgentWithProfile = {
  agent: AgentLike;
  profile?: AgentProfileLike | null;
};

const AGENT_RUNTIME_SYNC_INTERVAL_MS = 2_000;
const AGENT_RUNTIME_SYNC_TIMEOUT_MS = 120_000;

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
  refreshHubTemplates,
  refreshWorkspaceAgents,
  refreshWorkspaceBootstrap,
  refreshWorkspaceBootstrapConfig,
  refreshWorkspaceManagerProfile,
  rooms,
  selectAgent,
  selectComputer,
  selectConversation,
  selectHub,
  setAgentsData,
  setSelectedHubTemplateId,
  t,
}: UseAgentControllerArgs) {
  const queryClient = useQueryClient();
  const [cliproxyAuthBusy, setCLIProxyAuthBusy] = useState("");
  const [agentsError, setAgentsError] = useState("");
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [showManagerRebuildModal, setShowManagerRebuildModal] = useState(false);
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
  const [agentPageNotice, setAgentPageNotice] = useState("");
  const agentPageNoticeTimerRef = useRef<number | null>(null);
  const [teamActionBusy, setTeamActionBusy] = useState(false);
  const [teamActionError, setTeamActionError] = useState("");
  const [showCreateTeamModal, setShowCreateTeamModal] = useState(false);
  const [createTeamTitle, setCreateTeamTitle] = useState("");
  const [createTeamMemberIDs, setCreateTeamMemberIDs] = useState<string[]>([]);
  const [selectedAgentWorkspacePath, setSelectedAgentWorkspacePath] = useState("");
  const agentPageHasUnsavedChanges = Boolean(
    agentPageDraft && agentPageSavedDraft && JSON.stringify(agentPageDraft) !== JSON.stringify(agentPageSavedDraft),
  );
  const agentPageNavigationBlocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      agentPageHasUnsavedChanges && currentLocation.pathname !== nextLocation.pathname,
  );
  const managerProfileIncomplete = managerProfile && managerProfile.profile_complete === false;
  const usersById = useMemo(() => {
    const result = new Map<
      string,
      { avatar?: string | null; handle?: string | null; id: string; name?: string | null }
    >();
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
  const selectedAgentForPage = useMemo(() => {
    if (activePane.type !== WorkspacePaneTypes.agent) {
      return null;
    }
    return agentItems.find((item) => item.id === activePane.id) ?? null;
  }, [agentItems, activePane]);
  const selectedAgentWorkspaceSupported = useMemo(() => {
    const runtimeKind = normalizeRuntimeKind(selectedAgentForPage?.runtime_kind);
    return runtimeKind === "openclaw_sandbox" || runtimeKind === "picoclaw_sandbox";
  }, [selectedAgentForPage?.runtime_kind]);
  const workspaceAgentID = selectedAgentWorkspaceSupported ? selectedAgentForPage?.id || "" : "";
  const agentWorkspaceQuery = useWorkspaceAgentWorkspaceQuery(workspaceAgentID);
  const agentWorkspaceEntries = agentWorkspaceQuery.data?.entries ?? null;
  const agentWorkspaceError = agentWorkspaceQuery.error
    ? errorMessage(agentWorkspaceQuery.error, t("agentWorkspaceLoadFailed"))
    : "";
  const selectedAgentWorkspaceFilePath = hasWorkspaceFilePath(agentWorkspaceEntries, selectedAgentWorkspacePath)
    ? selectedAgentWorkspacePath
    : "";
  const agentWorkspaceFileQuery = useWorkspaceAgentWorkspaceFileQuery(workspaceAgentID, selectedAgentWorkspaceFilePath);
  const agentWorkspaceFileError = agentWorkspaceFileQuery.error
    ? errorMessage(agentWorkspaceFileQuery.error, t("agentWorkspaceFileLoadFailed"))
    : "";
  useEffect(() => {
    const entries = agentWorkspaceEntries ?? [];
    if (!selectedAgentWorkspaceSupported || !selectedAgentForPage?.id || !entries.length) {
      if (selectedAgentWorkspacePath) {
        setSelectedAgentWorkspacePath("");
      }
      return;
    }
    if (!hasWorkspaceFilePath(entries, selectedAgentWorkspacePath)) {
      const nextPath = firstWorkspaceFilePath(entries);
      if (nextPath !== selectedAgentWorkspacePath) {
        setSelectedAgentWorkspacePath(nextPath);
      }
    }
  }, [agentWorkspaceEntries, selectedAgentForPage?.id, selectedAgentWorkspacePath, selectedAgentWorkspaceSupported]);
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
    queryKey: ["workspace", "teams"],
    queryFn: fetchTeams,
  });

  const {
    models: agentModels,
    modelBusy: agentModelBusy,
    resetModels: resetAgentModels,
  } = useProfileModelOptions({
    draft: agentDraft,
    enabled: Boolean(showAgentModal),
    onDraftChange: setAgentDraft,
  });
  const {
    models: agentPageModels,
    modelBusy: agentPageModelBusy,
    resetModels: resetAgentPageModels,
  } = useProfileModelOptions({
    draft: agentPageDraft,
    enabled: activePane.type === WorkspacePaneTypes.agent,
    onDraftChange: setAgentPageDraft,
  });
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
  }, []);

  const showAgentPageNotice = useCallback((message: string) => {
    if (agentPageNoticeTimerRef.current !== null) {
      window.clearTimeout(agentPageNoticeTimerRef.current);
    }
    setAgentPageNotice(message);
    agentPageNoticeTimerRef.current = window.setTimeout(() => {
      setAgentPageNotice("");
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
    if (!agents.some((item) => item.id === activePane.id)) {
      if (activeConversationId) {
        selectConversation(activeConversationId, { replace: true });
      } else {
        selectComputer({ replace: true });
      }
    }
  }, [agents, agentsLoaded, activePane, activeConversationId, selectComputer, selectConversation]);

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
    if (!selectedAgentForPage) {
      setAgentPageDraft(null);
      setAgentPageSavedDraft(null);
      setAgentPageError("");
      setAgentPagePublishBusy(false);
      return;
    }
    loadAgentPageDraft(selectedAgentForPage);
    // Reload only when the routed agent changes; form draft edits should not refetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedAgentForPage?.id]);

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
    setShowManagerRebuildModal(true);
  }

  function updateManagerRebuildRuntimeKind(runtimeKind: string): void {
    const nextRuntimeKind = normalizeRuntimeKind(runtimeKind);
    setManagerRebuildRuntimeKind(nextRuntimeKind);
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
    setAgentsError("");
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
      setAgentsError(errorMessage(err, t("agentActionFailed")));
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
      setAgentPageDraft((current) =>
        agentDraftWithRuntimeFieldsFromAgent(current ?? agentToDraft(agent), agent),
      );
      setAgentPageSavedDraft((current) =>
        agentDraftWithRuntimeFieldsFromAgent(current ?? agentToDraft(agent), agent),
      );
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
      await refreshAgentWorkspace(latestAgent.id);
    }
  }

  async function refreshAgentWorkspace(agentID: string | null | undefined): Promise<void> {
    const id = String(agentID ?? "").trim();
    if (!id) {
      return;
    }
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentWorkspaceScope(id) }),
      queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentWorkspaceFileScope(id) }),
    ]);
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
        avatar: "",
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
    const preferredRuntimeKind =
      normalizeRuntimeKind(bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "") || DEFAULT_RUNTIME_KIND;
    const selectedTemplate =
      template === undefined
        ? pickDefaultAgentTemplate(hubTemplates, preferredRuntimeKind, bootstrapConfig)
        : normalizeTemplateSelection(template);
    try {
      const defaults = await fetchAgentProfileDefaults();
      const runtimeKind =
        normalizeRuntimeKind(
          selectedTemplate?.runtime_kind || bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "",
        ) || DEFAULT_RUNTIME_KIND;
      let draft = agentToDraft({
        avatar: "",
        image: runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""),
        runtime_kind: runtimeKind,
        bot_type: BOT_TYPE_NORMAL,
        agent_profile: defaults,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      setAgentDraft(draft);
      setShowAgentModal(true);
    } catch (_) {
      const runtimeKind =
        normalizeRuntimeKind(
          selectedTemplate?.runtime_kind || bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "",
        ) || DEFAULT_RUNTIME_KIND;
      let draft = agentToDraft({
        avatar: "",
        image: runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""),
        runtime_kind: runtimeKind,
        bot_type: BOT_TYPE_NORMAL,
        agent_profile: managerProfile,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      setAgentDraft(draft);
      setShowAgentModal(true);
    }
  }

  function openCreateTeamModal(): void {
    const firstAgentID = createTeamCandidateIDs[0] || "";
    setCreateTeamTitle("");
    setCreateTeamMemberIDs(firstAgentID ? [firstAgentID] : []);
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
      draftToProfile(normalized, {
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
    const runtimeOptions = draftNotifierRuntimeOptionsForSave(normalized, {
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

  async function saveAgentPage(draftOverride?: AgentDraft): Promise<void> {
    const draftToSave = draftOverride ?? agentPageDraft;
    if (!draftToSave || !selectedAgentForPage?.id) {
      return;
    }
    setAgentPageBusy(true);
    setAgentPageError("");
    try {
      const draft = ensureNotifierPullSubscriptionDraft(draftToSave);
      if (isNotifierRuntimeDraftOnAgentPage(draftToSave, selectedAgentForPage)) {
        const runtimeOptions = draftNotifierRuntimeOptionsForSave(draft, { mergeNotifier: true });
        const payload: AgentUpdatePayload = {
          name: draftToSave.name,
          avatar: draftToSave.avatar,
          description: draftToSave.description,
        };
        if (runtimeOptions) {
          payload.runtime_options = runtimeOptions;
        }
        const saved = await patchNotificationBotRequest(selectedAgentForPage.id, payload);
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        await refreshAgentWorkspace(saved.id || selectedAgentForPage.id);
        const savedDraft = agentToDraft(saved);
        setAgentPageDraft(savedDraft);
        setAgentPageSavedDraft(savedDraft);
        return;
      }
      const profile = draftToProfile(draft, {
        name: draftToSave.name,
        description: draftToSave.description,
      });
      const runtimeOptions = draftNotifierRuntimeOptionsForSave(draft, {
        mergeNotifier: false,
      });
      const profileChanged = profilePayloadForCompare(draftToSave) !== profilePayloadForCompare(agentPageSavedDraft);
      const runtimeOptionsChanged =
        runtimeOptionsPayloadForCompare(draftToSave) !== runtimeOptionsPayloadForCompare(agentPageSavedDraft);
      const hasProfileOrRuntimeChange = profileChanged || (runtimeOptionsChanged && hasObjectValues(runtimeOptions));

      const payload: AgentUpdatePayload = {
        name: draftToSave.name,
        avatar: draftToSave.avatar,
        description: draftToSave.description,
      };
      if (profileChanged) {
        payload.agent_profile = profile;
      }
      if (runtimeOptionsChanged && hasObjectValues(runtimeOptions)) {
        payload.runtime_options = runtimeOptions;
      }
      if (!hasProfileOrRuntimeChange) {
        debugAgentPageSavePayload("meta-only", payload);
        const savedMetaOnly = await updateAgentRequest(selectedAgentForPage.id, payload);
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        if (savedMetaOnly.id === MANAGER_AGENT_ID) {
          await refreshManagerProfile();
        }
        await refreshAgentWorkspace(savedMetaOnly.id || selectedAgentForPage.id);
        const nextDraft = await agentDraftFromItem(savedMetaOnly);
        setAgentPageDraft(nextDraft);
        setAgentPageSavedDraft(nextDraft);
        return;
      }
      debugAgentPageSavePayload("full", payload);
      const managerBeforeSave = selectedAgentForPage;
      const saved = await updateAgentRequest(selectedAgentForPage.id, payload);
      await refreshAgentsWithUpdatedAgent(saved);
      if (saved.id === MANAGER_AGENT_ID && profileChanged) {
        await syncManagerRuntimeAfterProfileSave(managerBeforeSave);
      }
      await refreshWorkspaceBootstrap();
      if (saved.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
      }
      await refreshAgentWorkspace(saved.id || selectedAgentForPage.id);
      const savedDraft = await agentDraftFromItem(saved);
      setAgentPageDraft(savedDraft);
      setAgentPageSavedDraft(savedDraft);
    } catch (err) {
      setAgentPageError(errorMessage(err, t("agentActionFailed")));
    } finally {
      setAgentPageBusy(false);
    }
  }

  async function saveAgentPageAvatar(avatar: string): Promise<void> {
    if (!agentPageDraft) {
      return;
    }
    const nextDraft = { ...agentPageDraft, avatar };
    setAgentPageDraft(nextDraft);
    await saveAgentPage(nextDraft);
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
    const runtimeKind = normalizeRuntimeKind(agentDraft.runtime_kind) || DEFAULT_RUNTIME_KIND;
    setAgentProgress(isCreate ? startAgentCreateProgress(isNotification ? BOT_TYPE_NOTIFICATION : runtimeKind) : null);
    try {
      const draft = ensureNotifierPullSubscriptionDraft(agentDraft);
      if (isNotification) {
        const runtimeOptions = draftNotifierRuntimeOptionsForSave(draft, { mergeNotifier: true });
        const payload: AgentUpdatePayload = {
          name: agentDraft.name,
          avatar: agentDraft.avatar,
          description: agentDraft.description,
        };
        if (runtimeOptions) {
          payload.runtime_options = runtimeOptions;
        }
        const saved = await (isCreate
          ? createNotificationBotRequest(payload)
          : patchNotificationBotRequest(editingAgentID, payload));
        await refreshAgents();
        await refreshWorkspaceBootstrap();
        if (!isCreate) {
          await refreshAgentWorkspace(editingAgentID);
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
      const runtimeOptions = draftNotifierRuntimeOptionsForSave(draft, {
        mergeNotifier: false,
      });
      const payload: AgentUpdatePayload = {
        name: agentDraft.name,
        avatar: agentDraft.avatar,
        role: WORKER_AGENT_ROLE,
        description: agentDraft.description,
        image: agentDraft.image,
        runtime_kind: runtimeKind,
        from_template: agentDraft.from_template || "",
        agent_profile: profile,
      };
      if (runtimeOptions) {
        payload.runtime_options = runtimeOptions;
      }
      const saved = isCreate
        ? await createBotRequest(payload)
        : await updateAgentRequest(editingAgentID, {
            name: payload.name,
            avatar: payload.avatar,
            description: payload.description,
            agent_profile: payload.agent_profile,
            ...(payload.runtime_options ? { runtime_options: payload.runtime_options } : {}),
          });
      await refreshAgents();
      await refreshWorkspaceBootstrap();
      if (saved.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
      }
      await refreshAgentWorkspace(saved.id || editingAgentID);
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
    if (action === "delete" && !window.confirm(`${t("agentDelete")} ${item.name}?`)) {
      return;
    }
    setAgentActionBusy(`${item.id}:${action}`);
    setAgentsError("");
    try {
      let updatedAgent: AgentLike | null = null;
      if (action === "delete") {
        await deleteBotRequest(csgclawParticipantIDForAgent(item), { deleteAgent: true });
      } else {
        updatedAgent = await runAgentActionRequest(item.id, action);
      }
      await refreshAgentsWithUpdatedAgent(updatedAgent);
      if (action === "delete") {
        await refreshAgentWorkspace(item.id);
      }
      if (item.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
        if (action === "recreate" || action === "start") {
          await syncAgentStateUntilRunning(MANAGER_AGENT_ID);
        }
      }
    } catch (err) {
      setAgentsError(errorMessage(err, t("agentActionFailed")));
    } finally {
      setAgentActionBusy("");
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
    setAgentsError("");
    try {
      await deleteBotRequest(csgclawParticipantIDForAgent(item));
      await refreshAgents();
      await refreshWorkspaceBootstrap();
      if (item.id === MANAGER_AGENT_ID) {
        await refreshManagerProfile();
      }
      return true;
    } catch (err) {
      setAgentsError(errorMessage(err, t("agentActionFailed")));
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
      setAgentsError("");
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
        setAgentsError(errorMessage(err, t("agentActionFailed")));
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
      channel: "csgclaw",
      title: createTeamTitle.trim() || t("teamNewFallbackTitle"),
      lead_agent_id: MANAGER_AGENT_ID,
      member_agent_ids: createTeamMemberIDs,
    });
    setShowCreateTeamModal(false);
  }

  async function addAgentsToTeamRoom(teamID: string, agentIDs: string[]): Promise<void> {
    if (teamActionBusy || !data?.current_user_id) {
      return;
    }
    const team = teamsQuery.data?.find((item) => item.id === teamID);
    if (!team?.room_id) {
      setTeamActionError(t("teamActionFailed"));
      return;
    }
    const room = rooms.find((item) => item.id === team.room_id);
    const roomMembers = new Set(room?.members ?? []);
    const nextAgentIDs = agentIDs.filter((id) => id && !roomMembers.has(id));
    if (!nextAgentIDs.length) {
      return;
    }
    const inviterID = resolveRoomInviterID(room, {
      preferredInviterIDs: [team.lead_agent_id, data.current_user_id, MANAGER_AGENT_ID],
    });
    if (!inviterID) {
      setTeamActionError(t("teamActionFailed"));
      return;
    }

    setTeamActionBusy(true);
    setTeamActionError("");
    try {
      await inviteRoomUsersRequest({
        room_id: team.room_id,
        inviter_id: inviterID,
        user_ids: nextAgentIDs,
        locale,
      });
      await refreshWorkspaceBootstrap();
    } catch (err) {
      setTeamActionError(errorMessage(err, t("teamActionFailed")));
      throw err;
    } finally {
      setTeamActionBusy(false);
    }
  }

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
        (room) => isDirectConversation(room) && room.members.includes(currentUserID) && room.members.includes(userID),
      ) ?? null
    );
  }

  async function openAgentDirectMessage(item: AgentLike | null | undefined): Promise<void> {
    const channelUserID = resolveAgentChannelUserID(item);
    if (!channelUserID || !data?.current_user_id) {
      return;
    }

    setAgentsError("");
    try {
      let nextData = null;
      let direct = directConversationForUser(channelUserID);
      if (!direct) {
        await createUserRequest({
          id: channelUserID,
          name: String(item?.name || item?.handle || channelUserID),
          handle: String(item?.handle || channelUserID.replace(/^u-/, "") || item?.name || channelUserID),
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
        setAgentsError(t("agentActionFailed"));
        return;
      }
      selectConversation(direct.id, { rooms: nextData?.rooms ?? rooms });
    } catch (err) {
      setAgentsError(errorMessage(err, t("agentActionFailed")));
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
    openCreateNotificationParticipantModal,
    openEditAgentModal,
    runningAgentCount,
    runAgentAction,
    selectedAgentForPage,
    teams: teamsQuery.data ?? [],
    teamsLoading: teamsQuery.isLoading,
    workerAgentItems,
    notifierWebhookPublicOrigin,
    agentViewProps: {
      item: selectedAgentForPage,
      t,
      busyKey: agentActionBusy,
      error: agentsDisplayError,
      draft: agentPageDraft,
      savedDraft: agentPageSavedDraft,
      hasUnsavedChanges: agentPageHasUnsavedChanges,
      models: agentPageModels,
      modelBusy: agentPageModelBusy,
      saving: agentPageBusy,
      publishBusy: agentPagePublishBusy,
      saveError: agentPageError,
      notice: agentPageNotice,
      authStatuses: cliproxyAuthStatuses,
      authBusyProvider: cliproxyAuthBusy,
      notifierWebhookPublicOrigin,
      workspaceEntries: agentWorkspaceQuery.data?.entries ?? [],
      workspaceLoading: agentWorkspaceQuery.isFetching,
      workspaceError: agentWorkspaceError,
      workspaceSupported: selectedAgentWorkspaceSupported,
      selectedWorkspacePath: selectedAgentWorkspacePath,
      workspaceFile: agentWorkspaceFileQuery.data ?? null,
      workspaceFileLoading: agentWorkspaceFileQuery.isFetching,
      workspaceFileError: agentWorkspaceFileError,
      onSelectWorkspaceFile: setSelectedAgentWorkspacePath,
      onDraftChange: setAgentPageDraft,
      onSave: saveAgentPage,
      onAvatarSave: saveAgentPageAvatar,
      onPublish: publishAgentPage,
      onProviderLogin: loginCLIProxyProvider,
      onStart: (item: AgentLike | null | undefined) => runAgentAction(item, "start"),
      onStop: (item: AgentLike | null | undefined) => runAgentAction(item, "stop"),
      onRecreate: (item: AgentLike | null | undefined) => runAgentAction(item, "recreate"),
      onUpgrade: (item: AgentLike | null | undefined) => runAgentAction(item, "upgrade"),
      onDelete: (item: AgentLike | null | undefined) => runAgentAction(item, "delete"),
      onInvite: inviteAgentToRoom,
      onOpenDM: openAgentDirectMessage,
      teamActionBusy,
      teamActionError,
      onCreateTeam: createAgentTeam,
      onAddAgentsToTeam: addAgentsToTeamRoom,
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
            agentModels,
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
          error: agentsError,
          progress: agentProgress,
          onRuntimeKindChange: updateManagerRebuildRuntimeKind,
          onClose: () => {
            setShowManagerRebuildModal(false);
            setAgentProgress(null);
          },
          onConfirm: confirmManagerRebuild,
        }
      : null,
    createTeamModalProps: showCreateTeamModal
      ? {
          t,
          candidates: createTeamCandidates,
          teamTitle: createTeamTitle,
          onTeamTitleChange: setCreateTeamTitle,
          teamMemberIDs: createTeamMemberIDs,
          onTeamMemberIDsChange: setCreateTeamMemberIDs,
          submitError: teamActionError,
          teamActionBusy,
          onClose: () => setShowCreateTeamModal(false),
          onCreate: createTeam,
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
