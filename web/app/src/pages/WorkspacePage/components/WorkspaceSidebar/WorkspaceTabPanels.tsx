import { useCallback, useEffect, useMemo, useState } from "react";
import type { DragEvent } from "react";
import { FileCode2, Plus } from "lucide-react";
import { ModelsIcon, UsersIcon } from "@/components/ui/Icons";
import { isDirectConversation, resolveConversationUser } from "@/models/conversations";
import { modelProviderAvatarPath, providerStatusTone, type ModelProvider } from "@/models/modelProviders";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { displayTeam } from "@/models/tasks";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { WORKSPACE_SECTION_ORDER_STORAGE_KEY } from "@/shared/storage/keys";
import { SkillUploadDialog } from "./SkillUploadDialog";
import {
  WorkspaceAgentRow,
  WorkspaceComputerRow,
  WorkspaceConversationRow,
  WorkspaceGroup,
  WorkspaceHumanRow,
  WorkspaceThreadRow,
} from "../WorkspaceRows";
import type { WorkspaceSidebarProps } from "./types";

const MessageSectionIds = {
  rooms: "rooms",
  directMessages: "direct-messages",
  threads: "threads",
} as const;

const AgentSectionIds = {
  models: "models",
  agents: "agents",
  humans: "humans",
  teams: "teams",
  notifications: "notifications",
  computers: "computers",
} as const;

const SectionPanels = {
  messages: "messages",
  agents: "agents",
  tasks: "tasks",
} as const;

type MessageSectionId = (typeof MessageSectionIds)[keyof typeof MessageSectionIds];
type AgentSectionId = (typeof AgentSectionIds)[keyof typeof AgentSectionIds];
type OrderedSectionPanel = typeof SectionPanels.messages | typeof SectionPanels.agents;
type SectionId = MessageSectionId | AgentSectionId;
type SectionOrders = Record<OrderedSectionPanel, SectionId[]>;
type SectionDragState = {
  id: SectionId | "";
  overId: SectionId | "";
  panel: OrderedSectionPanel | "";
};
type WorkspaceTabPanelsProps = Pick<
  WorkspaceSidebarProps,
  | "activePane"
  | "activeThreadRootID"
  | "agentItems"
  | "agentsError"
  | "channels"
  | "collapsedWorkspaceGroups"
  | "currentUserID"
  | "directMessages"
  | "hub"
  | "locale"
  | "modelProviders"
  | "modelProvidersLoaded"
  | "notificationAgentItems"
  | "onCreateAgent"
  | "onCreateModelProvider"
  | "onCreateNotificationParticipant"
  | "onCreateRoom"
  | "onOpenCreateTask"
  | "onOpenCreateTeam"
  | "onPreviewAgent"
  | "onPreviewUser"
  | "onSelectAgent"
  | "onSelectComputer"
  | "onSelectConversation"
  | "onSelectHuman"
  | "onSelectHubSkill"
  | "onSelectHubTemplate"
  | "onSelectModelProvider"
  | "onSelectTask"
  | "onSelectTaskBoardView"
  | "onSelectTeam"
  | "onSelectThread"
  | "onToggleWorkspaceGroup"
  | "onViewTaskDetails"
  | "planningTaskID"
  | "startingTaskID"
  | "t"
  | "taskCount"
  | "scheduledTaskCount"
  | "activeTaskBoardView"
  | "taskItems"
  | "teams"
  | "threadGroups"
  | "usersById"
  | "workerAgentItems"
  | "workspaceTab"
>;

const LEGACY_DEFAULT_MESSAGE_SECTION_ORDERS: readonly (readonly MessageSectionId[])[] = [
  [MessageSectionIds.rooms, MessageSectionIds.directMessages, MessageSectionIds.threads],
];

const LEGACY_DEFAULT_AGENT_SECTION_ORDERS: readonly (readonly AgentSectionId[])[] = [
  [AgentSectionIds.agents, AgentSectionIds.teams, AgentSectionIds.computers, AgentSectionIds.notifications],
  [
    AgentSectionIds.models,
    AgentSectionIds.agents,
    AgentSectionIds.humans,
    AgentSectionIds.computers,
    AgentSectionIds.notifications,
    AgentSectionIds.teams,
  ],
];

