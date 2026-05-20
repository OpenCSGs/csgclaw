import { HubIcon } from "@/components/ui/Icons";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { WorkspaceAgentRow, WorkspaceComputerRow, WorkspaceConversationRow, WorkspaceGroup } from "../WorkspaceRows";

export function WorkspaceTabPanels({
  workspaceTab,
  channels,
  directMessages,
  activePane,
  currentUserID,
  usersById,
  locale,
  t,
  collapsedWorkspaceGroups,
  onToggleWorkspaceGroup,
  onCreateRoom,
  onCreateAgent,
  onCreateNotificationBot,
  hub,
  onSelectHubTemplate,
  agentsError,
  onSelectConversation,
  onPreviewUser,
  agentItems,
  workerAgentItems = agentItems,
  notificationAgentItems = [],
  onSelectAgent,
  onPreviewAgent,
  onSelectComputer,
}) {
  const hubTemplates = hub?.templates ?? [];
  const hubError = hub?.listError ?? "";
  const hubLoaded = hub?.loaded ?? false;
  const selectedHubTemplateId = hub?.selectedHubTemplateId ?? "";

  return (
    <>
      {workspaceTab === WorkspaceTabs.messages ? (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("messagesTab")}>
          <WorkspaceGroup
            id="rooms"
            title={t("channelsSection")}
            count={channels.length}
            collapsed={Boolean(collapsedWorkspaceGroups.rooms)}
            onToggle={() => onToggleWorkspaceGroup("rooms")}
            onAdd={() => onCreateRoom()}
            addLabel={t("createRoom")}
          >
            {channels.length ? (
              channels.map((conversation) => (
                <WorkspaceConversationRow
                  key={conversation.id}
                  conversation={conversation}
                  active={activePane.type === WorkspacePaneTypes.conversation && activePane.id === conversation.id}
                  currentUserID={currentUserID}
                  usersById={usersById}
                  locale={locale}
                  t={t}
                  onSelect={onSelectConversation}
                  onPreviewUser={onPreviewUser}
                />
              ))
            ) : (
              <div className="workspace-empty">{t("noChannels")}</div>
            )}
          </WorkspaceGroup>
          <WorkspaceGroup
            id="direct-messages"
            title={t("directMessagesSection")}
            count={directMessages.length}
            collapsed={Boolean(collapsedWorkspaceGroups["direct-messages"])}
            onToggle={() => onToggleWorkspaceGroup("direct-messages")}
          >
            {directMessages.length ? (
              directMessages.map((conversation) => (
                <WorkspaceConversationRow
                  key={conversation.id}
                  conversation={conversation}
                  active={activePane.type === WorkspacePaneTypes.conversation && activePane.id === conversation.id}
                  currentUserID={currentUserID}
                  usersById={usersById}
                  locale={locale}
                  t={t}
                  onSelect={onSelectConversation}
                  onPreviewUser={onPreviewUser}
                />
              ))
            ) : (
              <div className="workspace-empty">{t("noDirectMessages")}</div>
            )}
          </WorkspaceGroup>
        </div>
      ) : workspaceTab === WorkspaceTabs.hub ? (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("hubTab")}>
          <WorkspaceGroup
            id="hub"
            title={t("hubTemplatesSection")}
            count={hubTemplates.length}
            collapsed={Boolean(collapsedWorkspaceGroups.hub)}
            onToggle={() => onToggleWorkspaceGroup("hub")}
          >
            {hubError ? (
              <div className="workspace-empty">{hubError}</div>
            ) : hubLoaded && hubTemplates.length === 0 ? (
              <div className="workspace-empty">{t("hubEmpty")}</div>
            ) : (
              hubTemplates.slice(0, 6).map((item) => (
                <button
                  key={item.id}
                  className={`workspace-row hub-template-row ${selectedHubTemplateId === item.id ? "active" : ""}`}
                  onClick={() => onSelectHubTemplate(item)}
                >
                  <span className="workspace-row-icon">
                    <HubIcon />
                  </span>
                  <span className="workspace-row-main">
                    <span className="workspace-row-title truncate">{item.name || item.id}</span>
                    <span className="workspace-row-meta truncate">
                      {item.description || item.source?.name || item.id}
                    </span>
                  </span>
                  <span className="mini-badge template-source-badge">
                    <span className="template-source-badge-dot" aria-hidden="true"></span>
                    {localizeTemplateSourceTag(item.source?.name, locale)}
                  </span>
                </button>
              ))
            )}
          </WorkspaceGroup>
        </div>
      ) : (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("agentsTab")}>
          <WorkspaceGroup
            id="agents"
            title={t("computerAgentsSection")}
            count={workerAgentItems.length}
            collapsed={Boolean(collapsedWorkspaceGroups.agents)}
            onToggle={() => onToggleWorkspaceGroup("agents")}
            onAdd={onCreateAgent}
            addLabel={t("createAgent")}
          >
            {workerAgentItems.length ? (
              workerAgentItems.map((item) => (
                <WorkspaceAgentRow
                  key={item.id}
                  item={item}
                  active={activePane.type === WorkspacePaneTypes.agent && activePane.id === item.id}
                  t={t}
                  onSelect={onSelectAgent}
                  onPreview={onPreviewAgent}
                />
              ))
            ) : (
              <div className="workspace-empty">{t("noAgents")}</div>
            )}
          </WorkspaceGroup>
          <WorkspaceGroup
            id="notifications"
            title={t("notificationsSection")}
            count={notificationAgentItems.length}
            collapsed={Boolean(collapsedWorkspaceGroups.notifications)}
            onToggle={() => onToggleWorkspaceGroup("notifications")}
            onAdd={onCreateNotificationBot}
            addLabel={t("createNotificationBot")}
          >
            {notificationAgentItems.length ? (
              notificationAgentItems.map((item) => (
                <WorkspaceAgentRow
                  key={item.id}
                  item={item}
                  active={activePane.type === WorkspacePaneTypes.agent && activePane.id === item.id}
                  t={t}
                  onSelect={onSelectAgent}
                  onPreview={onPreviewAgent}
                  notification
                />
              ))
            ) : (
              <div className="workspace-empty">{t("noNotificationBots")}</div>
            )}
          </WorkspaceGroup>
          <WorkspaceGroup
            id="computers"
            title={t("computersSection")}
            count={1}
            collapsed={Boolean(collapsedWorkspaceGroups.computers)}
            onToggle={() => onToggleWorkspaceGroup("computers")}
          >
            <WorkspaceComputerRow
              title={t("localComputer")}
              active={activePane.type === WorkspacePaneTypes.computer}
              subtitle={`${agentItems.length} ${t("computerAgentsSection")}`}
              onSelect={onSelectComputer}
            />
          </WorkspaceGroup>
        </div>
      )}
      {agentsError ? <div className="form-error agent-error">{agentsError}</div> : null}
    </>
  );
}
