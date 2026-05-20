import { useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { createTranslator } from "@/shared/i18n";
import { paneFromLocation, workspaceTabForPane } from "@/models/routing";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useWorkspaceData } from "./useWorkspaceData";
import { useWorkspaceNavigation } from "./useWorkspaceNavigation";
import { useWorkspaceShellController } from "./useWorkspaceShellController";
import { useWorkspaceHubController } from "./useWorkspaceHubController";
import { useUpgradeController } from "./useUpgradeController";
import { useAgentController } from "./useAgentController";
import { useConversationController } from "./useConversationController";
import { useProfilePreviewController } from "./useProfilePreviewController";
import type { HubTemplate } from "@/models/hubWorkspace";

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
  const collapsedWorkspaceGroups = useWorkspaceUiStore((state) => state.collapsedWorkspaceGroups);
  const setCollapsedWorkspaceGroups = useWorkspaceUiStore((state) => state.setCollapsedWorkspaceGroups);
  const activeConversationId = useWorkspaceUiStore((state) => state.activeConversationId);
  const setActiveConversationId = useWorkspaceUiStore((state) => state.setActiveConversationId);
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
  const t = useMemo(() => createTranslator(locale), [locale]);
  const activePane = useMemo(() => paneFromLocation(location.pathname), [location.pathname]);
  const workspaceTab = useMemo(() => workspaceTabForPane(activePane), [activePane]);
  const rooms = useMemo(() => data?.rooms ?? [], [data]);
  const loadingError = bootstrapQuery.isError ? t("loadingFailed") : "";
  const { navigatePane, selectConversation, selectAgent, selectComputer, selectHub } = useWorkspaceNavigation({
    location,
    navigate,
    dataReady: Boolean(data),
    setActiveConversationId,
    rooms,
  });
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
    setCollapsedWorkspaceGroups,
    t,
    theme,
    workspaceTab,
  });
  const { hub, refreshHubTemplates } = useWorkspaceHubController({
    hubLoaded,
    hubTemplates,
    hubTemplatesQuery,
    refreshWorkspaceHubTemplates,
    t,
  });
  const { setSelectedHubTemplateId } = hub;
  const upgrade = useUpgradeController({
    appVersion,
    refreshWorkspaceAppVersion,
    refreshWorkspaceUpgradeStatus,
    setAppVersionData,
    setUpgradeStatusData,
    t,
    upgradeStatus,
  });
  const agent = useAgentController({
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
    selectComputer,
    selectConversation,
    selectHub,
    setManagerProfileData,
    setSelectedHubTemplateId,
    t,
  });
  const conversation = useConversationController({
    activeConversationId,
    activePane,
    authBusyProvider: agent.cliproxyAuthBusy,
    authStatuses: agent.cliproxyAuthStatuses,
    data,
    locale,
    managerProfile,
    managerProfileIncomplete: agent.managerProfileIncomplete,
    messageActionBusy: agent.messageActionBusy,
    messageActionError: agent.messageActionError,
    navigatePane,
    onMessageAction: agent.handleMessageAction,
    onProviderLogin: agent.loginCLIProxyProvider,
    onUpgradeStatusChange: upgrade.handleUpgradeStatusChange,
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
  const profilePreview = useProfilePreviewController({
    agentActionBusy: agent.agentActionBusy,
    agentItems: agent.agentItems,
    closeConversationTools: conversation.closeConversationTools,
    deletePreviewBot: agent.deletePreviewBot,
    openAgentDirectMessage: agent.openAgentDirectMessage,
    selectedConversation: conversation.selectedConversation,
    selectAgent,
    t,
    usersById: conversation.usersById,
  });

  function selectHubTemplate(item: HubTemplate | null | undefined) {
    if (!item?.id) {
      selectHub();
      return;
    }
    setSelectedHubTemplateId(item.id);
    selectHub();
  }

  if (!data) {
    return {
      ready: false,
      loadingText: loadingError || t("loading"),
    };
  }

  return {
    ready: true,
    loadingText: "",
    shellClassName: shell.shellClassName,
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
      currentWorkspaceLabel: shell.currentWorkspaceLabel,
      runningAgentCount: agent.runningAgentCount,
      agentItems: agent.agentItems,
      workspaceTab: shell.workspaceTab,
      onWorkspaceTabChange: shell.selectWorkspaceTab,
      roomCount: conversation.roomCount,
      channels: conversation.channels,
      directMessages: conversation.directMessages,
      activePane,
      currentUserID: data.current_user_id,
      usersById: conversation.usersById,
      collapsedWorkspaceGroups,
      onToggleWorkspaceGroup: shell.toggleWorkspaceGroup,
      onCreateRoom: () => conversation.openCreateRoomModal(),
      onCreateAgent: agent.openCreateAgentModal,
      hub,
      onSelectHubTemplate: selectHubTemplate,
      onSelectHub: selectHub,
      agentsError: agent.agentsDisplayError,
      onSelectConversation: selectConversation,
      onPreviewUser: profilePreview.openParticipantPreview,
      onSelectAgent: selectAgent,
      onPreviewAgent: profilePreview.openAgentPreview,
      onSelectComputer: selectComputer,
      appVersion,
      upgradeStatus,
      upgradeBusy: upgrade.upgradeBusy,
      upgradePhase: upgrade.upgradePhase,
      upgradeError: upgrade.upgradeError,
      onOpenUpgrade: upgrade.openUpgradeModal,
    },
    hubViewProps: {
      t,
      locale,
      hub,
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
    conversationViewProps: {
      ...conversation.conversationViewProps,
      onPreviewUser: profilePreview.openParticipantPreview,
    },
    profilePreviewProps: profilePreview.profilePreviewProps,
    createRoomModalProps: conversation.createRoomModalProps,
    inviteMembersModalProps: conversation.inviteMembersModalProps,
    upgradeModalProps: upgrade.upgradeModalProps,
    agentProfileModalProps: agent.agentProfileModalProps,
    managerRebuildModalProps: agent.managerRebuildModalProps,
    managerProfileSetupModalProps: agent.managerProfileSetupModalProps,
  };
}
