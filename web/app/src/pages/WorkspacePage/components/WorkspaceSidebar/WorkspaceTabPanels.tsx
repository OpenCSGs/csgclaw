import { useCallback, useEffect, useMemo, useState } from "react";
import { HubIcon } from "@/components/ui/Icons";
import { isDirectConversation, resolveConversationUser } from "@/models/conversations";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { WORKSPACE_SECTION_ORDER_STORAGE_KEY } from "@/shared/storage/keys";
import {
  WorkspaceAgentRow,
  WorkspaceComputerRow,
  WorkspaceConversationRow,
  WorkspaceGroup,
  WorkspaceThreadRow,
} from "../WorkspaceRows";

const MessageSectionIds = {
  rooms: "rooms",
  directMessages: "direct-messages",
  threads: "threads",
} as const;

const AgentSectionIds = {
  agents: "agents",
  notifications: "notifications",
  computers: "computers",
} as const;

const SectionPanels = {
  messages: "messages",
  agents: "agents",
  tasks: "tasks",
} as const;

const LEGACY_DEFAULT_MESSAGE_SECTION_ORDERS = [
  [MessageSectionIds.rooms, MessageSectionIds.directMessages, MessageSectionIds.threads],
];

const DEFAULT_SECTION_ORDERS = {
  [SectionPanels.messages]: [MessageSectionIds.directMessages, MessageSectionIds.rooms, MessageSectionIds.threads],
  [SectionPanels.agents]: [AgentSectionIds.agents, AgentSectionIds.computers, AgentSectionIds.notifications],
} as const;

function orderEquals(left, right) {
  return left.length === right.length && left.every((item, index) => item === right[index]);
}

function normalizeSectionOrder(value, defaults, legacyDefaults = []) {
  const allowed = new Set(defaults);
  const ordered = Array.isArray(value) ? value.filter((item) => allowed.has(item)) : [];
  if (legacyDefaults.some((legacyDefault) => orderEquals(ordered, legacyDefault))) {
    return [...defaults];
  }
  return [...ordered, ...defaults.filter((item) => !ordered.includes(item))];
}

function readSectionOrders() {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(WORKSPACE_SECTION_ORDER_STORAGE_KEY) || "{}");
    return {
      [SectionPanels.messages]: normalizeSectionOrder(
        parsed?.[SectionPanels.messages],
        DEFAULT_SECTION_ORDERS.messages,
        LEGACY_DEFAULT_MESSAGE_SECTION_ORDERS,
      ),
      [SectionPanels.agents]: normalizeSectionOrder(parsed?.[SectionPanels.agents], DEFAULT_SECTION_ORDERS.agents),
    };
  } catch (_) {
    return {
      [SectionPanels.messages]: [...DEFAULT_SECTION_ORDERS.messages],
      [SectionPanels.agents]: [...DEFAULT_SECTION_ORDERS.agents],
    };
  }
}

function reorderSection(order, sourceId, targetId) {
  if (!sourceId || !targetId || sourceId === targetId || !order.includes(sourceId) || !order.includes(targetId)) {
    return order;
  }
  const next = order.filter((item) => item !== sourceId);
  next.splice(next.indexOf(targetId), 0, sourceId);
  return next;
}