const DEFAULT_SECTION_ORDERS = {
  [SectionPanels.messages]: [MessageSectionIds.directMessages, MessageSectionIds.rooms, MessageSectionIds.threads],
  [SectionPanels.agents]: [
    AgentSectionIds.agents,
    AgentSectionIds.humans,
    AgentSectionIds.computers,
    AgentSectionIds.notifications,
    AgentSectionIds.teams,
  ],
} as const;

function orderEquals(left: readonly SectionId[], right: readonly SectionId[]): boolean {
  return left.length === right.length && left.every((item, index) => item === right[index]);
}

function normalizeSectionOrder(
  value: unknown,
  defaults: readonly SectionId[],
  legacyDefaults: readonly (readonly SectionId[])[] = [],
): SectionId[] {
  const allowed = new Set(defaults);
  const ordered = Array.isArray(value)
    ? value.filter((item): item is SectionId => typeof item === "string" && allowed.has(item as SectionId))
    : [];
  if (legacyDefaults.some((legacyDefault) => orderEquals(ordered, legacyDefault))) {
    return [...defaults];
  }
  const next = [...ordered];
  defaults.forEach((defaultItem, defaultIndex) => {
    if (next.includes(defaultItem)) {
      return;
    }
    const precedingDefault = defaults
      .slice(0, defaultIndex)
      .reverse()
      .find((item) => next.includes(item));
    if (!precedingDefault) {
      next.unshift(defaultItem);
      return;
    }
    next.splice(next.indexOf(precedingDefault) + 1, 0, defaultItem);
  });
  return next;
}

function readSectionOrders(): SectionOrders {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(WORKSPACE_SECTION_ORDER_STORAGE_KEY) || "{}");
    return {
      [SectionPanels.messages]: normalizeSectionOrder(
        parsed?.[SectionPanels.messages],
        DEFAULT_SECTION_ORDERS.messages,
        LEGACY_DEFAULT_MESSAGE_SECTION_ORDERS,
      ),
      [SectionPanels.agents]: normalizeSectionOrder(
        parsed?.[SectionPanels.agents],
        DEFAULT_SECTION_ORDERS.agents,
        LEGACY_DEFAULT_AGENT_SECTION_ORDERS,
      ),
    };
  } catch (_) {
    return {
      [SectionPanels.messages]: [...DEFAULT_SECTION_ORDERS.messages],
      [SectionPanels.agents]: [...DEFAULT_SECTION_ORDERS.agents],
    };
  }
}

function reorderSection(order: readonly SectionId[], sourceId: SectionId, targetId: SectionId): SectionId[] {
  if (!sourceId || !targetId || sourceId === targetId || !order.includes(sourceId) || !order.includes(targetId)) {
    return [...order];
  }
  const next = order.filter((item) => item !== sourceId);
  next.splice(next.indexOf(targetId), 0, sourceId);
  return next;
}

