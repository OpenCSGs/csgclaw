// @ts-nocheck
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { applyUpgradeRequest } from "@/api/upgrade";
import {
  createBotRequest,
  createManagerAgentRequest,
  deleteBotRequest,
  fetchAgentProfile,
  fetchAgentProfileDefaults,
  runAgentActionRequest,
  saveManagerProfileRequest,
  updateAgentRequest,
} from "@/api/agents";
import { loginCLIProxyProviderRequest } from "@/api/cliproxy";
import { publishAgentTemplateRequest } from "@/api/hub";
import {
  createRoomRequest,
  createUserRequest,
  deleteRoomRequest,
  inviteRoomUsersRequest,
  joinAgentToRoomRequest,
  sendMessageRequest,
} from "@/api/im";
import { ACTION_REBUILD_MANAGER, MESSAGE_LIST_BOTTOM_THRESHOLD } from "@/bootstrap/constants";
import {
  applyTemplateToDraft,
  advanceAgentProgress,
  agentToDraft,
  availableManagerRuntimeOptions,
  draftNotifierRuntimeOptionsForSave,
  draftToProfile,
  ensureNotifierPullSubscriptionDraft,
  isAgentRunning,
  isManagerAgent,
  isNotifierRuntimeDraft,
  isNotifierRuntimeDraftOnAgentPage,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  normalizeTemplateSelection,
  pickDefaultAgentTemplate,
  profileToDraft,
  providerNeedsAuth,
  runtimeImageForKind,
  startAgentCreateProgress,
} from "@/models/agents";
import {
  agentMatchesUser,
  appendMessageToData,
  applyIMEvent,
  isDirectConversation,
  isToolCallMessage,
  removeConversationFromData,
  upsertConversationInData,
} from "@/models/conversations";
import {
  areComposerSegmentsEqual,
  getComposerMentionState,
  insertComposerLineBreak,
  parseComposerSegments,
  placeCaretAtEnd,
  removeAdjacentMentionToken,
  renderComposerSegments,
  replaceMentionQueryWithToken,
  segmentsToPlainText,
  serializeComposerSegments,
  updateDrafts,
} from "@/models/composer";
import { normalizeUpgradeStatus } from "@/models/upgradeStatus";
import { createTranslator, localizeError } from "@/shared/i18n";
import { messages } from "@/shared/i18n/messages";
import { initializeMermaidTheme } from "@/components/business/MessageContent";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import {
  LOCALE_STORAGE_KEY,
  SIDEBAR_COLLAPSED_STORAGE_KEY,
  THEME_STORAGE_KEY,
  WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY,
} from "@/shared/storage/keys";
import { errorMessage } from "@/api/client";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useCLIProxyAuthStatuses } from "./useCLIProxyAuthStatuses";
import { useProfileModelOptions } from "./useProfileModelOptions";
import { useWorkspaceData } from "./useWorkspaceData";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";
import { useWorkspaceNavigation } from "./useWorkspaceNavigation";