export function WorkspaceTabPanels({
  workspaceTab,
  taskCount = 0,
  channels,
  directMessages,
  threadGroups = [],
  activePane,
  activeThreadRootID,
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
  onSelectThread,
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
  const threadCount = useMemo(
    () => threadGroups.reduce((count, group) => count + group.threads.length, 0),
    [threadGroups],
  );
  const [sectionOrders, setSectionOrders] = useState(readSectionOrders);
  const [dragState, setDragState] = useState({ id: "", overId: "", panel: "" });

  useEffect(() => {
    window.localStorage.setItem(WORKSPACE_SECTION_ORDER_STORAGE_KEY, JSON.stringify(sectionOrders));
  }, [sectionOrders]);

  const moveSection = useCallback((panel, sourceId, targetId) => {
    setSectionOrders((current) => ({
      ...current,
      [panel]: reorderSection(current[panel] ?? [], sourceId, targetId),
    }));
  }, []);

  const sectionDragProps = useCallback(
    (panel, id) => ({
      dragOver: dragState.panel === panel && dragState.overId === id && dragState.id !== id,
      dragging: dragState.panel === panel && dragState.id === id,
      onDragStart: (event) => {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("application/x-csgclaw-section", `${panel}:${id}`);
        event.dataTransfer.setData("text/plain", `${panel}:${id}`);
        setDragState({ id, overId: "", panel });
      },
      onDragOver: (event) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
        setDragState((current) =>
          current.panel === panel && current.id && current.overId !== id ? { ...current, overId: id } : current,
        );
      },
      onDragLeave: (event) => {
        const relatedTarget = event.relatedTarget;
        if (relatedTarget instanceof Node && event.currentTarget.contains(relatedTarget)) {
          return;
        }
        setDragState((current) =>
          current.panel === panel && current.overId === id ? { ...current, overId: "" } : current,
        );
      },
      onDrop: (event) => {
        event.preventDefault();
        const payload =
          event.dataTransfer.getData("application/x-csgclaw-section") || event.dataTransfer.getData("text/plain");
        const [sourcePanel, sourceId] = payload.split(":");
        if (sourcePanel === panel) {
          moveSection(panel, sourceId, id);
        }
        setDragState({ id: "", overId: "", panel: "" });
      },
      onDragEnd: () => setDragState({ id: "", overId: "", panel: "" }),
    }),
    [dragState.id, dragState.overId, dragState.panel, moveSection],
  );

  function renderThreadRows() {
    if (!threadGroups.length) {
      return <div className="workspace-empty">{t("noThreads")}</div>;
    }
    return threadGroups.map((group) =>
      group.threads.map((thread) => {
        const rootID = thread.summary?.root_id || thread.root?.id;
        return (
          <WorkspaceThreadRow
            key={`${group.conversation.id}:${rootID}`}
            conversation={group.conversation}
            thread={thread}
            active={
              activePane.type === WorkspacePaneTypes.conversation &&
              activePane.id === group.conversation.id &&
              activeThreadRootID === rootID
            }
            locale={locale}
            t={t}
            onSelect={onSelectThread}
          />
        );
      }),
    );
  }

  function renderMessageSection(id) {
    if (id === MessageSectionIds.rooms) {
      return (
        <WorkspaceGroup
          key={id}
          id="rooms"
          title={t("channelsSection")}
          count={channels.length}
          collapsed={Boolean(collapsedWorkspaceGroups.rooms)}
          onToggle={() => onToggleWorkspaceGroup("rooms")}
          onAdd={() => onCreateRoom()}
          addLabel={t("createRoom")}
          {...sectionDragProps(SectionPanels.messages, id)}
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
      );
    }
    if (id === MessageSectionIds.directMessages) {
      return (
        <WorkspaceGroup
          key={id}
          id="direct-messages"
          title={t("directMessagesSection")}
          count={directMessages.length}
          collapsed={Boolean(collapsedWorkspaceGroups["direct-messages"])}
          onToggle={() => onToggleWorkspaceGroup("direct-messages")}
          {...sectionDragProps(SectionPanels.messages, id)}
        >
          {directMessages.length ? (
            directMessages.map((conversation) => (
              <WorkspaceConversationRow
                key={conversation.id}
                agents={agentItems}
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
      );
    }
    return (
      <WorkspaceGroup
        key={id}
        id="threads"
        title={t("threadsSection")}
        count={threadCount}
        collapsed={Boolean(collapsedWorkspaceGroups.threads)}
        onToggle={() => onToggleWorkspaceGroup("threads")}
        {...sectionDragProps(SectionPanels.messages, id)}
      >
        {renderThreadRows()}
      </WorkspaceGroup>
    );
  }

  function renderAgentSection(id) {
    if (id === AgentSectionIds.agents) {
      return (
        <WorkspaceGroup
          key={id}
          id="agents"
          title={t("computerAgentsSection")}
          count={workerAgentItems.length}
          collapsed={Boolean(collapsedWorkspaceGroups.agents)}
          onToggle={() => onToggleWorkspaceGroup("agents")}
          onAdd={onCreateAgent}
          addLabel={t("createAgent")}
          {...sectionDragProps(SectionPanels.agents, id)}
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
      );
    }
    if (id === AgentSectionIds.notifications) {
      return (
        <WorkspaceGroup
          key={id}
          id="notifications"
          title={t("notificationsSection")}
          count={notificationAgentItems.length}
          collapsed={Boolean(collapsedWorkspaceGroups.notifications)}
          onToggle={() => onToggleWorkspaceGroup("notifications")}
          onAdd={onCreateNotificationBot}
          addLabel={t("createNotificationBot")}
          {...sectionDragProps(SectionPanels.agents, id)}
        >
          {notificationAgentItems.length
            ? notificationAgentItems.map((item) => (
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
            : null}
        </WorkspaceGroup>
      );
    }
    return (
      <WorkspaceGroup
        key={id}
        id="computers"
        title={t("computersSection")}
        count={1}
        collapsed={Boolean(collapsedWorkspaceGroups.computers)}
        onToggle={() => onToggleWorkspaceGroup("computers")}
        {...sectionDragProps(SectionPanels.agents, id)}
      >
        <WorkspaceComputerRow
          title={t("localComputer")}
          active={activePane.type === WorkspacePaneTypes.computer}
          subtitle={`${t("computerAgentsSection")} ${agentItems.length}`}
          onSelect={onSelectComputer}
        />
      </WorkspaceGroup>
    );
  }

  return (
    <>
      {workspaceTab === WorkspaceTabs.messages ? (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("messagesTab")}>
          {sectionOrders.messages.map(renderMessageSection)}
        </div>
      ) : workspaceTab === WorkspaceTabs.threads ? (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("threadsTab")}>
          {threadGroups.length ? (
            threadGroups.map((group) => {
              const displayUser = isDirectConversation(group.conversation)
                ? resolveConversationUser(group.conversation, currentUserID, usersById)
                : null;
              const groupTitle = displayUser?.name || group.conversation.title;
              return (
                <WorkspaceGroup
                  key={group.conversation.id}
                  id={`threads-${group.conversation.id}`}
                  title={groupTitle}
                  count={group.threads.length}
                  collapsed={Boolean(collapsedWorkspaceGroups[`threads-${group.conversation.id}`])}
                  onToggle={() => onToggleWorkspaceGroup(`threads-${group.conversation.id}`)}
                >
                  {group.threads.map((thread) => {
                    const rootID = thread.summary?.root_id || thread.root?.id;
                    return (
                      <WorkspaceThreadRow
                        key={`${group.conversation.id}:${rootID}`}
                        conversation={group.conversation}
                        thread={thread}
                        active={
                          activePane.type === WorkspacePaneTypes.conversation &&
                          activePane.id === group.conversation.id &&
                          activeThreadRootID === rootID
                        }
                        locale={locale}
                        t={t}
                        onSelect={onSelectThread}
                      />
                    );
                  })}
                </WorkspaceGroup>
              );
            })
          ) : (
            <div className="workspace-empty">{t("noThreads")}</div>
          )}
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
      ) : workspaceTab === WorkspaceTabs.tasks ? (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("tasksTab")}>
          <WorkspaceGroup
            id="global-tasks"
            title={t("tasksTab")}
            count={taskCount}
            collapsed={false}
            onToggle={() => {}}
            {...sectionDragProps(SectionPanels.tasks, "global-tasks")}
          >
            <div className="workspace-empty">{t("tasksSidebarHint")}</div>
          </WorkspaceGroup>
        </div>
      ) : (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("agentsTab")}>
          {sectionOrders.agents.map(renderAgentSection)}
        </div>
      )}
      {agentsError ? <div className="form-error agent-error">{agentsError}</div> : null}
    </>
  );
}
