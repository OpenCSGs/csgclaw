import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { errorMessage } from "@/api/client";
import { checkModelProvider, createModelProvider, type ModelProviderPayload } from "@/api/modelProviders";
import { patchCsgclawUserRequest } from "@/api/participants";
import { isAuthenticated } from "@/models/auth";
import { createTranslator } from "@/shared/i18n";
import {
  agentMatchesUser,
  isDirectConversation,
  resolveConversationUser,
  upsertUserInData,
} from "@/models/conversations";
import { isAgentRunning, resolveAgentAvatarFallback, resolveAgentChannelUserID } from "@/models/agents";
import { MANAGER_AGENT_ID, MANAGER_AGENT_NAME, MANAGER_PARTICIPANT_ID } from "@/shared/constants/agents";
import { WorkspacePaneTypes, WorkspaceTabs, paneFromLocation } from "@/models/routing";
import { modelProviderCatalogForOpenCSGState } from "@/models/modelProviders";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useWorkspaceData } from "./useWorkspaceData";
import { useWorkspaceNavigation } from "./useWorkspaceNavigation";
import { useWorkspaceShellController } from "./useWorkspaceShellController";
import { useWorkspaceHubController } from "./useWorkspaceHubController";
import { useUpgradeController } from "./useUpgradeController";
import { useConfigController } from "./useConfigController";
import { useAuthController } from "./useAuthController";
import { useConnectorController } from "./useConnectorController";
import { useAgentController } from "./useAgentController";
import { useConversationController } from "./useConversationController";
import { useProfilePreviewController } from "./useProfilePreviewController";
import { useTaskController } from "./useTaskController";
import { useWorkspaceRealtime } from "./useWorkspaceRealtime";
import { useParticipantWorkStatus } from "./useParticipantWorkStatus";
import type { CreateTeamPayload } from "@/api/tasks";
import type { AgentLike } from "@/models/agents";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { MCPServer } from "@/models/mcp";
import type { IMConversation, IMData, IMUser } from "@/models/conversations";
import type { SkillSummary } from "@/models/skillhub";

function isBootstrapAdminUser(user: IMUser | null | undefined) {
  return user?.id === "u-admin" || String(user?.name ?? "").toLowerCase() === "admin";
}

function initialsForIdentity(name: string) {
  const trimmed = name.trim();
  if (!trimmed) {
    return "ME";
  }
  const parts = trimmed.split(/\s+/).filter(Boolean);
  if (parts.length > 1) {
    return `${parts[0]?.[0] ?? ""}${parts[1]?.[0] ?? ""}`.toUpperCase();
  }
  return trimmed.slice(0, 2).toUpperCase();
}

function resolveManagerDirectConversation(
  rooms: readonly IMConversation[],
  currentUserID: string,
  managerAgent: AgentLike | null | undefined,
): IMConversation | null {
  if (!currentUserID) {
    return null;
  }
  const managerUserIDs = new Set(
    [
      resolveAgentChannelUserID(managerAgent),
      managerAgent?.id,
      managerAgent?.user_id,
      MANAGER_PARTICIPANT_ID,
      MANAGER_AGENT_ID,
    ]
      .map((value) => String(value ?? "").trim())
      .filter(Boolean),
  );
  return (
    rooms.find(
      (room) =>
        isDirectConversation(room) &&
        room.members.includes(currentUserID) &&
        room.members.some((memberID) => memberID !== currentUserID && managerUserIDs.has(memberID)),
    ) ?? null
  );
}