export function WorkspaceTabPanels({
  workspaceTab,
  taskCount = 0,
  scheduledTaskCount = 0,
  activeTaskBoardView = "tasks",
  teams = [],
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
  onCreateNotificationParticipant,
  onOpenCreateTeam,
  onOpenCreateTask,
  hub,
  onSelectHubTemplate,
  onSelectTeam,
  agentsError,
  onSelectConversation,
  onSelectThread,
  onPreviewUser,
  onSelectHuman,
  onSelectHubSkill,
  agentItems,
  modelProviders = null,
  modelProvidersLoaded = false,
  workerAgentItems = agentItems,
  notificationAgentItems = [],
  onSelectAgent,
  onSelectModelProvider = () => {},
  onPreviewAgent,
  onSelectComputer,
  onCreateModelProvider,
  onSelectTaskBoardView = () => {},
  onSelectTask,
}: WorkspaceTabPanelsProps) {
  const [skillUploadOpen, setSkillUploadOpen] = useState(false);
  const resourcesTemplates = hub?.templates ?? [];
  const resourcesSkills = hub?.skills ?? [];
  const resourcesError = hub?.listError ?? "";
  const resourcesSkillsError = hub?.skillsError ?? "";
  const resourcesUploadBusy = hub?.uploadBusy ?? false;
  const resourcesUploadError = hub?.uploadError ?? "";
  const resourcesLoaded = hub?.loaded ?? false;
  const selectedHubResourceType = hub?.selectedHubResourceType ?? "template";
  const selectedHubSkillName = hub?.selectedHubSkillName ?? "";
  const selectedHubTemplateId = hub?.selectedHubTemplateId ?? "";
  const resourcesPaneActive = activePane.type === WorkspacePaneTypes.hub;
  const threadCount = useMemo(
    () => threadGroups.reduce((count, group) => count + group.threads.length, 0),
    [threadGroups],
  );
  const [sectionOrders, setSectionOrders] = useState<SectionOrders>(readSectionOrders);
  const [dragState, setDragState] = useState<SectionDragState>({ id: "", overId: "", panel: "" });

  useEffect(() => {
    window.localStorage.setItem(WORKSPACE_SECTION_ORDER_STORAGE_KEY, JSON.stringify(sectionOrders));
  }, [sectionOrders]);

  const moveSection = useCallback((panel: OrderedSectionPanel, sourceId: SectionId, targetId: SectionId) => {
    setSectionOrders((current) => ({
      ...current,
      [panel]: reorderSection(current[panel] ?? [], sourceId, targetId),
    }));
  }, []);

  const sectionDragProps = useCallback(
    (panel: OrderedSectionPanel, id: SectionId) => ({
      dragOver: dragState.panel === panel && dragState.overId === id && dragState.id !== id,
      dragging: dragState.panel === panel && dragState.id === id,
      onDragStart: (event: DragEvent<HTMLElement>) => {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("application/x-csgclaw-section", `${panel}:${id}`);
        event.dataTransfer.setData("text/plain", `${panel}:${id}`);
        setDragState({ id, overId: "", panel });
      },
      onDragOver: (event: DragEvent<HTMLElement>) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
        setDragState((current) =>
          current.panel === panel && current.id && current.overId !== id ? { ...current, overId: id } : current,
        );
      },
      onDragLeave: (event: DragEvent<HTMLElement>) => {
        const relatedTarget = event.relatedTarget;
        if (relatedTarget instanceof Node && event.currentTarget.contains(relatedTarget)) {
          return;
        }
        setDragState((current) =>
          current.panel === panel && current.overId === id ? { ...current, overId: "" } : current,
        );
      },
      onDrop: (event: DragEvent<HTMLElement>) => {
        event.preventDefault();
        const payload =
          event.dataTransfer.getData("application/x-csgclaw-section") || event.dataTransfer.getData("text/plain");
        const [sourcePanel, sourceId] = payload.split(":");
        if (sourcePanel === panel && isSectionId(sourceId)) {
          moveSection(panel, sourceId, id);
        }
        setDragState({ id: "", overId: "", panel: "" });
      },
      onDragEnd: () => setDragState({ id: "", overId: "", panel: "" }),
    }),
    [dragState.id, dragState.overId, dragState.panel, moveSection],
  );

  function isSectionId(value: string | undefined): value is SectionId {
    return (
      value === MessageSectionIds.rooms ||
      value === MessageSectionIds.directMessages ||
      value === MessageSectionIds.threads ||
      value === AgentSectionIds.agents ||
      value === AgentSectionIds.humans ||
      value === AgentSectionIds.teams ||
      value === AgentSectionIds.notifications ||
      value === AgentSectionIds.computers
    );
  }

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

  function renderMessageSection(id: SectionId) {
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

  function renderModelProviderRow(provider: ModelProvider) {
    const tone = providerStatusTone(provider.status, provider);
    const modelCount = provider.models.length;
    const metaParts = [
      modelCount ? t("modelProviderModelCount", { count: modelCount }) : t("modelProviderNoModels"),
      provider.message || (provider.status === "connected" ? t("modelProviderConnected") : ""),
    ].filter(Boolean);
    return (
      <button
        key={provider.id}
        className={`workspace-row model-provider-row ${
          activePane.type === WorkspacePaneTypes.modelProvider && activePane.id === provider.id ? "active" : ""
        }`}
        onClick={() => onSelectModelProvider(provider)}
      >
        <span className="workspace-row-icon">
          <img src={modelProviderAvatarPath(provider)} alt="" aria-hidden="true" />
        </span>
        <span className="workspace-row-main">
          <span className="workspace-row-title-line">
            <span className="workspace-row-title truncate">{provider.display_name || provider.id}</span>
            <span className={`workspace-status-dot ${tone}`} aria-hidden="true"></span>
          </span>
          <span className="workspace-row-meta truncate">{metaParts.join(" · ") || provider.kind}</span>
        </span>
      </button>
    );
  }

  function renderModelProviderSection() {
    const providers = modelProviders?.providers ?? [];
    const builtins = modelProviders?.builtinProviders ?? [];
    const custom = modelProviders?.customProviders ?? [];
    return (
      <WorkspaceGroup
        id="models"
        title={t("resourcesModelProvidersSection")}
        count={providers.length}
        collapsed={Boolean(collapsedWorkspaceGroups.models)}
        onToggle={() => onToggleWorkspaceGroup("models")}
        onAdd={onCreateModelProvider}
        addLabel={t("modelProviderAdd")}
        addIcon={<Plus aria-hidden="true" size={16} />}
      >
        {!modelProvidersLoaded ? <div className="workspace-empty">{t("profileLoadingModels")}</div> : null}
        {modelProvidersLoaded && builtins.map(renderModelProviderRow)}
        {modelProvidersLoaded && custom.length ? <div className="workspace-provider-divider" /> : null}
        {modelProvidersLoaded && custom.map(renderModelProviderRow)}
      </WorkspaceGroup>
    );
  }

  function renderAgentSection(id: SectionId) {
    if (id === AgentSectionIds.models) {
      return null;
    }
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
    if (id === AgentSectionIds.humans) {
      const currentUser = usersById.get(currentUserID);
      const humanUsers = currentUser ? [currentUser] : [];
      return (
        <WorkspaceGroup
          key={id}
          id="humans"
          title={t("humanSection")}
          count={humanUsers.length}
          collapsed={Boolean(collapsedWorkspaceGroups.humans)}
          onToggle={() => onToggleWorkspaceGroup("humans")}
          {...sectionDragProps(SectionPanels.agents, id)}
        >
          {humanUsers.map((user) => (
            <WorkspaceHumanRow
              key={user.id}
              user={user}
              active={activePane.type === WorkspacePaneTypes.human && activePane.id === user.id}
              t={t}
              onSelect={onSelectHuman}
              onPreview={onPreviewUser}
            />
          ))}
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
          onAdd={onCreateNotificationParticipant}
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
    if (id === AgentSectionIds.teams) {
      return (
        <WorkspaceGroup
          key={id}
          id="teams"
          title={t("teamsSection")}
          count={teams.length}
          collapsed={Boolean(collapsedWorkspaceGroups.teams)}
          onToggle={() => onToggleWorkspaceGroup("teams")}
          onAdd={onOpenCreateTeam}
          addLabel={t("teamCreate")}
          {...sectionDragProps(SectionPanels.agents, id)}
        >
          {teams.length ? (
            teams.map((team) => {
              const memberCount = team.member_agent_ids.length + (team.lead_agent_id ? 1 : 0);
              return (
                <button
                  key={team.id}
                  className={`workspace-row team-nav-row ${
                    activePane.type === WorkspacePaneTypes.team && activePane.id === team.id ? "active" : ""
                  }`}
                  onClick={() => onSelectTeam?.(team)}
                >
                  <span className="workspace-row-icon">
                    <UsersIcon />
                  </span>
                  <span className="workspace-row-main">
                    <span className="workspace-row-title truncate">{displayTeam(team)}</span>
                    <span className="workspace-row-meta truncate">
                      {t("teamMembersCount", { count: memberCount })} · {team.status}
                    </span>
                  </span>
                </button>
              );
            })
          ) : (
            <div className="workspace-empty">{t("noTeams")}</div>
          )}
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
              const groupTitle = displayUser?.name || group.conversation.title || "";
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
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("resourcesTab")}>
          <WorkspaceGroup
            id="hub-templates"
            title={t("resourcesTemplatesSection")}
            count={resourcesTemplates.length}
            collapsed={Boolean(collapsedWorkspaceGroups["hub-templates"])}
            onToggle={() => onToggleWorkspaceGroup("hub-templates")}
          >
            {resourcesError ? (
              <div className="workspace-empty">{resourcesError}</div>
            ) : resourcesLoaded && resourcesTemplates.length === 0 && resourcesSkills.length === 0 ? (
              <div className="workspace-empty">{t("resourcesEmpty")}</div>
            ) : (
              <>
                {resourcesTemplates.slice(0, 6).map((item) => (
                  <button
                    key={item.id}
                    className={`workspace-row hub-template-row ${
                      resourcesPaneActive && selectedHubTemplateId === item.id && selectedHubResourceType === "template"
                        ? "active"
                        : ""
                    }`}
                    onClick={() => onSelectHubTemplate(item)}
                  >
                    <span className="workspace-row-icon">
                      <ModelsIcon />
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
                ))}
              </>
            )}
          </WorkspaceGroup>
          <WorkspaceGroup
            id="hub-skills"
            title={t("resourcesSkillsLabel")}
            count={resourcesSkills.length}
            collapsed={Boolean(collapsedWorkspaceGroups["hub-skills"])}
            onToggle={() => onToggleWorkspaceGroup("hub-skills")}
            onAdd={() => setSkillUploadOpen(true)}
            addLabel={t("resourcesSkillUpload")}
            addIcon={<Plus size={15} strokeWidth={2.2} aria-hidden="true" />}
          >
            {resourcesSkills.length ? (
              resourcesSkills.map((item) => (
                <button
                  key={item.name}
                  className={`workspace-row hub-template-row hub-skill-row ${
                    resourcesPaneActive && selectedHubSkillName === item.name && selectedHubResourceType === "skill"
                      ? "active"
                      : ""
                  }`}
                  onClick={() => onSelectHubSkill(item)}
                >
                  <span className="workspace-row-icon">
                    <FileCode2 size={16} strokeWidth={2} aria-hidden="true" />
                  </span>
                  <span className="workspace-row-main">
                    <span className="workspace-row-title truncate">{item.name}</span>
                    <span className="workspace-row-meta truncate">{item.description || item.name}</span>
                  </span>
                </button>
              ))
            ) : resourcesSkillsError ? (
              <div className="workspace-empty">{resourcesSkillsError}</div>
            ) : resourcesLoaded && resourcesTemplates.length === 0 ? (
              <div className="workspace-empty">{t("resourcesSkillsEmpty")}</div>
            ) : null}
          </WorkspaceGroup>
          {renderModelProviderSection()}
          <SkillUploadDialog
            open={skillUploadOpen}
            onOpenChange={setSkillUploadOpen}
            onSubmit={(file) => hub?.uploadSkill?.(file)}
            busy={resourcesUploadBusy}
            error={resourcesUploadError}
            installedSkills={resourcesSkills}
            onInstallRemoteSkill={hub?.installRemoteSkill}
            onLoadMoreRemoteSkills={hub?.loadMoreRemoteSkills}
            remoteInstallBusy={hub?.remoteInstallBusy || ""}
            remoteInstallError={hub?.remoteInstallError || ""}
            remoteSkillsHasMore={Boolean(hub?.remoteSkillsHasMore)}
            remoteSkills={hub?.remoteSkills ?? []}
            remoteSkillsLoading={Boolean(hub?.remoteSkillsLoading)}
            remoteSkillsLoadingMore={Boolean(hub?.remoteSkillsLoadingMore)}
            remoteSkillsSearch={hub?.remoteSkillsSearch || ""}
            remoteSkillsError={hub?.remoteSkillsError || ""}
            onRefreshRemoteSkills={hub?.refetchRemoteSkills}
            onRemoteSkillsSearchChange={hub?.setRemoteSkillsSearch}
            onRemoteVisibleChange={hub?.setRemoteSkillsEnabled}
            t={t}
          />
        </div>
      ) : workspaceTab === WorkspaceTabs.tasks ? (
        <div className="workspace-tab-panel" role="tabpanel" aria-label={t("tasksTab")}>
          <WorkspaceGroup
            id="tasks"
            title={t("tasksTab")}
            count={taskCount + scheduledTaskCount}
            collapsed={Boolean(collapsedWorkspaceGroups.tasks)}
            onToggle={() => onToggleWorkspaceGroup("tasks")}
            onAdd={onOpenCreateTask}
            addLabel={t("taskCreate")}
            addIcon={<Plus size={15} strokeWidth={2.2} aria-hidden="true" />}
          >
            <button
              type="button"
              className={`workspace-row workspace-task-section-row ${activeTaskBoardView === "tasks" ? "active" : ""}`}
              onClick={() => {
                onSelectTaskBoardView("tasks");
                onSelectTask();
              }}
            >
              <span className="workspace-row-main">
                <span className="workspace-row-title truncate">{t("normalTasksTab")}</span>
              </span>
              <span className="workspace-row-count">{taskCount}</span>
            </button>
            <button
              type="button"
              className={`workspace-row workspace-task-section-row ${
                activeTaskBoardView === "scheduled" ? "active" : ""
              }`}
              onClick={() => {
                onSelectTaskBoardView("scheduled");
                onSelectTask();
              }}
            >
              <span className="workspace-row-main">
                <span className="workspace-row-title truncate">{t("scheduledTasksTab")}</span>
              </span>
              <span className="workspace-row-count">{scheduledTaskCount}</span>
            </button>
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
