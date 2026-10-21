import { pathForNewAgentTask, pathForNewTeamTask } from "@/models/routing";
import { bindAgentViews } from "./bindings/agent";
import {
  initialsForIdentity,
  resolveManagerDirectConversation,
  resolveDirectConversationAgent,
  withLocalIdentity,
} from "@/models/workspaceIdentity";
import { bindWorkspaceHub, saveMCPServerAndSelect } from "./bindings/hub";
import { useModelProviderCreation } from "./useModelProviderCreation";
import type { WorkspaceController } from "./workspaceControllerContract";
import type { TaskConfiguration } from "@/models/taskConfiguration";
import { settingsDialogSearch } from "@/models/workspaceDialogs";
import { useTaskEntry } from "./useTaskEntry";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { errorMessage } from "@/api/client";
import { patchCsgclawUserRequest } from "@/api/participants";
import { createTranslator } from "@/shared/i18n";
import {
  resolveConversationUser,
  upsertUserInData,
  mergeRoomMetadataUpdateInData,
  appendMessageToData,
  removeConversationFromData,
} from "@/models/conversations";
import { isAgentRunning, resolveAgentAvatarFallback } from "@/models/agents";
import { MANAGER_AGENT_NAME } from "@/shared/constants/agents";
import { agentTemplatesSearch, WorkspacePaneTypes, WorkspaceTabs, paneFromLocation } from "@/models/routing";
import { MODEL_PROVIDER_IDS, modelProviderCatalogForOpenCSGState } from "@/models/modelProviders";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useWorkspaceData } from "./useWorkspaceData";
import { useWorkspaceNavigation } from "./useWorkspaceNavigation";
import { useWorkspaceShellController } from "./useWorkspaceShellController";
import { useWorkspaceHubController } from "./useWorkspaceHubController";
import { useUpgradeController } from "./useUpgradeController";
import { useConfigController } from "./useConfigController";
import { useAuthController } from "./useAuthController";
import { useOpenCSGAuthGuard } from "./useOpenCSGAuthGuard";
import { useAgentController } from "./useAgentController";
import { useConversationController } from "./useConversationController";
import { useProfilePreviewController } from "./useProfilePreviewController";
import { useTaskController } from "./useTaskController";
import { useWorkspaceRealtime } from "./useWorkspaceRealtime";
import { useParticipantWorkStatus } from "./useParticipantWorkStatus";
import { useAgentTurnNotifications } from "./useAgentTurnNotifications";
import type { CreateTeamPayload } from "@/api/tasks";
import type { AgentLike } from "@/models/agents";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { MCPServer, MCPServerPayload } from "@/models/mcp";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";
import type { ModelProvider } from "@/models/modelProviders";
import type { SkillSummary } from "@/models/skillhub";
import { TurnNotificationModes } from "@/models/turnNotifications";
import type { TurnNotificationMode } from "@/models/turnNotifications";