function resolveDirectConversationAgent(
  conversation: IMConversation | null | undefined,
  currentUserID: string,
  usersById: Map<string, IMUser>,
  agents: readonly AgentLike[],
): AgentLike | null {
  if (!conversation || !currentUserID || !isDirectConversation(conversation)) {
    return null;
  }
  const otherMemberID = conversation.members.find((id) => id && id !== currentUserID) ?? "";
  const directUser = resolveConversationUser(conversation, currentUserID, usersById);
  return (
    agents.find((item) => {
      const channelUserID = resolveAgentChannelUserID(item);
      return (
        Boolean(otherMemberID && (item.id === otherMemberID || item.user_id === otherMemberID)) ||
        Boolean(channelUserID && channelUserID === otherMemberID) ||
        agentMatchesUser(item, directUser)
      );
    }) ?? null
  );
}

function withLocalIdentity(data: IMData | null, fallbackName: string): IMData | null {
  if (!data?.current_user_id) {
    return data;
  }

  const currentUser = data.users.find((user) => user.id === data.current_user_id);
  if (!currentUser) {
    return data;
  }

  const displayName = isBootstrapAdminUser(currentUser) ? fallbackName : currentUser.name;
  if (!displayName || displayName === currentUser.name) {
    return data;
  }

  return {
    ...data,
    users: data.users.map((user) =>
      user.id === data.current_user_id
        ? {
            ...user,
            avatar: user.avatar || initialsForIdentity(displayName),
            name: displayName,
          }
        : user,
    ),
  };
}