export function useWorkspaceController() {
  const location = useLocation();
  const navigate = useNavigate();
  const locale = useWorkspaceUiStore((state) => state.locale);
  const setLocale = useWorkspaceUiStore((state) => state.setLocale);
  const theme = useWorkspaceUiStore((state) => state.theme);
  const setTheme = useWorkspaceUiStore((state) => state.setTheme);
  const showToolCalls = useWorkspaceUiStore((state) => state.showToolCalls);
  const setShowToolCalls = useWorkspaceUiStore((state) => state.setShowToolCalls);
  const isSidebarCollapsed = useWorkspaceUiStore((state) => state.isSidebarCollapsed);
  const setIsSidebarCollapsed = useWorkspaceUiStore((state) => state.setIsSidebarCollapsed);
  const workspaceTab = useWorkspaceUiStore((state) => state.workspaceTab);
  const setWorkspaceTab = useWorkspaceUiStore((state) => state.setWorkspaceTab);
  const collapsedWorkspaceGroups = useWorkspaceUiStore((state) => state.collapsedWorkspaceGroups);
  const setCollapsedWorkspaceGroups = useWorkspaceUiStore((state) => state.setCollapsedWorkspaceGroups);
  const activeConversationId = useWorkspaceUiStore((state) => state.activeConversationId);
  const setActiveConversationId = useWorkspaceUiStore((state) => state.setActiveConversationId);
  const activePane = useWorkspaceUiStore((state) => state.activePane);
  const setActivePane = useWorkspaceUiStore((state) => state.setActivePane);
  const {
    bootstrapQuery,
    agentsQuery,
    hubTemplatesQuery,
    data,
    bootstrapConfig,
    managerProfile,
    agents,
    agentsLoaded,
    hubTemplates,
    hubLoaded,
    appVersion,
    upgradeStatus,
    setBootstrapData,
    setManagerProfileData,
    setUpgradeStatusData,
    setAppVersionData,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceUpgradeStatus,
    refreshWorkspaceAppVersion,
    refreshWorkspaceManagerProfile,
    refreshWorkspaceAgents,
    refreshWorkspaceHubTemplates,
  } = useWorkspaceData();
  const [draftsByConversationId, setDraftsByConversationId] = useState({});
  const [composerMentionState, setComposerMentionState] = useState(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [showCreateRoom, setShowCreateRoom] = useState(false);
  const [showInvite, setShowInvite] = useState(false);
  const [showMemberList, setShowMemberList] = useState(false);
  const [showChannelTools, setShowChannelTools] = useState(false);
  const [roomTitle, setRoomTitle] = useState("");
  const [roomDescription, setRoomDescription] = useState("");
  const [roomMemberIDs, setRoomMemberIDs] = useState([]);
  const [lockedRoomMemberIDs, setLockedRoomMemberIDs] = useState([]);
  const [inviteUserIDs, setInviteUserIDs] = useState([]);
  const [submitError, setSubmitError] = useState("");
  const [composerError, setComposerError] = useState("");
  const [profileDraft, setProfileDraft] = useState(null);
  const [profileError, setProfileError] = useState("");
  const [profileBusy, setProfileBusy] = useState(false);
  const [cliproxyAuthBusy, setCLIProxyAuthBusy] = useState("");
  const [agentsError, setAgentsError] = useState("");
  const [hubManualError, setHubManualError] = useState("");
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [showManagerRebuildModal, setShowManagerRebuildModal] = useState(false);
  const [managerRebuildRuntimeKind, setManagerRebuildRuntimeKind] = useState("picoclaw_sandbox");
  const [managerRebuildImage, setManagerRebuildImage] = useState("");
  const [agentModalMode, setAgentModalMode] = useState("create");
  const [editingAgent, setEditingAgent] = useState(null);
  const [agentDraft, setAgentDraft] = useState(null);
  const [agentBusy, setAgentBusy] = useState(false);
  const [agentError, setAgentError] = useState("");
  const [agentProgress, setAgentProgress] = useState(null);
  const [agentActionBusy, setAgentActionBusy] = useState("");
  const [messageActionBusy, setMessageActionBusy] = useState("");
  const [messageActionError, setMessageActionError] = useState({ key: "", message: "" });
  const [agentPageDraft, setAgentPageDraft] = useState(null);
  const [agentPageBusy, setAgentPageBusy] = useState(false);
  const [agentPagePublishBusy, setAgentPagePublishBusy] = useState(false);
  const [agentPageError, setAgentPageError] = useState("");
  const [notifierModalWebhookOrigin, setNotifierModalWebhookOrigin] = useState("");
  const [notifierPageWebhookOrigin, setNotifierPageWebhookOrigin] = useState("");
  const [profilePreview, setProfilePreview] = useState(null);
  const [upgradeBusy, setUpgradeBusy] = useState(false);
  const [upgradeError, setUpgradeError] = useState("");
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradePhase, setUpgradePhase] = useState("idle");
  const editorRef = useRef(null);
  const messageListRef = useRef(null);
  const memberMenuRef = useRef(null);
  const channelToolsRef = useRef(null);
  const profilePreviewRef = useRef(null);
  const agentRefreshTimerRef = useRef(null);
  const upgradePollTimerRef = useRef(null);
  const shouldAutoScrollRef = useRef(true);
  const autoScrollConversationRef = useRef(activeConversationId);
  const t = useMemo(() => createTranslator(locale), [locale]);
  const managerProfileIncomplete = managerProfile && managerProfile.profile_complete === false;
  const hub = useWorkspaceHubSelection({
    templates: hubTemplates,
    templatesQuery: hubTemplatesQuery,
    loaded: hubLoaded,
    manualError: hubManualError,
    refreshTemplates: refreshHubTemplates,
    t,
  });
  const { selectedHubTemplateId, setSelectedHubTemplateId } = hub;
  const {
    models: profileModels,
    modelBusy: profileModelBusy,
    resetModels: resetProfileModels,
  } = useProfileModelOptions({
    draft: profileDraft,
    enabled: Boolean(managerProfileIncomplete),
    onDraftChange: setProfileDraft,
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
    enabled: activePane.type === "agent",
    onDraftChange: setAgentPageDraft,
  });
  const { cliproxyAuthStatuses, setCLIProxyAuthStatus } = useCLIProxyAuthStatuses(
    [
      managerProfile?.provider,
      profileDraft?.provider,
      isNotifierRuntimeDraft(agentDraft) ? "" : agentDraft?.provider,
      isNotifierRuntimeDraft(agentPageDraft) ? "" : agentPageDraft?.provider,
    ],
    t,
  );

  useEffect(() => {
    return () => {
      if (upgradePollTimerRef.current) {
        window.clearInterval(upgradePollTimerRef.current);
        upgradePollTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (!bootstrapConfig?.runtime_kind) {
      return;
    }
    setProfileDraft((current) =>
      current && !current.runtime_kind
        ? { ...current, runtime_kind: normalizeRuntimeKind(bootstrapConfig.runtime_kind) }
        : current,
    );
  }, [bootstrapConfig?.runtime_kind]);

  useEffect(() => {
    if (!agentBusy || !agentProgress?.steps?.length) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      setAgentProgress((current) => advanceAgentProgress(current));
    }, 1200);
    return () => window.clearInterval(timer);
  }, [agentBusy, agentProgress?.startedAt]);

  useEffect(() => {
    // Temporarily disable background agent polling for debugging multi-tab pending requests.
    // function refreshVisibleAgents() {
    //   if (document.visibilityState === "visible") {
    //     refreshAgents({ silent: true });
    //   }
    // }
    //
    // const intervalID = window.setInterval(refreshVisibleAgents, AGENT_STATUS_REFRESH_INTERVAL_MS);
    // document.addEventListener("visibilitychange", refreshVisibleAgents);
    return () => {
      // window.clearInterval(intervalID);
      // document.removeEventListener("visibilitychange", refreshVisibleAgents);
    };
  }, []);

  useEffect(() => {
    const unsubscribe = subscribeIMEvents((payload) => {
      setBootstrapData((current) => applyIMEvent(current, payload));
      if (payload?.type === "upgrade.status_changed" && payload.upgrade) {
        const next = normalizeUpgradeStatus(payload.upgrade);
        setUpgradeStatusData(next);
        if (next?.upgrading) {
          setUpgradeBusy(true);
          setUpgradePhase((phase) => (phase === "done" ? phase : "restarting"));
        } else if (!next?.update_available) {
          setUpgradeBusy(false);
        }
      }
      // Temporarily disable event-driven agent polling for debugging multi-tab pending requests.
      // if (isAgentRosterEvent(payload)) {
      //   scheduleAgentsRefresh();
      // }
    });

    return () => {
      unsubscribe();
      if (agentRefreshTimerRef.current) {
        window.clearTimeout(agentRefreshTimerRef.current);
        agentRefreshTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    document.title = messages[locale].pageTitle;
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  }, [locale]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    initializeMermaidTheme(theme);
  }, [theme]);

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, String(isSidebarCollapsed));
  }, [isSidebarCollapsed]);

  useEffect(() => {
    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, JSON.stringify(collapsedWorkspaceGroups));
  }, [collapsedWorkspaceGroups]);

  useEffect(() => {
    if (!showAgentModal || !agentDraft || !isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent)) {
      return;
    }
    setNotifierModalWebhookOrigin(typeof window !== "undefined" ? window.location.origin : "");
  }, [showAgentModal, agentDraft?.runtime_kind, editingAgent?.id]);

  useEffect(() => {
    if (hubTemplatesQuery.isSuccess) {
      setHubManualError("");
    }
  }, [hubTemplatesQuery.isSuccess, hubTemplatesQuery.dataUpdatedAt]);

  const loadingError = bootstrapQuery.isError ? t("loadingFailed") : "";
  const agentsDisplayError =
    agentsError || (agentsQuery.isError ? errorMessage(agentsQuery.error, t("agentActionFailed")) : "");
  const rooms = useMemo(() => data?.rooms ?? [], [data]);
  const { navigatePane, selectConversation, selectAgent, selectComputer, selectHub } = useWorkspaceNavigation({
    location,
    navigate,
    dataReady: Boolean(data),
    activePane,
    setActivePane,
    activeConversationId,
    setActiveConversationId,
    setWorkspaceTab,
    setShowMemberList,
    setShowChannelTools,
    rooms,
  });

  useEffect(() => {
    if (!managerProfile) {
      return;
    }
    setProfileDraft({
      ...profileToDraft(managerProfile),
      runtime_kind: normalizeRuntimeKind(bootstrapConfig?.runtime_kind || managerProfile.runtime_kind),
    });
  }, [managerProfile, bootstrapConfig?.runtime_kind]);

  const usersById = useMemo(() => {
    const result = new Map();
    data?.users.forEach((user) => result.set(user.id, user));
    return result;
  }, [data]);

  const activeConversation = useMemo(
    () => data?.rooms.find((item) => item.id === activeConversationId) ?? null,
    [data, activeConversationId],
  );

  const visibleMessages = useMemo(() => {
    if (!activeConversation) {
      return [];
    }
    if (showToolCalls) {
      return activeConversation.messages;
    }
    return activeConversation.messages.filter((message) => !isToolCallMessage(message.content));
  }, [activeConversation, showToolCalls]);

  const roomCount = rooms.length;
  const channels = useMemo(() => rooms.filter((room) => !isDirectConversation(room)), [rooms]);
  const directMessages = useMemo(() => rooms.filter((room) => isDirectConversation(room)), [rooms]);
  const selectedAgentForPage = useMemo(() => {
    if (activePane.type !== "agent") {
      return null;
    }
    const managerAgent = agents.find((item) => item.role === "manager" || item.id === "u-manager");
    const workerAgents = agents.filter((item) => item.id !== managerAgent?.id);
    return [managerAgent, ...workerAgents].filter(Boolean).find((item) => item.id === activePane.id) ?? null;
  }, [agents, activePane]);

  useEffect(() => {
    if (!agentPageDraft || !isNotifierRuntimeDraftOnAgentPage(agentPageDraft, selectedAgentForPage)) {
      return;
    }
    setNotifierPageWebhookOrigin(typeof window !== "undefined" ? window.location.origin : "");
  }, [agentPageDraft?.runtime_kind, selectedAgentForPage?.id]);

  const mentionCandidates = useMemo(() => {
    if (!data || !composerMentionState) {
      return [];
    }
    const allowed = new Set(activeConversation?.members ?? []);
    return data.users
      .filter((user) => allowed.has(user.id))
      .filter(
        (user) =>
          user.handle.toLowerCase().includes(composerMentionState.query.toLowerCase()) ||
          user.name.toLowerCase().includes(composerMentionState.query.toLowerCase()),
      )
      .slice(0, 5);
  }, [data, activeConversation, composerMentionState]);
  const mentionableUsersByHandle = useMemo(() => {
    const result = new Map();
    if (!data) {
      return result;
    }
    const allowed = new Set(activeConversation?.members ?? []);
    data.users
      .filter((user) => allowed.has(user.id))
      .forEach((user) => {
        const handle = String(user.handle ?? "")
          .trim()
          .toLowerCase();
        if (handle && !result.has(handle)) {
          result.set(handle, user);
        }
      });
    return result;
  }, [data, activeConversation]);

  const draftSegments = useMemo(
    () => draftsByConversationId[activeConversationId] ?? [],
    [draftsByConversationId, activeConversationId],
  );
  const draftText = useMemo(() => segmentsToPlainText(draftSegments), [draftSegments]);

  useEffect(() => {
    setMentionIndex(0);
  }, [activeConversationId, composerMentionState?.query, draftText]);

  useEffect(() => {
    if (!showCreateRoom) {
      setRoomTitle("");
      setRoomDescription("");
      setRoomMemberIDs([]);
      setLockedRoomMemberIDs([]);
      setSubmitError("");
    }
  }, [showCreateRoom]);

  useEffect(() => {
    if (!showInvite) {
      setInviteUserIDs([]);
      setSubmitError("");
    }
  }, [showInvite]);

  useEffect(() => {
    setShowMemberList(false);
    setShowChannelTools(false);
  }, [activeConversationId]);

  useEffect(() => {
    if (!showMemberList) {
      return undefined;
    }

    function handlePointerDown(event) {
      const menu = memberMenuRef.current;
      if (!menu || menu.contains(event.target)) {
        return;
      }
      setShowMemberList(false);
    }

    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [showMemberList]);

  useEffect(() => {
    if (!showChannelTools) {
      return undefined;
    }

    function handlePointerDown(event) {
      const menu = channelToolsRef.current;
      if (!menu || menu.contains(event.target)) {
        return;
      }
      setShowChannelTools(false);
    }

    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [showChannelTools]);

  useEffect(() => {
    if (!profilePreview) {
      return undefined;
    }

    function handlePointerDown(event) {
      const preview = profilePreviewRef.current;
      const anchor = profilePreview?.anchorEl;
      if (!preview || preview.contains(event.target) || anchor?.contains?.(event.target)) {
        return;
      }
      closeProfilePreview();
    }

    function handleViewportChange() {
      closeProfilePreview();
    }

    document.addEventListener("mousedown", handlePointerDown);
    window.addEventListener("resize", handleViewportChange);
    window.addEventListener("scroll", handleViewportChange, true);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      window.removeEventListener("resize", handleViewportChange);
      window.removeEventListener("scroll", handleViewportChange, true);
    };
  }, [profilePreview]);

  useEffect(() => {
    if (!data) {
      return;
    }
    if (!activeConversationId) {
      if (data.rooms.length > 0) {
        setActiveConversationId(data.rooms[0].id);
        if (!activePane.id) {
          const next = { type: "conversation", id: data.rooms[0].id };
          setActivePane(next);
          navigatePane(next, data.rooms, { replace: true });
        }
      } else {
        if (!activePane.id) {
          const next = { type: "computer", id: "local" };
          setActivePane(next);
          navigatePane(next, data.rooms, { replace: true });
        }
      }
      return;
    }
    if (!data.rooms.some((room) => room.id === activeConversationId)) {
      const nextID = data.rooms[0]?.id ?? "";
      if (nextID) {
        selectConversation(nextID, { replace: true });
      } else {
        setActiveConversationId("");
        selectComputer({ replace: true });
      }
    }
  }, [data, activeConversationId, activePane.id]);

  useEffect(() => {
    if (!activePane || activePane.type !== "agent") {
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
  }, [agents, agentsLoaded, activePane, activeConversationId]);

  useEffect(() => {
    if (!selectedAgentForPage) {
      setAgentPageDraft(null);
      setAgentPageError("");
      setAgentPagePublishBusy(false);
      return;
    }
    loadAgentPageDraft(selectedAgentForPage);
  }, [selectedAgentForPage?.id]);

  useEffect(() => {
    const el = messageListRef.current;
    if (!el) {
      return;
    }
    const updateAutoScrollState = () => {
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      shouldAutoScrollRef.current = distanceFromBottom <= MESSAGE_LIST_BOTTOM_THRESHOLD;
    };
    updateAutoScrollState();
    el.addEventListener("scroll", updateAutoScrollState);
    return () => el.removeEventListener("scroll", updateAutoScrollState);
  }, [activeConversationId]);

  useLayoutEffect(() => {
    if (activePane.type !== "conversation") {
      return;
    }
    const el = messageListRef.current;
    if (!el) {
      return;
    }
    autoScrollConversationRef.current = activeConversationId;
    el.scrollTop = el.scrollHeight;
    shouldAutoScrollRef.current = true;
  }, [activePane.type, activeConversationId]);

  useEffect(() => {
    const el = messageListRef.current;
    if (autoScrollConversationRef.current !== activeConversationId) {
      autoScrollConversationRef.current = activeConversationId;
      shouldAutoScrollRef.current = false;
      return;
    }
    if (!el || !shouldAutoScrollRef.current) {
      return;
    }
    el.scrollTop = el.scrollHeight;
  }, [visibleMessages.length]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    if (!areComposerSegmentsEqual(parseComposerSegments(editor), draftSegments)) {
      renderComposerSegments(editor, draftSegments);
    }
    setComposerMentionState(null);
  }, [activeConversationId]);

  useEffect(() => {
    if (!activeConversationId || showCreateRoom || showInvite) {
      return;
    }
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    requestAnimationFrame(() => {
      if (editorRef.current !== editor) {
        return;
      }
      editor.focus();
      placeCaretAtEnd(editor);
    });
  }, [activeConversationId, showCreateRoom, showInvite]);

  async function refreshBootstrap() {
    const normalized = await refreshWorkspaceBootstrap();
    if (!normalized) {
      return null;
    }
    setInviteUserIDs([]);
    if (!activeConversationId && normalized.rooms.length > 0) {
      if (activePane.id && activePane.type !== "conversation") {
        setActiveConversationId(normalized.rooms[0].id);
      } else {
        selectConversation(normalized.rooms[0].id, { replace: true, rooms: normalized.rooms });
      }
    }
    return normalized;
  }

  async function refreshBootstrapConfig() {
    return refreshWorkspaceBootstrapConfig();
  }

  async function refreshUpgradeStatus() {
    const payload = await refreshWorkspaceUpgradeStatus();
    if (payload?.upgrading) {
      setUpgradeBusy(true);
      setUpgradePhase((phase) => (phase === "done" ? phase : "restarting"));
    } else if (!payload?.update_available) {
      setUpgradeBusy(false);
    }
    return payload;
  }

  function stopUpgradePoll() {
    if (upgradePollTimerRef.current) {
      window.clearInterval(upgradePollTimerRef.current);
      upgradePollTimerRef.current = null;
    }
  }

  function startUpgradeReconnectPoll(expectedVersion) {
    stopUpgradePoll();
    let attempts = 0;
    const poll = async () => {
      attempts += 1;
      try {
        const version = await refreshWorkspaceAppVersion({ cacheBust: true });
        const expected = (expectedVersion || "").trim();
        if (version && (!expected || version === expected)) {
          stopUpgradePoll();
          setAppVersionData(version);
          setUpgradeBusy(false);
          setUpgradePhase("done");
          setUpgradeStatusData((current) => ({
            ...(current || {}),
            current_version: version,
            latest_version: version,
            update_available: false,
            checking: false,
            upgrading: false,
            last_error: "",
          }));
          return;
        }
      } catch (_) {
        // The daemon is expected to be unavailable while the upgrade helper restarts it.
      }
      if (attempts >= 60) {
        stopUpgradePoll();
        setUpgradeBusy(false);
        setUpgradePhase("error");
        const latest = await refreshUpgradeStatus();
        const detail = latest?.last_error ? ` ${latest.last_error}` : "";
        setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
      }
    };
    poll();
    upgradePollTimerRef.current = window.setInterval(poll, 2000);
  }

  async function applyUpgrade() {
    if (upgradeBusy || upgradeStatus?.upgrading) {
      return;
    }

    setUpgradeBusy(true);
    setUpgradeError("");
    setUpgradePhase("starting");
    setShowUpgradeModal(true);
    try {
      await applyUpgradeRequest();
      setUpgradePhase("restarting");
      setUpgradeStatusData((current) => ({
        ...(current || {}),
        upgrading: true,
        last_error: "",
      }));
      startUpgradeReconnectPoll(upgradeStatus?.latest_version);
      setShowUpgradeModal(false);
    } catch (err) {
      setUpgradeBusy(false);
      setUpgradePhase("error");
      const detail = err?.message && err.message !== "upgrade apply failed" ? ` ${err.message}` : "";
      setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
    }
  }

  async function sendMessage() {
    if (managerProfileIncomplete) {
      setComposerError(t("profileIncomplete"));
      return;
    }
    const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
    if (providerNeedsAuth(managerProvider) && cliproxyAuthStatuses[managerProvider]?.authenticated === false) {
      setComposerError(t("authRequired"));
      return;
    }
    if (!data || !activeConversation || !draftText.trim()) {
      return;
    }

    setComposerError("");
    try {
      const created = await sendMessageRequest({
        room_id: activeConversation.id,
        sender_id: data.current_user_id,
        content: serializeComposerSegments(draftSegments),
      });
      setBootstrapData((current) => appendMessageToData(current, activeConversation.id, created));
      clearComposer();
    } catch (_) {
      setComposerError(t("sendFailed"));
    }
  }

  async function createRoom() {
    if (!data || !roomTitle.trim()) {
      return;
    }

    setSubmitError("");
    const memberIDs = roomMemberIDs.filter((id) => id && id !== data.current_user_id);
    try {
      const created = await createRoomRequest({
        title: roomTitle,
        description: roomDescription,
        creator_id: data.current_user_id,
        member_ids: memberIDs,
        locale,
      });
      setBootstrapData((current) => upsertConversationInData(current, created));
      selectConversation(created.id);
      setComposerError("");
      setShowCreateRoom(false);
    } catch (err) {
      setSubmitError(localizeError(err.message, t));
    }
  }

  function selectHubTemplate(item) {
    if (!item?.id) {
      selectHub();
      return;
    }
    setSelectedHubTemplateId(item.id);
    selectHub();
  }

  function toggleWorkspaceGroup(id) {
    setCollapsedWorkspaceGroups((current) => ({
      ...current,
      [id]: !current[id],
    }));
  }

  function openCreateRoomModal(options = {}) {
    if (!data) {
      return;
    }
    const lockedIDs = Array.from(new Set((options.lockedMemberIDs ?? [data.current_user_id]).filter(Boolean)));
    const selectedIDs = Array.from(new Set((options.preselectedMemberIDs ?? lockedIDs).filter(Boolean)));
    setRoomTitle(options.title ?? "");
    setRoomDescription(options.description ?? "");
    setRoomMemberIDs(selectedIDs);
    setLockedRoomMemberIDs(lockedIDs);
    setSubmitError("");
    setShowInvite(false);
    setShowCreateRoom(true);
  }

  function handleInviteAction() {
    if (!activeConversation) {
      return;
    }
    if (isDirectConversation(activeConversation)) {
      openCreateRoomModal({
        preselectedMemberIDs: activeConversation.members,
        lockedMemberIDs: activeConversation.members,
      });
      return;
    }
    setSubmitError("");
    setInviteUserIDs([]);
    setShowInvite(true);
  }

  async function inviteUsers() {
    if (!data || !activeConversation || inviteUserIDs.length === 0) {
      return;
    }

    setSubmitError("");
    try {
      const updated = await inviteRoomUsersRequest({
        room_id: activeConversation.id,
        inviter_id: data.current_user_id,
        user_ids: inviteUserIDs,
        locale,
      });
      setBootstrapData((current) => upsertConversationInData(current, updated));
      setComposerError("");
      setShowInvite(false);
    } catch (err) {
      setSubmitError(localizeError(err.message, t));
    }
  }

  async function deleteRoom(roomID) {
    if (!data || !roomID) {
      return;
    }

    try {
      await deleteRoomRequest(roomID);
    } catch (err) {
      setComposerError(localizeError(err.message, t));
      return;
    }

    const remainingRooms = rooms.filter((item) => item.id !== roomID);
    setBootstrapData((current) => removeConversationFromData(current, roomID));
    setDraftsByConversationId((current) => {
      if (!current[roomID]) {
        return current;
      }
      const next = { ...current };
      delete next[roomID];
      return next;
    });
    setComposerError("");
    setSubmitError("");
    if (activeConversationId === roomID) {
      const nextID = remainingRooms[0]?.id ?? "";
      if (nextID) {
        selectConversation(nextID, { replace: true });
      } else {
        setActiveConversationId("");
        selectComputer({ replace: true });
      }
    }
  }

  function applyMention(user) {
    const editor = editorRef.current;
    const state = getComposerMentionState(editor);
    if (!state) {
      return;
    }
    if (!replaceMentionQueryWithToken(editor, state, user)) {
      return;
    }
    syncComposerFromEditor();
  }

  function onComposerKeyDown(event) {
    if (mentionCandidates.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setMentionIndex((value) => (value + 1) % mentionCandidates.length);
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setMentionIndex((value) => (value - 1 + mentionCandidates.length) % mentionCandidates.length);
        return;
      }
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        applyMention(mentionCandidates[mentionIndex]);
        return;
      }
    }

    if (event.key === "Backspace" && removeAdjacentMentionToken(editorRef.current, "backward")) {
      event.preventDefault();
      syncComposerFromEditor();
      return;
    }

    if (event.key === "Delete" && removeAdjacentMentionToken(editorRef.current, "forward")) {
      event.preventDefault();
      syncComposerFromEditor();
      return;
    }

    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      sendMessage();
      return;
    }

    if (event.key === "Enter" && event.shiftKey) {
      event.preventDefault();
      insertComposerLineBreak(editorRef.current);
      syncComposerFromEditor();
    }
  }

  function syncComposerFromEditor() {
    const editor = editorRef.current;
    if (!editor || !activeConversationId) {
      return;
    }
    const segments = parseComposerSegments(editor);
    setDraftsByConversationId((current) => updateDrafts(current, activeConversationId, segments));
    setComposerMentionState(getComposerMentionState(editor));
  }

  function clearComposer() {
    const editor = editorRef.current;
    if (editor) {
      editor.innerHTML = "";
      editor.focus();
    }
    if (activeConversationId) {
      setDraftsByConversationId((current) => updateDrafts(current, activeConversationId, []));
    }
    setComposerMentionState(null);
  }

  if (!data) {
    return {
      ready: false,
      loadingText: loadingError || t("loading"),
    };
  }

  const inviteCandidates = activeConversation
    ? data.users.filter((user) => !activeConversation.members.includes(user.id))
    : [];
  const activeConversationMembers = activeConversation
    ? activeConversation.members.map((id) => usersById.get(id)).filter(Boolean)
    : [];
  const inviteActionLabel =
    activeConversation && isDirectConversation(activeConversation) ? t("createRoomFromDM") : t("inviteMembers");

  const managerAgent = agents.find((item) => item.role === "manager" || item.id === "u-manager");
  const managerRuntimeOptions = availableManagerRuntimeOptions(bootstrapConfig);
  const workerAgents = agents.filter((item) => item.id !== managerAgent?.id);
  const agentItems = [managerAgent, ...workerAgents].filter(Boolean);
  const runningAgentCount = agentItems.filter(isAgentRunning).length;
  const selectedAgent = selectedAgentForPage;
  const selectedConversation = activePane.type === "conversation" ? activeConversation : null;
  const activeChannel =
    selectedConversation && !isDirectConversation(selectedConversation) ? selectedConversation : null;
  const selectedMessageCount = selectedConversation?.messages?.length ?? 0;
  const currentWorkspaceLabel =
    activePane.type === "agent"
      ? t("agentOverview")
      : activePane.type === "computer"
        ? t("computerOverview")
        : activePane.type === "hub"
          ? t("hubOverview")
          : t("conversationOverview");
  const previewUser =
    profilePreview?.type === "user"
      ? (usersById.get(profilePreview.id) ?? null)
      : profilePreview?.type === "agent"
        ? (usersById.get(profilePreview.id) ?? null)
        : null;
  const previewAgent = profilePreview
    ? (agentItems.find((item) => item.id === profilePreview.id || agentMatchesUser(item, previewUser)) ?? null)
    : null;

  async function refreshManagerProfile() {
    const profile = await refreshWorkspaceManagerProfile();
    if (!profile) {
      // The manager may not exist during the first bootstrap milliseconds.
      return;
    }
    setProfileDraft({
      ...profileToDraft(profile),
      runtime_kind: normalizeRuntimeKind(bootstrapConfig?.runtime_kind || profile.runtime_kind),
    });
  }

  async function refreshHubTemplates() {
    try {
      await refreshWorkspaceHubTemplates();
      setHubManualError("");
    } catch (_) {
      setHubManualError(t("hubLoadFailed"));
    }
  }

  async function loginCLIProxyProvider(provider) {
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
        message: err.message || t("authMissing"),
      });
    } finally {
      setCLIProxyAuthBusy("");
    }
  }

  function openManagerRebuildModal(item = managerAgent) {
    const initialRuntimeKind = normalizeRuntimeKind(
      item?.runtime_kind || bootstrapConfig?.runtime_kind || managerRebuildRuntimeKind,
    );
    const fallbackRuntimeKind = managerRuntimeOptions[0]?.value || "picoclaw_sandbox";
    const resolvedRuntimeKind = managerRuntimeOptions.some((option) => option.value === initialRuntimeKind)
      ? initialRuntimeKind
      : fallbackRuntimeKind;
    const resolvedImage = runtimeImageForKind(
      resolvedRuntimeKind,
      bootstrapConfig,
      item?.image || managerAgent?.image || "",
    );
    setManagerRebuildRuntimeKind(resolvedRuntimeKind);
    setManagerRebuildImage(resolvedImage);
    setShowManagerRebuildModal(true);
  }

  async function requestManagerRebuild(options = {}) {
    const runtimeKind = normalizeRuntimeKind(
      options.runtimeKind ||
        managerAgent?.runtime_kind ||
        bootstrapConfig?.runtime_kind ||
        managerRuntimeOptions[0]?.value,
    );
    const image = String(options.image ?? managerAgent?.image ?? "").trim();
    await createManagerAgentRequest({
      runtime_kind: runtimeKind,
      image,
    });
    await refreshAgents();
    await refreshManagerProfile();
    await refreshBootstrapConfig();
  }

  async function rebuildManagerFromBrowser(options = {}) {
    setAgentActionBusy("u-manager:recreate");
    setAgentsError("");
    try {
      await requestManagerRebuild(options);
      return true;
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
      return false;
    } finally {
      setAgentActionBusy("");
    }
  }

  async function confirmManagerRebuild() {
    if (agentActionBusy) {
      return;
    }
    const selectedRuntimeKind = normalizeRuntimeKind(
      managerRebuildRuntimeKind || managerAgent?.runtime_kind || bootstrapConfig?.runtime_kind,
    );
    const selectedImage = String(managerRebuildImage ?? "").trim();
    setMessageActionError({ key: "", message: "" });
    const rebuilt = await rebuildManagerFromBrowser({ runtimeKind: selectedRuntimeKind, image: selectedImage });
    if (rebuilt) {
      setShowManagerRebuildModal(false);
    }
  }

  async function handleMessageAction(action, message) {
    if (!action || action.id !== ACTION_REBUILD_MANAGER) {
      return;
    }
    if (messageActionBusy || agentActionBusy) {
      return;
    }
    setMessageActionError({ key: "", message: "" });
    openManagerRebuildModal(managerAgent);
  }

  async function saveManagerProfile() {
    if (!profileDraft) {
      return;
    }
    setProfileBusy(true);
    setProfileError("");
    try {
      const payload = draftToProfile(profileDraft);
      const saved = await saveManagerProfileRequest(payload);
      setManagerProfileData(saved);
      setProfileDraft({ ...profileToDraft(saved), agent_id: "u-manager" });
      await refreshManagerProfile();
      setComposerError("");
    } catch (err) {
      setProfileError(err.message || t("sendFailed"));
    } finally {
      setProfileBusy(false);
    }
  }

  async function refreshAgents(options = {}) {
    try {
      await refreshWorkspaceAgents(options);
      setAgentsError("");
    } catch (err) {
      if (!options.silent) {
        setAgentsError(errorMessage(err, t("agentActionFailed")));
      }
    }
  }

  function scheduleAgentsRefresh() {
    if (agentRefreshTimerRef.current) {
      window.clearTimeout(agentRefreshTimerRef.current);
    }
    agentRefreshTimerRef.current = window.setTimeout(() => {
      agentRefreshTimerRef.current = null;
      refreshAgents({ silent: true });
    }, 120);
  }

  async function openCreateAgentModal(template = undefined) {
    setAgentModalMode("create");
    setEditingAgent(null);
    setAgentError("");
    setAgentProgress(null);
    resetAgentModels();
    const preferredRuntimeKind =
      normalizeRuntimeKind(bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "") || "picoclaw_sandbox";
    const selectedTemplate =
      template === undefined
        ? pickDefaultAgentTemplate(hubTemplates, preferredRuntimeKind, bootstrapConfig)
        : normalizeTemplateSelection(template);
    try {
      const defaults = await fetchAgentProfileDefaults();
      const runtimeKind =
        normalizeRuntimeKind(
          selectedTemplate?.runtime_kind || bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "",
        ) || "picoclaw_sandbox";
      let draft = agentToDraft({
        image: runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""),
        runtime_kind: runtimeKind,
        agent_profile: defaults,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      setAgentDraft(draft);
      setShowAgentModal(true);
    } catch (_) {
      const runtimeKind =
        normalizeRuntimeKind(
          selectedTemplate?.runtime_kind || bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "",
        ) || "picoclaw_sandbox";
      let draft = agentToDraft({
        image: runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""),
        runtime_kind: runtimeKind,
        agent_profile: managerProfile,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      setAgentDraft(draft);
      setShowAgentModal(true);
    }
  }

  async function openEditAgentModal(item) {
    setAgentModalMode("edit");
    setEditingAgent(item);
    setAgentError("");
    setAgentProgress(null);
    resetAgentModels();
    try {
      let profile = item.agent_profile;
      try {
        profile = await fetchAgentProfile(item.id);
      } catch (err) {
        if (!profile) {
          throw err;
        }
      }
      const draft = ensureNotifierPullSubscriptionDraft(agentToDraft({ ...item, agent_profile: profile }));
      setAgentDraft(draft);
      setShowAgentModal(true);
    } catch (err) {
      setAgentError(err.message || t("agentActionFailed"));
    }
  }

  async function loadAgentPageDraft(item) {
    if (!item?.id) {
      return;
    }
    setAgentPageError("");
    resetAgentPageModels();
    try {
      let profile = item.agent_profile;
      try {
        profile = await fetchAgentProfile(item.id);
      } catch (err) {
        if (!profile) {
          throw err;
        }
      }
      const draft = ensureNotifierPullSubscriptionDraft(agentToDraft({ ...item, agent_profile: profile }));
      setAgentPageDraft(draft);
    } catch (err) {
      setAgentPageError(err.message || t("agentActionFailed"));
      const draft = ensureNotifierPullSubscriptionDraft(agentToDraft(item));
      setAgentPageDraft(draft);
    }
  }

  async function saveAgentPage() {
    if (!agentPageDraft || !selectedAgentForPage?.id) {
      return;
    }
    setAgentPageBusy(true);
    setAgentPageError("");
    try {
      const draft = ensureNotifierPullSubscriptionDraft(agentPageDraft);
      const profile = draftToProfile(draft, {
        name: agentPageDraft.name,
        description: agentPageDraft.description,
      });
      const runtimeOptions = draftNotifierRuntimeOptionsForSave(draft, {
        mergeNotifier: isNotifierRuntimeDraftOnAgentPage(agentPageDraft, selectedAgentForPage),
      });
      const payload = {
        name: agentPageDraft.name,
        description: agentPageDraft.description,
        agent_profile: profile,
      };
      if (runtimeOptions) {
        payload.runtime_options = runtimeOptions;
      }
      const saved = await updateAgentRequest(selectedAgentForPage.id, payload);
      await refreshAgents();
      if (saved.id === "u-manager") {
        await refreshManagerProfile();
      }
      setAgentPageDraft(agentToDraft(saved));
    } catch (err) {
      setAgentPageError(err.message || t("agentActionFailed"));
    } finally {
      setAgentPageBusy(false);
    }
  }

  async function publishAgentPage() {
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
      setAgentPageError(err.message || t("agentActionFailed"));
    } finally {
      setAgentPagePublishBusy(false);
    }
  }

  async function saveAgent() {
    if (!agentDraft) {
      return;
    }
    setAgentBusy(true);
    setAgentError("");
    const isCreate = agentModalMode === "create";
    const runtimeKind = normalizeRuntimeKind(agentDraft.runtime_kind) || "picoclaw_sandbox";
    setAgentProgress(isCreate ? startAgentCreateProgress(runtimeKind) : null);
    try {
      const draft = ensureNotifierPullSubscriptionDraft(agentDraft);
      const profile = draftToProfile(draft, {
        name: agentDraft.name,
        description: agentDraft.description,
      });
      const runtimeOptions = draftNotifierRuntimeOptionsForSave(draft, {
        mergeNotifier: isNotifierRuntimeDraftOnAgentPage(agentDraft, editingAgent),
      });
      const payload = {
        name: agentDraft.name,
        role: "worker",
        description: agentDraft.description,
        image: isNotifierRuntimeDraft(draft) ? "" : agentDraft.image,
        runtime_kind: runtimeKind,
        from_template: agentDraft.from_template || "",
        agent_profile: profile,
      };
      if (runtimeOptions) {
        payload.runtime_options = runtimeOptions;
      }
      const saved = isCreate
        ? await createBotRequest(payload)
        : await updateAgentRequest(editingAgent.id, {
            name: payload.name,
            description: payload.description,
            agent_profile: payload.agent_profile,
            ...(payload.runtime_options ? { runtime_options: payload.runtime_options } : {}),
          });
      await refreshAgents();
      if (isCreate) {
        await refreshBootstrap();
      }
      if (saved.id === "u-manager") {
        await refreshManagerProfile();
      }
      if (isCreate) {
        setAgentProgress((current) =>
          current
            ? { ...current, percent: 100, status: "done", index: Math.max(0, (current.steps?.length || 1) - 1) }
            : current,
        );
      }
      setShowAgentModal(false);
      setAgentDraft(null);
      setAgentProgress(null);
    } catch (err) {
      setAgentProgress((current) => (current ? { ...current, status: "failed" } : current));
      setAgentError(err.message || t("agentActionFailed"));
    } finally {
      setAgentBusy(false);
    }
  }

  async function runAgentAction(item, action) {
    if (!item?.id || agentActionBusy) {
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
      if (action === "delete") {
        await deleteBotRequest(item.id);
      } else {
        await runAgentActionRequest(item.id, action);
      }
      await refreshAgents();
      if (item.id === "u-manager") {
        await refreshManagerProfile();
      }
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
    } finally {
      setAgentActionBusy("");
    }
  }

  async function deletePreviewBot(item) {
    if (!item?.id || agentActionBusy) {
      return;
    }
    if (!window.confirm(`${t("agentDelete")} ${item.name}?`)) {
      return;
    }
    setAgentActionBusy(`${item.id}:delete-bot`);
    setAgentsError("");
    try {
      await deleteBotRequest(item.id);
      closeProfilePreview();
      await refreshAgents();
      await refreshBootstrap();
      if (item.id === "u-manager") {
        await refreshManagerProfile();
      }
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
    } finally {
      setAgentActionBusy("");
    }
  }

  async function inviteAgentToRoom(item, options = {}) {
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
      await refreshBootstrap();
    } catch (err) {
      if (!options.silent) {
        setAgentsError(err.message || t("agentActionFailed"));
      }
    }
  }

  function directConversationForUser(userID, roomList = rooms, currentUserID = data?.current_user_id) {
    if (!userID || !currentUserID) {
      return null;
    }
    return (
      roomList.find(
        (room) => isDirectConversation(room) && room.members.includes(currentUserID) && room.members.includes(userID),
      ) ?? null
    );
  }

  async function openAgentDirectMessage(item) {
    if (!item?.id || !data?.current_user_id) {
      return;
    }

    setAgentsError("");
    try {
      let nextData = null;
      let direct = directConversationForUser(item.id);
      if (!direct) {
        await createUserRequest({
          id: item.id,
          name: item.name,
          handle: item.handle || item.id.replace(/^u-/, "") || item.name,
          role: item.role || "worker",
        });
        nextData = await refreshBootstrap();
        direct = directConversationForUser(
          item.id,
          nextData?.rooms ?? rooms,
          nextData?.current_user_id ?? data.current_user_id,
        );
      }

      if (!direct) {
        setAgentsError(t("agentActionFailed"));
        return;
      }
      selectConversation(direct.id, { rooms: nextData?.rooms ?? rooms });
      closeProfilePreview();
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
    }
  }

  function openParticipantPreview(user, anchor) {
    if (!user?.id) {
      return;
    }
    const rect = anchor?.getBoundingClientRect?.();
    if (!rect) {
      return;
    }
    const agent = agents.find((item) => agentMatchesUser(item, user));
    setProfilePreview((current) => {
      const nextType = agent ? "agent" : "user";
      const nextID = agent ? agent.id : user.id;
      if (current?.type === nextType && current?.id === nextID) {
        return null;
      }
      return {
        type: nextType,
        id: nextID,
        anchorRect: {
          top: rect.top,
          right: rect.right,
          bottom: rect.bottom,
          left: rect.left,
        },
        anchorEl: anchor,
      };
    });
    setShowMemberList(false);
    setShowChannelTools(false);
  }

  function openAgentPreview(item, anchor) {
    if (!item?.id) {
      return;
    }
    const rect = anchor?.getBoundingClientRect?.();
    if (!rect) {
      return;
    }
    setProfilePreview((current) => {
      if (current?.type === "agent" && current?.id === item.id) {
        return null;
      }
      return {
        type: "agent",
        id: item.id,
        anchorRect: {
          top: rect.top,
          right: rect.right,
          bottom: rect.bottom,
          left: rect.left,
        },
        anchorEl: anchor,
      };
    });
    setShowChannelTools(false);
  }

  function closeProfilePreview() {
    setProfilePreview(null);
  }

  return {
    ready: true,
    loadingText: "",
    shellClassName: `app-shell ${isSidebarCollapsed ? "sidebar-collapsed" : ""}`,
    activePane,
    sidebarProps: {
      isSidebarCollapsed,
      onCollapseSidebar: () => setIsSidebarCollapsed(true),
      onExpandSidebar: () => setIsSidebarCollapsed(false),
      theme,
      onThemeChange: setTheme,
      locale,
      onLocaleChange: setLocale,
      t,
      currentWorkspaceLabel,
      runningAgentCount,
      agentItems,
      workspaceTab,
      onWorkspaceTabChange: setWorkspaceTab,
      roomCount,
      channels,
      directMessages,
      activePane,
      currentUserID: data.current_user_id,
      usersById,
      collapsedWorkspaceGroups,
      onToggleWorkspaceGroup: toggleWorkspaceGroup,
      onCreateRoom: () => openCreateRoomModal(),
      onCreateAgent: openCreateAgentModal,
      hub,
      onSelectHubTemplate: selectHubTemplate,
      onSelectHub: selectHub,
      agentsError: agentsDisplayError,
      onSelectConversation: selectConversation,
      onPreviewUser: openParticipantPreview,
      onSelectAgent: selectAgent,
      onPreviewAgent: openAgentPreview,
      onSelectComputer: selectComputer,
      appVersion,
      upgradeStatus,
      upgradeBusy,
      upgradePhase,
      upgradeError,
      onOpenUpgrade: () => {
        setUpgradeError("");
        setUpgradePhase(upgradeBusy || upgradeStatus?.upgrading ? "restarting" : "idle");
        setShowUpgradeModal(true);
      },
    },
    hubViewProps: {
      t,
      locale,
      hub,
      onCreateFromTemplate: openCreateAgentModal,
    },
    agentViewProps: {
      item: selectedAgent,
      t,
      activeRoom: activeChannel,
      busyKey: agentActionBusy,
      error: agentsDisplayError,
      draft: agentPageDraft,
      models: agentPageModels,
      modelBusy: agentPageModelBusy,
      saving: agentPageBusy,
      publishBusy: agentPagePublishBusy,
      saveError: agentPageError,
      authStatuses: cliproxyAuthStatuses,
      authBusyProvider: cliproxyAuthBusy,
      notifierWebhookOrigin: notifierPageWebhookOrigin,
      setNotifierWebhookOrigin: setNotifierPageWebhookOrigin,
      onDraftChange: setAgentPageDraft,
      onSave: saveAgentPage,
      onPublish: publishAgentPage,
      onProviderLogin: loginCLIProxyProvider,
      onStart: (item) => runAgentAction(item, "start"),
      onStop: (item) => runAgentAction(item, "stop"),
      onRecreate: (item) => runAgentAction(item, "recreate"),
      onDelete: (item) => runAgentAction(item, "delete"),
      onInvite: inviteAgentToRoom,
      onOpenDM: openAgentDirectMessage,
    },
    computerViewProps: {
      t,
      agents: agentItems,
      channels,
      directMessages,
      activeAgentID: activePane.type === "agent" ? activePane.id : "",
      busyKey: agentActionBusy,
      onSelectAgent: selectAgent,
      onCreateAgent: openCreateAgentModal,
      onStartAgent: (item) => runAgentAction(item, "start"),
    },
    conversationViewProps: {
      conversation: selectedConversation,
      visibleMessages,
      currentUserID: data.current_user_id,
      usersById,
      locale,
      t,
      theme,
      selectedMessageCount,
      conversationMembers: activeConversationMembers,
      showMemberList,
      onToggleMemberList: setShowMemberList,
      showChannelTools,
      onToggleChannelTools: setShowChannelTools,
      showToolCalls,
      onToggleToolCalls: setShowToolCalls,
      memberMenuRef,
      channelToolsRef,
      messageListRef,
      editorRef,
      onPreviewUser: openParticipantPreview,
      onDeleteRoom: deleteRoom,
      inviteActionLabel,
      onInviteAction: handleInviteAction,
      mentionCandidates,
      mentionIndex,
      onApplyMention: applyMention,
      managerProfile,
      managerProfileIncomplete,
      authStatuses: cliproxyAuthStatuses,
      authBusyProvider: cliproxyAuthBusy,
      onProviderLogin: loginCLIProxyProvider,
      draftSegments,
      draftText,
      mentionableUsersByHandle,
      onSyncComposer: syncComposerFromEditor,
      onComposerKeyDown,
      onSendMessage: sendMessage,
      composerError,
      messageActionBusy,
      messageActionError,
      onMessageAction: handleMessageAction,
    },
    profilePreviewProps:
      profilePreview && (previewAgent || previewUser)
        ? {
            previewRef: profilePreviewRef,
            agent: previewAgent,
            user: previewUser,
            anchorRect: profilePreview.anchorRect,
            t,
            inDirectConversation: Boolean(selectedConversation && isDirectConversation(selectedConversation)),
            busyKey: agentActionBusy,
            onClose: closeProfilePreview,
            onOpenAgent: (item) => {
              selectAgent(item);
              closeProfilePreview();
            },
            onOpenDM: openAgentDirectMessage,
            onDelete: deletePreviewBot,
          }
        : null,
    createRoomModalProps: showCreateRoom
      ? {
          t,
          roomTitle,
          onRoomTitleChange: setRoomTitle,
          roomDescription,
          onRoomDescriptionChange: setRoomDescription,
          candidates: data.users,
          roomMemberIDs,
          lockedRoomMemberIDs,
          onRoomMemberIDsChange: setRoomMemberIDs,
          submitError,
          onClose: () => setShowCreateRoom(false),
          onCreate: createRoom,
        }
      : null,
    inviteMembersModalProps: showInvite
      ? {
          t,
          candidates: inviteCandidates,
          inviteUserIDs,
          onInviteUserIDsChange: setInviteUserIDs,
          submitError,
          onClose: () => setShowInvite(false),
          onInvite: inviteUsers,
        }
      : null,
    upgradeModalProps: showUpgradeModal
      ? {
          t,
          upgradeStatus,
          appVersion,
          upgradePhase,
          upgradeBusy,
          upgradeError,
          onClose: () => setShowUpgradeModal(false),
          onApply: applyUpgrade,
        }
      : null,
    agentProfileModalProps:
      showAgentModal && agentDraft
        ? {
            t,
            agentModalMode,
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
            notifierWebhookOrigin: notifierModalWebhookOrigin,
            setNotifierWebhookOrigin: setNotifierModalWebhookOrigin,
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
          bootstrapConfig,
          managerAgent,
          busy: agentActionBusy === "u-manager:recreate",
          error: agentsError,
          onRuntimeKindChange: setManagerRebuildRuntimeKind,
          onImageChange: setManagerRebuildImage,
          onClose: () => setShowManagerRebuildModal(false),
          onConfirm: confirmManagerRebuild,
        }
      : null,
    managerProfileSetupModalProps:
      managerProfileIncomplete && profileDraft
        ? {
            t,
            managerProfile,
            profileDraft,
            onProfileDraftChange: setProfileDraft,
            onProfileModelsReset: resetProfileModels,
            bootstrapConfig,
            profileModels,
            profileModelBusy,
            authStatuses: cliproxyAuthStatuses,
            authBusyProvider: cliproxyAuthBusy,
            onProviderLogin: loginCLIProxyProvider,
            profileError,
            profileBusy,
            onSave: saveManagerProfile,
          }
        : null,
  };
}