export function useWorkspaceController(): WorkspaceController {
  const location = useLocation();
  const navigate = useNavigate();
  const locale = useWorkspaceUiStore((state) => state.locale);
  const setLocale = useWorkspaceUiStore((state) => state.setLocale);
  const theme = useWorkspaceUiStore((state) => state.theme);
  const setTheme = useWorkspaceUiStore((state) => state.setTheme);
  const turnNotificationMode = useWorkspaceUiStore((state) => state.turnNotificationMode);
  const turnNotificationSound = useWorkspaceUiStore((state) => state.turnNotificationSound);
  const setTurnNotificationSound = useWorkspaceUiStore((state) => state.setTurnNotificationSound);
  const setTurnNotificationMode = useWorkspaceUiStore((state) => state.setTurnNotificationMode);
  const showToolCalls = useWorkspaceUiStore((state) => state.showToolCalls);
  const setShowToolCalls = useWorkspaceUiStore((state) => state.setShowToolCalls);
  const floatingChatOpen = useWorkspaceUiStore((state) => state.floatingChatOpen);
  const storedHubResourceType = useWorkspaceUiStore((state) => state.selectedHubResourceType);
  const setFloatingChatOpen = useWorkspaceUiStore((state) => state.setFloatingChatOpen);
  const isSidebarCollapsed = useWorkspaceUiStore((state) => state.isSidebarCollapsed);
  const setIsSidebarCollapsed = useWorkspaceUiStore((state) => state.setIsSidebarCollapsed);
  const collapsedWorkspaceGroups = useWorkspaceUiStore((state) => state.collapsedWorkspaceGroups);
  const setCollapsedWorkspaceGroups = useWorkspaceUiStore((state) => state.setCollapsedWorkspaceGroups);
  const activeConversationId = useWorkspaceUiStore((state) => state.activeConversationId);
  const setActiveConversationId = useWorkspaceUiStore((state) => state.setActiveConversationId);
  const workspaceTab = useWorkspaceUiStore((state) => state.workspaceTab);
  const setWorkspaceTab = useWorkspaceUiStore((state) => state.setWorkspaceTab);
  const {
    bootstrapQuery,
    agentsQuery,
    hubTemplatesQuery,
    data,
    bootstrapConfig,
    managerProfile,
    agents,
    agentsLoaded,
    modelProviders: rawModelProviders,
    modelProvidersLoaded,
    hubTemplates,
    hubLoaded,
    appVersion,
    upgradeStatus,
    setBootstrapData,
    setManagerProfileData,
    setAgentsData,
    setUpgradeStatusData,
    setAppVersionData,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceUpgradeStatus,
    refreshWorkspaceAppVersion,
    refreshWorkspaceManagerProfile,
    refreshWorkspaceAgents,
    refreshWorkspaceModelProviders,
    refreshWorkspaceHubTemplates,
  } = useWorkspaceData();
  const t = useMemo(() => createTranslator(locale), [locale]);

  useEffect(() => {
    const currentUserID = String(data?.current_user_id ?? "").trim();
    if (!currentUserID) return;
    const currentUser = data?.users.find((user) => user.id === currentUserID);
    if (currentUser?.locale === locale) return;
    void patchCsgclawUserRequest(currentUserID, { locale })
      .then((updated) => {
        setBootstrapData((current) => upsertUserInData(current, updated));
      })
      .catch(() => undefined);
  }, [data?.current_user_id, data?.users, locale, setBootstrapData]);
  const displayData = useMemo(() => withLocalIdentity(data, t("localIdentityFallback")), [data, t]);
  const activePane = useMemo(() => paneFromLocation(location.pathname), [location.pathname]);
  const rooms = useMemo(() => displayData?.rooms ?? [], [displayData]);
  const [conversationProfileDetailAgentID, setConversationProfileDetailAgentID] = useState("");
  const conversationProfileDetailTriggerRef = useRef<HTMLElement | null>(null);
  const handleAgentDeleted = useCallback((item: AgentLike) => {
    setConversationProfileDetailAgentID((current) => {
      if (current !== item.id) {
        return current;
      }
      conversationProfileDetailTriggerRef.current = null;
      return "";
    });
  }, []);
  const loadingError = bootstrapQuery.isError ? t("loadingFailed") : "";
  const {
    navigatePane,
    selectConversation,
    selectAgent,
    selectHuman,
    selectTeam,
    selectTeamSection,
    selectNotificationSection,
    selectModelProvider,
    selectApps,
    selectComputer,
    selectHub,
    selectSettings,
    selectTasks,
  } = useWorkspaceNavigation({
    location,
    navigate,
    dataReady: Boolean(displayData),
    setActiveConversationId,
    rooms,
  });
  const ignoreFloatingChatNavigation = useCallback(() => {}, []);
  const shell = useWorkspaceShellController({
    activeConversationId,
    activePane,
    collapsedWorkspaceGroups,
    isSidebarCollapsed,
    locale,
    navigatePane,
    rooms,
    selectComputer,
    selectConversation,
    selectHub,
    selectTasks,
    setCollapsedWorkspaceGroups,
    setIsSidebarCollapsed,
    setWorkspaceTab,
    t,
    workspaceTab,
  });
  const auth = useAuthController(t);
  const loginOpenCSG = auth.login;
  const openCSGAuthGuard = useOpenCSGAuthGuard({ login: loginOpenCSG, status: auth.status });
  const modelProviders = useMemo(
    () =>
      modelProviderCatalogForOpenCSGState(rawModelProviders, {
        aiGatewayBaseURL: auth.environment.aiGatewayBaseURL,
        authenticationRequiredMessage: t("modelProviderOpenCSGAuthenticationRequired"),
        authenticated: openCSGAuthGuard.authenticated,
      }),
    [auth.environment.aiGatewayBaseURL, openCSGAuthGuard.authenticated, rawModelProviders, t],
  );
  const navigateAfterSkillDelete = useCallback(
    (nextSkill: SkillSummary | null) => {
      if (!nextSkill?.name) {
        selectHub({ replace: true });
        return;
      }
      navigatePane({ type: WorkspacePaneTypes.hub, id: nextSkill.name, resourceType: "skill" }, rooms, {
        replace: true,
      });
    },
    [navigatePane, rooms, selectHub],
  );
  const navigateBeforeTemplateDelete = useCallback(
    (templateID: string) => {
      const params = new URLSearchParams(location.search);
      if (
        activePane.type === WorkspacePaneTypes.agent &&
        params.get("tab") === "templates" &&
        params.get("template") === templateID
      ) {
        navigatePane({ type: WorkspacePaneTypes.agent, id: "" }, rooms, {
          replace: true,
          search: agentTemplatesSearch("", params.get("filter") === "personal" ? "personal" : "official"),
        });
        return;
      }
      if (
        activePane.type === WorkspacePaneTypes.hub &&
        activePane.resourceType === "template" &&
        activePane.id === templateID
      ) {
        navigatePane({ type: WorkspacePaneTypes.hub, id: "", resourceType: "template" }, rooms, { replace: true });
      }
    },
    [activePane, location.search, navigatePane, rooms],
  );
  const { hub, refreshHubTemplates } = useWorkspaceHubController({
    activePane,
    hubLoaded,
    hubTemplates,
    hubTemplatesQuery,
    onSkillDeleted: navigateAfterSkillDelete,
    onTemplateDeleteStarted: navigateBeforeTemplateDelete,
    openCSGAuthGuard,
    refreshWorkspaceHubTemplates,
    t,
  });
  const {
    setSelectedMCPServerName,
    setSelectedHubResourceType,
    setSelectedHubSkillName,
    setSelectedHubTemplateId,
    setSelectedKnowledgeBaseID,
  } = hub;
  const [skillUploadOpen, setSkillUploadOpen] = useState(false);
  const upgrade = useUpgradeController({
    appVersion,
    refreshWorkspaceAppVersion,
    refreshWorkspaceUpgradeStatus,
    setAppVersionData,
    setUpgradeStatusData,
    t,
    upgradeStatus,
  });
  const configSettings = useConfigController({
    hubTemplates,
    refreshWorkspaceAppVersion,
    t,
  });
  const participantWork = useParticipantWorkStatus({ agents, users: displayData?.users ?? [] });
  const agent = useAgentController({
    workingParticipantsForRoom: participantWork.workingParticipantsForRoom,
    redirectToIncompleteManagerProfile: false,
    activeConversationId,
    activePane,
    agents,
    agentsLoaded,
    agentsQuery,
    bootstrapConfig,
    data: displayData,
    catalogMCPServers: hub.mcpServers,
    catalogMCPServersError: hub.mcpStateError,
    catalogMCPServersLoading: hub.mcpServersLoading,
    hubTemplates,
    locale,
    managerProfile,
    modelProviders,
    modelProvidersLoaded,
    openCSGAuthGuard,
    onAgentDeleted: handleAgentDeleted,
    onBrowseTemplates: () => navigate("/agents?tab=templates"),
    onTemplatePublished: hub.showPublishedTemplate,
    profileDetailAgentID: conversationProfileDetailAgentID,
    refreshMCPServers: hub.refetchMCPServers,
    refreshHubTemplates,
    refreshWorkspaceAgents,
    refreshWorkspaceModelProviders,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceManagerProfile,
    rooms,
    navigatePane,
    selectAgent,
    selectComputer,
    selectConversation,
    selectHub,
    selectModelProvider,
    setAgentsData,
    setBootstrapData,
    setManagerProfileData,
    setSelectedHubTemplateId,
    t,
  });
  const closeConversationAgentDetail = useCallback(
    (restoreFocus = true, options: { skipUnsavedCheck?: boolean } = {}) => {
      if (!conversationProfileDetailAgentID) {
        return true;
      }
      if (
        !options.skipUnsavedCheck &&
        agent.agentViewProps.hasUnsavedChanges &&
        !window.confirm(t("agentUnsavedChangesWarning"))
      ) {
        return false;
      }
      const trigger = conversationProfileDetailTriggerRef.current;
      conversationProfileDetailTriggerRef.current = null;
      setConversationProfileDetailAgentID("");
      if (restoreFocus && trigger) {
        window.setTimeout(() => {
          if (trigger.isConnected) {
            trigger.focus();
          }
        }, 0);
      }
      return true;
    },
    [agent.agentViewProps.hasUnsavedChanges, conversationProfileDetailAgentID, t],
  );
  const managerDirectConversation = useMemo(
    () => resolveManagerDirectConversation(rooms, displayData?.current_user_id ?? "", agent.managerAgent),
    [agent.managerAgent, displayData?.current_user_id, rooms],
  );
  const conversation = useConversationController({
    activeConversationId,
    activePane,
    autoSelectFallbackConversation: location.pathname !== "/" && location.pathname !== "/new",
    messageListActive: location.pathname !== "/" && location.pathname !== "/new",
    agents,
    authBusyProvider: agent.cliproxyAuthBusy,
    authStatuses: agent.cliproxyAuthStatuses,
    modelProviders,
    data: displayData,
    locale,
    managerProfile,
    managerProfileIncomplete: agent.managerProfileIncomplete,
    managerRuntimeUnavailable: agent.managerRuntimeUnavailable,
    managerRuntimeWarning: agent.managerRuntimeWarning,
    messageActionBusy: agent.messageActionBusy,
    messageActionFeedback: agent.messageActionFeedback,
    hasObservedWorkLease: participantWork.hasObservedWorkLease,
    stopWorkingTurn: participantWork.stopWorkingTurn,
    workingParticipantsForRoom: participantWork.workingParticipantsForRoom,
    navigatePane,
    onMessageAction: agent.handleMessageAction,
    onProviderLogin: agent.loginCLIProxyProvider,
    openCSGAuthGuard,
    preferredFallbackConversationId: managerDirectConversation?.id ?? "",
    rooms,
    selectComputer,
    selectConversation,
    setActiveConversationId,
    setBootstrapData,
    showToolCalls,
    setShowToolCalls,
    t,
    theme,
  });
  const turnNotifications = useAgentTurnNotifications({
    agents,
    currentUserID: displayData?.current_user_id ?? "",
    mode: turnNotificationMode,
    soundEnabled: turnNotificationSound,
    onSelectConversation: selectConversation,
    rooms,
    t,
    usersById: conversation.usersById,
  });
  const {
    handleRealtimeEvent: handleTurnNotificationEvent,
    permission: turnNotificationPermission,
    requestPermission: requestTurnNotificationPermission,
  } = turnNotifications;
  const changeTurnNotificationMode = useCallback(
    async (mode: TurnNotificationMode) => {
      setTurnNotificationMode(mode);
      if (mode !== TurnNotificationModes.off && turnNotificationPermission !== "granted") {
        await requestTurnNotificationPermission();
      }
    },
    [requestTurnNotificationPermission, setTurnNotificationMode, turnNotificationPermission],
  );
  const floatingChatTargetConversation = managerDirectConversation;
  const floatingChatConversationID = floatingChatTargetConversation?.id ?? "";
  const floatingChatRooms = useMemo(
    () => (floatingChatTargetConversation ? [floatingChatTargetConversation] : []),
    [floatingChatTargetConversation],
  );
  const floatingChatPane = useMemo(
    () => ({ type: WorkspacePaneTypes.conversation, id: floatingChatConversationID }),
    [floatingChatConversationID],
  );
  const floatingConversation = useConversationController({
    activeConversationId: floatingChatConversationID,
    activePane: floatingChatPane,
    agents,
    autoSelectFallbackConversation: false,
    authBusyProvider: agent.cliproxyAuthBusy,
    authStatuses: agent.cliproxyAuthStatuses,
    modelProviders,
    data: displayData,
    locale,
    managerProfile,
    managerProfileIncomplete: agent.managerProfileIncomplete,
    managerRuntimeUnavailable: agent.managerRuntimeUnavailable,
    managerRuntimeWarning: agent.managerRuntimeWarning,
    messageActionBusy: agent.messageActionBusy,
    messageActionFeedback: agent.messageActionFeedback,
    hasObservedWorkLease: participantWork.hasObservedWorkLease,
    stopWorkingTurn: participantWork.stopWorkingTurn,
    workingParticipantsForRoom: participantWork.workingParticipantsForRoom,
    messageListActive: floatingChatOpen,
    navigatePane: ignoreFloatingChatNavigation,
    onMessageAction: agent.handleMessageAction,
    onProviderLogin: agent.loginCLIProxyProvider,
    openCSGAuthGuard,
    rooms: floatingChatRooms,
    selectComputer: ignoreFloatingChatNavigation,
    selectConversation: ignoreFloatingChatNavigation,
    setActiveConversationId: ignoreFloatingChatNavigation,
    setBootstrapData,
    showToolCalls,
    setShowToolCalls,
    t,
    theme,
  });
  const closeThreadPanel = conversation.conversationViewProps.onCloseThread;
  const openConversationAgentDetail = useCallback(
    (item: AgentLike | null | undefined, trigger?: HTMLElement | null) => {
      const agentID = String(item?.id || "").trim();
      if (!agentID) {
        return false;
      }
      if (conversationProfileDetailAgentID && conversationProfileDetailAgentID !== agentID) {
        const closed = closeConversationAgentDetail(false);
        if (!closed) {
          return false;
        }
      }
      closeThreadPanel();
      conversationProfileDetailTriggerRef.current =
        trigger ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
      setConversationProfileDetailAgentID(agentID);
      return true;
    },
    [closeConversationAgentDetail, closeThreadPanel, conversationProfileDetailAgentID],
  );
  useWorkspaceRealtime({
    agents,
    onAgentTurnNotificationEvent: handleTurnNotificationEvent,
    onConversationEvent: conversation.handleRealtimeEvent,
    onFloatingConversationEvent: floatingConversation.handleRealtimeEvent,
    onParticipantWorkEvent: participantWork.handleRealtimeEvent,
    onRefreshAgentState: agent.refreshAgentState,
    onUpgradeStatusChange: upgrade.handleUpgradeStatusChange,
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap,
    setBootstrapData,
    usersById: conversation.usersById,
  });
  const profilePreview = useProfilePreviewController({
    agentItems: agent.agentItems,
    closeConversationTools: () => {
      conversation.closeConversationTools();
      floatingConversation.closeConversationTools();
    },
    openAgentDirectMessage: agent.openAgentDirectMessage,
    selectAgent,
    t,
    usersById: conversation.usersById,
  });
  const closeProfilePreview = profilePreview.closeProfilePreview;
  const openConversationAgentDetailFromAvatar = useCallback(
    (item: AgentLike | null | undefined, trigger: HTMLElement) => {
      const opened = openConversationAgentDetail(item, trigger);
      if (!opened) {
        return;
      }
      closeProfilePreview();
    },
    [closeProfilePreview, openConversationAgentDetail],
  );
  const openThreadPanel = conversation.conversationViewProps.onOpenThread;
  const openConversationThreadPanel = conversation.openThreadInConversation;
  const selectConversationAndCloseThreadPanel = conversation.selectConversationAndCloseThread;
  const openConversationThread = useCallback(
    async (message: Parameters<typeof openThreadPanel>[0]) => {
      if (!closeConversationAgentDetail()) {
        return;
      }
      await openThreadPanel(message);
    },
    [closeConversationAgentDetail, openThreadPanel],
  );
  const openThreadInConversation = useCallback(
    async (...args: Parameters<typeof openConversationThreadPanel>) => {
      if (!closeConversationAgentDetail()) {
        return;
      }
      await openConversationThreadPanel(...args);
    },
    [closeConversationAgentDetail, openConversationThreadPanel],
  );
  const selectConversationAndCloseSidePanels = useCallback(
    (id: string) => {
      if (!closeConversationAgentDetail()) {
        return;
      }
      selectConversationAndCloseThreadPanel(id);
    },
    [closeConversationAgentDetail, selectConversationAndCloseThreadPanel],
  );
  const task = useTaskController({
    activePane,
    agents: agent.agentItems,
    rooms,
    t,
    onSelectConversation: selectConversation,
    onSelectTask: selectTasks,
  });
  const selectedTeamID = activePane.type === WorkspacePaneTypes.team ? String(activePane.id || "") : "";
  const selectedTeam = agent.teams.find((item) => item.id === selectedTeamID) ?? null;
  const selectedTeamTasks = selectedTeam ? task.tasks.filter((item) => item.team_id === selectedTeam.id) : [];
  const selectedHumanID =
    new URLSearchParams(location.search).get("dialog") === "profile"
      ? new URLSearchParams(location.search).get("human") || ""
      : activePane.type === WorkspacePaneTypes.human
        ? String(activePane.id || "")
        : "";
  const selectedHuman = selectedHumanID ? (conversation.usersById.get(selectedHumanID) ?? null) : null;
  const modelProviderCreation = useModelProviderCreation({
    catalog: modelProviders,
    locale,
    refresh: refreshWorkspaceModelProviders,
    onSelect: selectModelProvider,
    keepCurrentPage: new URLSearchParams(location.search).get("dialog") === "settings",
    t,
  });
  const [humanAvatarBusyID, setHumanAvatarBusyID] = useState("");
  const [humanAvatarError, setHumanAvatarError] = useState("");
  const [humanDescriptionBusyID, setHumanDescriptionBusyID] = useState("");
  const [humanDescriptionError, setHumanDescriptionError] = useState("");
  const updateHumanAvatar = useCallback(
    async (avatar: string) => {
      const selected = selectedHuman;
      const nextAvatar = String(avatar || "").trim();
      if (!selected?.id || !nextAvatar || selected.avatar === nextAvatar) {
        return;
      }

      setHumanAvatarBusyID(selected.id);
      setHumanAvatarError("");
      try {
        const updated = await patchCsgclawUserRequest(selected.id, { avatar: nextAvatar });
        const savedAvatar = String(updated.avatar || nextAvatar).trim() || nextAvatar;
        const updatedUserID = String(updated.id || selected.id).trim() || selected.id;
        setBootstrapData((current) => {
          if (!current) {
            return current;
          }
          const existing =
            current.users.find((item) => item.id === updatedUserID) ??
            current.users.find((item) => item.id === selected.id) ??
            selected;
          return upsertUserInData(current, {
            ...existing,
            ...updated,
            avatar: savedAvatar,
            participants: updated.participants ?? existing.participants,
          });
        });
      } catch (error) {
        setHumanAvatarError(errorMessage(error, t("humanAvatarSaveFailed")));
      } finally {
        setHumanAvatarBusyID((current) => (current === selected.id ? "" : current));
      }
    },
    [selectedHuman, setBootstrapData, t],
  );
  const humanAvatarBusy = Boolean(selectedHuman?.id && humanAvatarBusyID === selectedHuman.id);
  const updateHumanDescription = useCallback(
    async (description: string) => {
      const selected = selectedHuman;
      const nextDescription = String(description || "").trim();
      if (!selected?.id || String(selected.description || "").trim() === nextDescription) {
        return;
      }

      setHumanDescriptionBusyID(selected.id);
      setHumanDescriptionError("");
      try {
        const updated = await patchCsgclawUserRequest(selected.id, { description: nextDescription });
        setBootstrapData((current) => {
          if (!current) {
            return current;
          }
          const existing = current.users.find((item) => item.id === selected.id) ?? selected;
          return upsertUserInData(current, {
            ...existing,
            ...updated,
            description: String(updated.description || nextDescription),
            participants: updated.participants ?? existing.participants,
          });
        });
      } catch (error) {
        setHumanDescriptionError(errorMessage(error, t("humanDescriptionSaveFailed")));
      } finally {
        setHumanDescriptionBusyID((current) => (current === selected.id ? "" : current));
      }
    },
    [selectedHuman, setBootstrapData, t],
  );
  const humanDescriptionBusy = Boolean(selectedHuman?.id && humanDescriptionBusyID === selectedHuman.id);
  const floatingChatAgent = useMemo(
    () =>
      resolveDirectConversationAgent(
        floatingChatTargetConversation,
        displayData?.current_user_id ?? "",
        conversation.usersById,
        agent.agentItems,
      ),
    [agent.agentItems, conversation.usersById, displayData?.current_user_id, floatingChatTargetConversation],
  );
  const floatingChatUser =
    floatingChatTargetConversation && displayData?.current_user_id
      ? (resolveConversationUser(floatingChatTargetConversation, displayData.current_user_id, conversation.usersById) ??
        null)
      : null;
  const floatingChatTitle =
    floatingChatUser?.name || floatingChatAgent?.name || agent.managerAgent?.name || MANAGER_AGENT_NAME;
  const floatingChatAvatar = floatingChatUser?.avatar || floatingChatAgent?.avatar || null;
  const floatingChatAvatarFallback = floatingChatAgent
    ? resolveAgentAvatarFallback(floatingChatAgent, conversation.usersById)
    : initialsForIdentity(floatingChatTitle);
  const floatingChatConversation = floatingConversation.conversationViewProps.conversation;
  const floatingChatConversationProps = floatingChatConversation
    ? {
        ...floatingConversation.conversationViewProps,
        agents: agent.agentItems,
        conversation: floatingChatConversation,
        onPreviewUser: profilePreview.openParticipantPreview,
        showInviteAction: false,
        threadDisplay: "dialog" as const,
      }
    : null;

  const selectHubTemplate = useCallback(
    (item: HubTemplate | null | undefined) => {
      if (!item?.id) {
        setSelectedHubResourceType("template");
        setSelectedHubTemplateId("");
        selectHub();
        return;
      }
      setSelectedHubResourceType("template");
      setSelectedHubTemplateId(item.id);
      navigatePane({ type: WorkspacePaneTypes.hub, id: item.id, resourceType: "template" }, rooms);
    },
    [navigatePane, rooms, selectHub, setSelectedHubResourceType, setSelectedHubTemplateId],
  );

  const selectHubSkill = useCallback(
    (item: SkillSummary | null | undefined) => {
      if (!item?.name) {
        setSelectedHubResourceType("skill");
        setSelectedHubSkillName("");
        selectHub();
        return;
      }
      setSelectedHubResourceType("skill");
      setSelectedHubSkillName(item.name);
      navigatePane({ type: WorkspacePaneTypes.hub, id: item.name, resourceType: "skill" }, rooms);
    },
    [navigatePane, rooms, selectHub, setSelectedHubResourceType, setSelectedHubSkillName],
  );
  const selectMCPServer = useCallback(
    (item: MCPServer | null | undefined) => {
      if (!item?.name) {
        setSelectedHubResourceType("mcp");
        setSelectedMCPServerName("");
        navigatePane({ type: WorkspacePaneTypes.apps }, rooms);
        return;
      }
      setSelectedHubResourceType("mcp");
      setSelectedMCPServerName(item.name);
      navigatePane({ type: WorkspacePaneTypes.hub, id: item.name, resourceType: "mcp" }, rooms);
    },
    [navigatePane, rooms, setSelectedMCPServerName, setSelectedHubResourceType],
  );
  const selectKnowledgeBase = useCallback(
    (item: RemoteKnowledgeBase | null | undefined) => {
      const id = String(item?.id || "").trim();
      setSelectedHubResourceType("knowledge");
      setSelectedKnowledgeBaseID(id);
      navigatePane({ type: WorkspacePaneTypes.hub, id, resourceType: "knowledge" }, rooms);
    },
    [navigatePane, rooms, setSelectedHubResourceType, setSelectedKnowledgeBaseID],
  );

  const openCreateAgentModal = agent.openCreateAgentModal;
  const requireOpenCSGAuthentication = openCSGAuthGuard.requireAuthentication;
  const openCreateAgentFromTemplate = useCallback(
    (template: HubTemplate) => openCreateAgentModal(template),
    [openCreateAgentModal],
  );
  const selectModelProviderWithAuthentication = useCallback(
    (item: ModelProvider | null | undefined, options?: { requireAuthentication?: boolean }): boolean => {
      if (
        options?.requireAuthentication &&
        item?.id === MODEL_PROVIDER_IDS.OpenCSG &&
        !requireOpenCSGAuthentication()
      ) {
        return false;
      }
      selectModelProvider(item);
      return true;
    },
    [requireOpenCSGAuthentication, selectModelProvider],
  );

  const createMCPServerAndNavigate = useCallback(
    (payload: MCPServerPayload) =>
      saveMCPServerAndSelect(payload, hub.detailPaneProps.onCreateMCP, () => {
        if (hub.mcpCreateSource === "knowledge") {
          setSelectedKnowledgeBaseID("");
          navigatePane({ type: WorkspacePaneTypes.hub, id: "", resourceType: "knowledge" }, rooms, { replace: true });
        } else {
          navigatePane({ type: WorkspacePaneTypes.hub, id: "", resourceType: "mcp" }, rooms, { replace: true });
        }
      }),
    [hub.detailPaneProps.onCreateMCP, hub.mcpCreateSource, navigatePane, rooms, setSelectedKnowledgeBaseID],
  );

  useEffect(() => {
    if (activePane.type !== WorkspacePaneTypes.hub) {
      return;
    }
    if (activePane.resourceType === "template" && activePane.id) {
      setSelectedHubResourceType("template");
      setSelectedHubTemplateId(String(activePane.id));
      return;
    }
    if (activePane.resourceType === "skill" && activePane.id) {
      setSelectedHubResourceType("skill");
      setSelectedHubSkillName(String(activePane.id));
      return;
    }
    if (activePane.resourceType === "mcp" && activePane.id) {
      setSelectedHubResourceType("mcp");
      setSelectedMCPServerName(String(activePane.id));
      return;
    }
    if (activePane.resourceType === "knowledge") {
      setSelectedHubResourceType("knowledge");
      setSelectedKnowledgeBaseID(String(activePane.id || ""));
    }
  }, [
    activePane,
    setSelectedMCPServerName,
    setSelectedHubResourceType,
    setSelectedHubSkillName,
    setSelectedHubTemplateId,
    setSelectedKnowledgeBaseID,
  ]);

  const hubViewHub = useMemo(
    () =>
      bindWorkspaceHub({
        hub,
        onSelectTemplate: selectHubTemplate,
        onSelectSkill: selectHubSkill,
        onSelectMCP: selectMCPServer,
        onSelectKnowledgeBase: selectKnowledgeBase,
        onCreateMCP: createMCPServerAndNavigate,
        onOpenSkillUpload: () => setSkillUploadOpen(true),
        onRequireAuthentication: requireOpenCSGAuthentication,
      }),
    [
      createMCPServerAndNavigate,
      hub,
      requireOpenCSGAuthentication,
      selectKnowledgeBase,
      selectMCPServer,
      selectHubSkill,
      selectHubTemplate,
    ],
  );

  const hubResourceType =
    activePane.type === WorkspacePaneTypes.hub && activePane.resourceType
      ? activePane.resourceType
      : storedHubResourceType;
  const resourceLoadingText =
    hubResourceType === "skill"
      ? t("resourcesSkillsLoading")
      : hubResourceType === "mcp"
        ? t("resourcesMCPLoading")
        : hubResourceType === "knowledge"
          ? t("resourcesKnowledgeBasesLoading")
          : t("resourcesLoading");

  const taskEntry = useTaskEntry({
    currentUserID: displayData?.current_user_id ?? "",
    rooms,
    onUpdate: (room) => setBootstrapData((current) => mergeRoomMetadataUpdateInData(current, room)),
    onMessage: (roomID, message) => setBootstrapData((current) => appendMessageToData(current, roomID, message)),
    onDelete: (id) => {
      setBootstrapData((current) => removeConversationFromData(current, id));
      if (activePane.type === WorkspacePaneTypes.conversation && activePane.id === id) navigate("/new");
    },
    onOpen: (room) => selectConversation(room.id, { rooms: [room] }),
    isWorking: (id) => participantWork.workingParticipantsForRoom(id).length > 0,
  });

  if (!displayData) {
    return {
      ready: false,
      loadingText: loadingError || resourceLoadingText,
      activePane,
      configSettings: configSettings.settings,
      mainPanelHasThread: false,
      modelProviders,
      modelProvidersLoaded,
      refreshWorkspaceModelProviders,
      t,
    };
  }

  const agentViews = bindAgentViews({
    props: agent.agentViewProps,
    activeRoom: conversation.activeChannel,
    detailAgentID: conversationProfileDetailAgentID,
    onClose: closeConversationAgentDetail,
    onNewTask: (item) => {
      if (item?.id) navigate(pathForNewAgentTask(item.id));
    },
  });
  const mainPanelHasThread = Boolean(conversation.activeThreadRootID && conversation.selectedConversation);

  return {
    ready: true,
    taskEntry,
    openAgentSettings: agent.openEditAgentModal,
    openWorkspaceSettings: (section = "") =>
      navigate({ search: settingsDialogSearch(location.search, section) }, { replace: true }),
    configSettings: configSettings.settings,
    loadingText: "",
    t,
    shellClassName: shell.shellClassName,
    mainPanelHasThread,
    activePane,
    modelProviders,
    modelProvidersLoaded,
    refreshWorkspaceModelProviders,
    floatingChatProps: {
      avatar: floatingChatAvatar,
      avatarFallback: floatingChatAvatarFallback,
      chatProps: floatingChatConversationProps,
      locale,
      online: floatingChatAgent ? isAgentRunning(floatingChatAgent) : Boolean(floatingChatUser?.is_online),
      open: floatingChatOpen,
      t,
      title: floatingChatTitle,
      onOpenChange: setFloatingChatOpen,
    },
    sidebarProps: {
      taskEntry,
      isSidebarCollapsed,
      onCollapseSidebar: () => setIsSidebarCollapsed(true),
      onExpandSidebar: () => setIsSidebarCollapsed(false),
      skillUploadOpen,
      onSkillUploadOpenChange: setSkillUploadOpen,
      theme,
      onThemeChange: setTheme,
      turnNotificationMode,
      turnNotificationPermission,
      turnNotificationSound,
      onTurnNotificationSoundChange: setTurnNotificationSound,
      onRequestTurnNotificationPermission: requestTurnNotificationPermission,
      onTurnNotificationModeChange: changeTurnNotificationMode,
      locale,
      onLocaleChange: setLocale,
      t,
      currentWorkspaceLabel: shell.currentWorkspaceLabel,
      runningAgentCount: agent.runningAgentCount,
      showHubNewBadge: shell.showHubNewBadge,
      agentItems: agent.agentItems,
      modelProviders,
      modelProvidersLoaded,
      workerAgentItems: agent.workerAgentItems,
      notificationAgentItems: agent.notificationAgentItems,
      teams: agent.teams,
      workspaceTab: shell.workspaceTab,
      onWorkspaceTabChange: shell.selectWorkspaceTab,
      taskCount: task.rootTaskCount,
      scheduledTaskCount: task.scheduledTaskCount,
      activeTaskBoardView: task.taskBoardView,
      taskItems: task.tasks,
      planningTaskID: task.planningTaskID,
      startingTaskID: task.startingTaskID,
      roomCount: conversation.roomCount,
      threadCount: conversation.threadCount,
      channels: conversation.channels,
      directMessages: conversation.directMessages,
      threadGroups: conversation.threadGroups,
      activePane,
      activeThreadRootID: conversation.activeThreadRootID,
      currentUserID: displayData.current_user_id ?? "",
      usersById: conversation.usersById,
      collapsedWorkspaceGroups,
      showUpgradeControls: bootstrapConfig?.show_upgrade !== false,
      onToggleWorkspaceGroup: shell.toggleWorkspaceGroup,
      onCreateRoom: () => navigate("/new"),
      onCreateAgent: agent.openCreateAgentModal,
      onCreateCustomAgent: agent.openCreateCustomAgentModal,
      onCreateModelProvider: modelProviderCreation.open,
      onCreateNotificationParticipant: agent.openCreateNotificationParticipantModal,
      onCreateTeam: async (payload: CreateTeamPayload) => {
        await agent.agentViewProps.onCreateTeam?.(payload);
      },
      teamActionBusy: agent.agentViewProps.teamActionBusy,
      teamActionError: agent.agentViewProps.teamActionError,
      onOpenCreateTeam: agent.openCreateTeamModal,
      hub,
      onSelectMCPServer: selectMCPServer,
      onSelectKnowledgeBase: selectKnowledgeBase,
      onSelectHubSkill: selectHubSkill,
      onSelectHubTemplate: selectHubTemplate,
      onSelectHub: () => shell.selectWorkspaceTab(WorkspaceTabs.hub),
      onSelectNotificationSection: selectNotificationSection,
      onSelectTeamSection: selectTeamSection,
      onSelectTask: selectTasks,
      onSelectTaskBoardView: task.setTaskBoardView,
      onOpenCreateTask: () => navigate("/new"),
      onOpenCreateScheduledTask: task.openCreateScheduledTaskModal,
      onViewTaskDetails: task.openParentTaskDetail,
      onSelectTeam: selectTeam,
      agentsError: agent.agentsDisplayError,
      onSelectConversation: selectConversationAndCloseSidePanels,
      onSelectSearchMatch: (id, message) => {
        if (!closeConversationAgentDetail()) return;
        selectConversationAndCloseThreadPanel(id);
        conversation.navigateToSearchMatch(id, message);
      },
      onSelectThread: openThreadInConversation,
      onPreviewUser: profilePreview.openParticipantPreview,
      onSelectAgent: selectAgent,
      onSelectModelProvider: selectModelProviderWithAuthentication,
      onSelectApps: selectApps,
      onRequireOpenCSGAuthentication: requireOpenCSGAuthentication,
      onSelectHuman: selectHuman,
      onSelectComputer: selectComputer,
      appVersion,
      upgradeStatus,
      upgradeBusy: upgrade.upgradeBusy,
      upgradeChannelBusy: upgrade.upgradeChannelBusy,
      upgradeChannelError: upgrade.upgradeChannelError,
      upgradeChannelLocked: upgrade.upgradeChannelLocked,
      upgradePhase: upgrade.upgradePhase,
      upgradeError: upgrade.upgradeError,
      suppressUpgradeIssue: upgrade.showUpgradeModal,
      onOpenUpgrade: upgrade.openUpgradeModal,
      onRefreshUpgradeStatus: upgrade.refreshUpgradeStatus,
      onUpgradeChannelChange: upgrade.changeUpgradeChannel,
      onOpenConfigSettings: configSettings.openConfigModal,
      onOpenSettings: selectSettings,
      authStatus: auth.status,
      authExpired: auth.status.authenticated === true && !openCSGAuthGuard.authenticated,
      authEnvironment: auth.environment,
      authBusy: auth.busy,
      authLoggingOut: auth.loggingOut,
      authPending: auth.pending,
      authError: auth.error,
      onLogin: auth.login,
      onLogout: auth.logout,
      onAuthEnvironmentChange: auth.setEnvironment,
    },
    authNotice: auth.notice,
    onDismissAuthNotice: auth.dismissNotice,
    openCSGLoginRequiredDialogProps: openCSGAuthGuard.dialogOpen
      ? {
          busy: auth.busy || auth.pending,
          environment: auth.environment,
          error: auth.error,
          onConnect: openCSGAuthGuard.login,
          onOpenChange: (open: boolean) => {
            if (!open) {
              openCSGAuthGuard.closeDialog();
            }
          },
          open: true,
          t,
          variant: "authentication-required" as const,
        }
      : null,
    hubViewProps: {
      templateUsername: openCSGAuthGuard.authenticated ? auth.status.user_id : "",
      onTemplateLogin: () => {
        openCSGAuthGuard.requireAuthentication();
      },
      agenticHubSiteURL: openCSGAuthGuard.authenticated ? auth.status.opencsg_base_url || auth.status.base_url : "",
      t,
      locale,
      hub: hubViewHub,
      onSelectTemplateInPlace: (item: HubTemplate) => {
        setSelectedHubResourceType("template");
        setSelectedHubTemplateId(item.id || "");
      },
      onCreateFromTemplate: openCreateAgentFromTemplate,
    },
    agentViewProps: agentViews.agentViewProps,
    computerViewProps: {
      ...agent.computerViewProps,
      channels: conversation.channels,
      directMessages: conversation.directMessages,
      onSelectAgent: selectAgent,
    },
    humanViewProps: {
      t,
      locale,
      avatarBusy: humanAvatarBusy,
      avatarError: selectedHuman ? humanAvatarError : "",
      descriptionBusy: humanDescriptionBusy,
      descriptionError: selectedHuman ? humanDescriptionError : "",
      user: selectedHuman,
      onAvatarChange: updateHumanAvatar,
      onDescriptionSave: updateHumanDescription,
    },
    conversationViewProps: {
      ...conversation.conversationViewProps,
      memberPanelProps: conversation.conversationViewProps.memberPanelProps
        ? {
            ...conversation.conversationViewProps.memberPanelProps,
            onPreviewUser: profilePreview.openParticipantPreview,
            onManageExternalMessages: () =>
              navigate({ search: settingsDialogSearch(location.search, "external") }, { replace: true }),
          }
        : null,
      onNewTask: (agentID?: string, configuration?: TaskConfiguration) =>
        navigate(agentID ? `/new?agent=${encodeURIComponent(agentID)}` : "/new", { state: { configuration } }),
      agents: agent.agentItems,
      onOpenAgentDetail: openConversationAgentDetailFromAvatar,
      onOpenThread: openConversationThread,
      onPreviewUser: profilePreview.openParticipantPreview,
      agentDetailPanelProps: agentViews.detailPanelProps,
    },
    taskViewProps: {
      ...task.taskViewProps,
      onOpenCreateTaskModal: () => navigate("/new"),
    },
    teamViewProps: {
      t,
      team: selectedTeam,
      teamsLoading: agent.teamsLoading,
      agents,
      usersById: conversation.usersById,
      tasks: selectedTeamTasks,
      teamActionBusy: agent.agentViewProps.teamActionBusy,
      teamActionError: agent.agentViewProps.teamActionError,
      onStartCollaboration: (team) => navigate(pathForNewTeamTask(team.id)),
      onManageMembers: agent.openManageTeamMembers,
      onDeleteTeam: agent.deleteTeam,
      onSelectAgent: selectAgent,
      onSelectTask: selectTasks,
    },
    profilePreviewProps: profilePreview.profilePreviewProps,
    createTeamModalProps: agent.createTeamModalProps,
    upgradeModalProps: upgrade.upgradeModalProps,
    configModalProps: configSettings.configModalProps,
    createModelProviderModalProps: modelProviderCreation.modalProps,
    agentProfileModalProps: agent.agentProfileModalProps,
  };
}