export function useWorkspaceController() {
  const location = useLocation();
  const navigate = useNavigate();
  const locale = useWorkspaceUiStore((state) => state.locale);
  const setLocale = useWorkspaceUiStore((state) => state.setLocale);
  const theme = useWorkspaceUiStore((state) => state.theme);
  const setTheme = useWorkspaceUiStore((state) => state.setTheme);
  const showToolCalls = useWorkspaceUiStore((state) => state.showToolCalls);
  const setShowToolCalls = useWorkspaceUiStore((state) => state.setShowToolCalls);
  const floatingChatOpen = useWorkspaceUiStore((state) => state.floatingChatOpen);
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
  const displayData = useMemo(() => withLocalIdentity(data, t("localIdentityFallback")), [data, t]);
  const activePane = useMemo(() => paneFromLocation(location.pathname), [location.pathname]);
  const rooms = useMemo(() => displayData?.rooms ?? [], [displayData]);
  const [conversationProfileDetailAgentID, setConversationProfileDetailAgentID] = useState("");
  const conversationProfileDetailTriggerRef = useRef<HTMLElement | null>(null);
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
    theme,
    workspaceTab,
  });
  const auth = useAuthController(t);
  const modelProviders = useMemo(
    () =>
      modelProviderCatalogForOpenCSGState(rawModelProviders, {
        aiGatewayBaseURL: auth.environment.aiGatewayBaseURL,
        authenticated: isAuthenticated(auth.status),
      }),
    [auth.environment.aiGatewayBaseURL, auth.status, rawModelProviders],
  );
  const connectors = useConnectorController(t);
  const { hub, refreshHubTemplates } = useWorkspaceHubController({
    hubLoaded,
    hubTemplates,
    hubTemplatesQuery,
    refreshWorkspaceHubTemplates,
    t,
  });
  const { setSelectedMCPServerName, setSelectedHubResourceType, setSelectedHubSkillName, setSelectedHubTemplateId } =
    hub;
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
  const agent = useAgentController({
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
    profileDetailAgentID: conversationProfileDetailAgentID,
    refreshMCPServers: hub.refetchMCPServers,
    refreshHubTemplates,
    refreshWorkspaceAgents,
    refreshWorkspaceModelProviders,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceManagerProfile,
    rooms,
    selectAgent,
    selectComputer,
    selectConversation,
    selectHub,
    selectModelProvider,
    setAgentsData,
    setBootstrapData,
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
  const participantWork = useParticipantWorkStatus({ agents, users: displayData?.users ?? [] });
  const conversation = useConversationController({
    activeConversationId,
    activePane,
    agents,
    authBusyProvider: agent.cliproxyAuthBusy,
    authStatuses: agent.cliproxyAuthStatuses,
    connectorStatus: connectors.github,
    gitlabConnectorStatus: connectors.gitlab,
    connectorBusyAction: connectors.busyAction,
    connectorBusyProvider: connectors.busyProvider,
    connectorError: connectors.error,
    connectorPending: connectors.pending,
    onSaveConnectorConfig: connectors.saveGitHubConfig,
    onConnectConnector: connectors.connectGitHub,
    onDisconnectConnector: connectors.disconnectGitHub,
    onDisconnectGitLabConnector: connectors.disconnectGitLab,
    onManageConnector: connectors.manageGitHub,
    onSaveGitLabConnectorConfig: connectors.saveGitLabConfig,
    data: displayData,
    locale,
    managerProfile,
    managerProfileIncomplete: agent.managerProfileIncomplete,
    managerRuntimeUnavailable: agent.managerRuntimeUnavailable,
    managerRuntimeWarning: agent.managerRuntimeWarning,
    messageActionBusy: agent.messageActionBusy,
    messageActionFeedback: agent.messageActionFeedback,
    hasObservedWorkLease: participantWork.hasObservedWorkLease,
    workingParticipantsForRoom: participantWork.workingParticipantsForRoom,
    navigatePane,
    onMessageAction: agent.handleMessageAction,
    onProviderLogin: agent.loginCLIProxyProvider,
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
    connectorStatus: connectors.github,
    gitlabConnectorStatus: connectors.gitlab,
    connectorBusyAction: connectors.busyAction,
    connectorBusyProvider: connectors.busyProvider,
    connectorError: connectors.error,
    connectorPending: connectors.pending,
    onSaveConnectorConfig: connectors.saveGitHubConfig,
    onConnectConnector: connectors.connectGitHub,
    onDisconnectConnector: connectors.disconnectGitHub,
    onDisconnectGitLabConnector: connectors.disconnectGitLab,
    onManageConnector: connectors.manageGitHub,
    onSaveGitLabConnectorConfig: connectors.saveGitLabConfig,
    data: displayData,
    locale,
    managerProfile,
    managerProfileIncomplete: agent.managerProfileIncomplete,
    managerRuntimeUnavailable: agent.managerRuntimeUnavailable,
    managerRuntimeWarning: agent.managerRuntimeWarning,
    messageActionBusy: agent.messageActionBusy,
    messageActionFeedback: agent.messageActionFeedback,
    hasObservedWorkLease: participantWork.hasObservedWorkLease,
    workingParticipantsForRoom: participantWork.workingParticipantsForRoom,
    messageListActive: floatingChatOpen,
    navigatePane: ignoreFloatingChatNavigation,
    onMessageAction: agent.handleMessageAction,
    onProviderLogin: agent.loginCLIProxyProvider,
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
  const selectedHumanID = activePane.type === WorkspacePaneTypes.human ? String(activePane.id || "") : "";
  const selectedHuman = selectedHumanID ? (conversation.usersById.get(selectedHumanID) ?? null) : null;
  const [showCreateModelProviderModal, setShowCreateModelProviderModal] = useState(false);
  const [createModelProviderBusy, setCreateModelProviderBusy] = useState(false);
  const [createModelProviderError, setCreateModelProviderError] = useState("");
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
        onCancelProfilePreviewClose: profilePreview.cancelProfilePreviewClose,
        onCloseProfilePreview: profilePreview.scheduleProfilePreviewClose,
        onPreviewUser: profilePreview.showParticipantPreview,
        showInviteAction: false,
        threadDisplay: "dialog" as const,
      }
    : null;

  const selectHubTemplate = useCallback(
    (item: HubTemplate | null | undefined) => {
      if (!item?.id) {
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
        selectHub();
        return;
      }
      setSelectedHubResourceType("mcp");
      setSelectedMCPServerName(item.name);
      navigatePane({ type: WorkspacePaneTypes.hub, id: item.name, resourceType: "mcp" }, rooms);
    },
    [navigatePane, rooms, selectHub, setSelectedMCPServerName, setSelectedHubResourceType],
  );

  function openCreateModelProviderModal() {
    setCreateModelProviderError("");
    setShowCreateModelProviderModal(true);
  }

  async function createOpenAIModelProvider(payload: ModelProviderPayload) {
    if (createModelProviderBusy) {
      return;
    }
    setCreateModelProviderBusy(true);
    setCreateModelProviderError("");
    try {
      const created = await createModelProvider(payload);
      if (created.id && payload.base_url && (payload.api_key || created.api_key_set)) {
        try {
          await checkModelProvider(created.id, payload);
        } catch (_) {
          // The provider was saved; the detail page can still surface a later manual check failure.
        }
      }
      const refreshed = await refreshWorkspaceModelProviders();
      const next = refreshed?.providers.find((provider) => provider.id === created.id) ?? created;
      selectModelProvider(next);
      setShowCreateModelProviderModal(false);
    } catch (_) {
      setCreateModelProviderError(t("modelProviderCreateFailed"));
    } finally {
      setCreateModelProviderBusy(false);
    }
  }

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
    }
  }, [
    activePane,
    setSelectedMCPServerName,
    setSelectedHubResourceType,
    setSelectedHubSkillName,
    setSelectedHubTemplateId,
  ]);

  const hubViewHub = useMemo(
    () => ({
      ...hub,
      detailPaneProps: {
        ...hub.detailPaneProps,
        onSelectTemplate: selectHubTemplate,
        onSelectSkill: (name: string | null | undefined) =>
          selectHubSkill(name ? ({ name, description: "" } as SkillSummary) : null),
        onSelectMCP: (name: string | null | undefined) =>
          selectMCPServer(name ? ({ name, config: {} } as MCPServer) : null),
      },
    }),
    [hub, selectMCPServer, selectHubSkill, selectHubTemplate],
  );

  if (!displayData) {
    return {
      ready: false,
      loadingText: loadingError || t("loading"),
      activePane,
      mainPanelHasThread: false,
      modelProviders,
      modelProvidersLoaded,
      refreshWorkspaceModelProviders,
      t,
    };
  }

  const conversationAgentDetailPanelProps =
    conversationProfileDetailAgentID && agent.agentViewProps.item?.id === conversationProfileDetailAgentID
      ? {
          ...agent.agentViewProps,
          activeRoom: conversation.activeChannel,
          item: agent.agentViewProps.item,
          onClose: closeConversationAgentDetail,
        }
      : null;
  const mainPanelHasThread = Boolean(conversation.activeThreadRootID && conversation.selectedConversation);

  return {
    ready: true,
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
      isSidebarCollapsed,
      onCollapseSidebar: () => setIsSidebarCollapsed(true),
      onExpandSidebar: () => setIsSidebarCollapsed(false),
      theme,
      onThemeChange: setTheme,
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
      onCreateRoom: () => conversation.openCreateRoomModal(),
      onCreateAgent: agent.openCreateAgentModal,
      onCreateModelProvider: openCreateModelProviderModal,
      onCreateNotificationParticipant: agent.openCreateNotificationParticipantModal,
      onCreateTeam: async (payload: CreateTeamPayload) => {
        await agent.agentViewProps.onCreateTeam?.(payload);
      },
      teamActionBusy: agent.agentViewProps.teamActionBusy,
      teamActionError: agent.agentViewProps.teamActionError,
      onOpenCreateTeam: agent.openCreateTeamModal,
      hub,
      onSelectMCPServer: selectMCPServer,
      onSelectHubSkill: selectHubSkill,
      onSelectHubTemplate: selectHubTemplate,
      onSelectHub: () => shell.selectWorkspaceTab(WorkspaceTabs.hub),
      onSelectNotificationSection: selectNotificationSection,
      onSelectTeamSection: selectTeamSection,
      onSelectTask: selectTasks,
      onSelectTaskBoardView: task.setTaskBoardView,
      onOpenCreateTask: task.openCreateTaskModal,
      onOpenCreateScheduledTask: task.openCreateScheduledTaskModal,
      onViewTaskDetails: task.openParentTaskDetail,
      onSelectTeam: selectTeam,
      agentsError: agent.agentsDisplayError,
      onSelectConversation: selectConversationAndCloseSidePanels,
      onSelectThread: openThreadInConversation,
      onPreviewUser: profilePreview.openParticipantPreview,
      onSelectAgent: selectAgent,
      onSelectModelProvider: selectModelProvider,
      onSelectHuman: selectHuman,
      onPreviewAgent: profilePreview.openAgentPreview,
      onSelectComputer: selectComputer,
      appVersion,
      upgradeStatus,
      upgradeBusy: upgrade.upgradeBusy,
      upgradePhase: upgrade.upgradePhase,
      upgradeError: upgrade.upgradeError,
      suppressUpgradeIssue: upgrade.showUpgradeModal,
      onOpenUpgrade: upgrade.openUpgradeModal,
      onOpenConfigSettings: configSettings.openConfigModal,
      onOpenSettings: selectSettings,
      authStatus: auth.status,
      authEnvironment: auth.environment,
      authBusy: auth.busy,
      authPending: auth.pending,
      authError: auth.error,
      onLogin: auth.login,
      onLogout: auth.logout,
      onAuthEnvironmentChange: auth.setEnvironment,
    },
    authNotice: auth.notice,
    onDismissAuthNotice: auth.dismissNotice,
    hubViewProps: {
      t,
      locale,
      hub: hubViewHub,
      onCreateFromTemplate: agent.openCreateAgentModal,
    },
    agentViewProps: {
      ...agent.agentViewProps,
      activeRoom: conversation.activeChannel,
    },
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
      agents: agent.agentItems,
      onCancelProfilePreviewClose: profilePreview.cancelProfilePreviewClose,
      onCloseProfilePreview: profilePreview.scheduleProfilePreviewClose,
      onOpenAgentDetail: openConversationAgentDetailFromAvatar,
      onOpenThread: openConversationThread,
      onPreviewUser: profilePreview.showParticipantPreview,
      agentDetailPanelProps: conversationAgentDetailPanelProps,
    },
    taskViewProps: task.taskViewProps,
    teamViewProps: {
      t,
      team: selectedTeam,
      teamsLoading: agent.teamsLoading,
      agents,
      usersById: conversation.usersById,
      tasks: selectedTeamTasks,
      teamActionBusy: agent.agentViewProps.teamActionBusy,
      teamActionError: agent.agentViewProps.teamActionError,
      onManageMembers: agent.openManageTeamMembers,
      onDeleteTeam: agent.deleteTeam,
      onSelectAgent: selectAgent,
      onSelectTask: selectTasks,
    },
    profilePreviewProps: profilePreview.profilePreviewProps,
    createRoomModalProps: conversation.createRoomModalProps,
    createTeamModalProps: agent.createTeamModalProps,
    inviteMembersModalProps: conversation.inviteMembersModalProps,
    upgradeModalProps: upgrade.upgradeModalProps,
    configModalProps: configSettings.configModalProps,
    createModelProviderModalProps: showCreateModelProviderModal
      ? {
          busy: createModelProviderBusy,
          error: createModelProviderError,
          modelProviders,
          onClose: () => {
            if (!createModelProviderBusy) {
              setShowCreateModelProviderModal(false);
            }
          },
          onCreate: createOpenAIModelProvider,
          onCheckAccess: (payload: ModelProviderPayload) => checkModelProvider("openai-draft", payload),
          t,
        }
      : null,
    agentProfileModalProps: agent.agentProfileModalProps,
  };
}
