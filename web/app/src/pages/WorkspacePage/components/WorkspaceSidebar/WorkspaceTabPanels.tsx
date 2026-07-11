import { useCallback, useEffect, useMemo, useState } from "react";
import type { DragEvent } from "react";
import { FileCode2, Plus } from "lucide-react";
import { ModelsIcon, UsersIcon } from "@/components/ui/Icons";
import { isDirectConversation, resolveConversationUser } from "@/models/conversations";
import { modelProviderAvatarPath, providerStatusTone, type ModelProvider } from "@/models/modelProviders";
import { WorkspacePaneTypes, WorkspaceTabs } from "@/models/routing";
import { displayTeam } from "@/models/tasks";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { classNames } from "@/shared/lib/classNames";
import { WORKSPACE_SECTION_ORDER_STORAGE_KEY } from "@/shared/storage/keys";
import { SkillUploadDialog } from "./SkillUploadDialog";
import { WorkspaceContextSectionIds } from "./types";
import styles from "./WorkspaceTabPanels.module.css";
import rowStyles from "../WorkspaceRows/WorkspaceRows.module.css";
import {
  WorkspaceAgentRow,
  WorkspaceComputerRow,
  WorkspaceConversationRow,
  WorkspaceGroup,
  WorkspaceHumanRow,
  WorkspaceThreadRow,
} from "../WorkspaceRows";
import type { WorkspaceContextSectionId, WorkspaceSidebarProps } from "./types";

const MessageSectionIds = {
  rooms: WorkspaceContextSectionIds.rooms,
  directMessages: WorkspaceContextSectionIds.directMessages,
  threads: WorkspaceContextSectionIds.threads,
} as const;

const AgentSectionIds = {
  agents: WorkspaceContextSectionIds.agents,
  humans: WorkspaceContextSectionIds.humans,
  teams: WorkspaceContextSectionIds.teams,
  notifications: WorkspaceContextSectionIds.notifications,
  computers: WorkspaceContextSectionIds.computers,
} as const;

const HubSectionIds = {
  templates: WorkspaceContextSectionIds.hubTemplates,
  skills: WorkspaceContextSectionIds.hubSkills,
  models: WorkspaceContextSectionIds.models,
} as const;

const SectionPanels = {
  messages: "messages",
  agents: "agents",
  tasks: "tasks",
} as const;

type MessageSectionId = (typeof MessageSectionIds)[keyof typeof MessageSectionIds];
type AgentSectionId = (typeof AgentSectionIds)[keyof typeof AgentSectionIds];
type HubSectionId = (typeof HubSectionIds)[keyof typeof HubSectionIds];
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
  | "onOpenCreateScheduledTask"
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
> & {
  contextQuery?: string;
  contextSectionId?: WorkspaceContextSectionId;
  onSkillUploadOpenChange?: (open: boolean) => void;
  skillUploadOpen?: boolean;
};

const LEGACY_DEFAULT_MESSAGE_SECTION_ORDERS: readonly (readonly MessageSectionId[])[] = [
  [MessageSectionIds.rooms, MessageSectionIds.directMessages, MessageSectionIds.threads],
];

const LEGACY_DEFAULT_AGENT_SECTION_ORDERS: readonly (readonly AgentSectionId[])[] = [
  [AgentSectionIds.agents, AgentSectionIds.teams, AgentSectionIds.computers, AgentSectionIds.notifications],
  [
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

function normalizeSearchQuery(value: string) {
  return value.trim().toLocaleLowerCase();
}

function matchesSearch(query: string, ...values: unknown[]) {
  if (!query) {
    return true;
  }
  return values.some((value) =>
    String(value ?? "")
      .toLocaleLowerCase()
      .includes(query),
  );
}

export function WorkspaceTabPanels({
  contextQuery = "",
  contextSectionId,
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
  onOpenCreateScheduledTask,
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
  onSkillUploadOpenChange,
  skillUploadOpen,
}: WorkspaceTabPanelsProps) {
  const [internalSkillUploadOpen, setInternalSkillUploadOpen] = useState(false);
  const resolvedSkillUploadOpen = skillUploadOpen ?? internalSkillUploadOpen;
  const setResolvedSkillUploadOpen = onSkillUploadOpenChange ?? setInternalSkillUploadOpen;
  const normalizedContextQuery = normalizeSearchQuery(contextQuery);
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

  function conversationMatchesQuery(conversation: (typeof channels)[number]): boolean {
    if (!normalizedContextQuery) {
      return true;
    }
    const displayUser = isDirectConversation(conversation)
      ? resolveConversationUser(conversation, currentUserID, usersById)
      : null;
    return matchesSearch(
      normalizedContextQuery,
      conversation.id,
      conversation.title,
      displayUser?.id,
      displayUser?.name,
    );
  }

  function threadMatchesQuery(
    group: (typeof threadGroups)[number],
    thread: (typeof threadGroups)[number]["threads"][number],
  ) {
    if (!normalizedContextQuery) {
      return true;
    }
    const rootID = thread.summary?.root_id || thread.root?.id;
    return (
      conversationMatchesQuery(group.conversation) ||
      matchesSearch(
        normalizedContextQuery,
        rootID,
        thread.root?.content,
        thread.summary?.context_summary?.root_excerpt,
        thread.summary?.latest_reply?.content,
      )
    );
  }

  function agentMatchesQuery(item: (typeof agentItems)[number]) {
    const profile = typeof item.profile === "object" ? item.profile : null;
    return matchesSearch(
      normalizedContextQuery,
      item.id,
      item.name,
      item.role,
      profile?.model_provider_id,
      profile?.model_id,
    );
  }

  function userMatchesQuery(user: NonNullable<ReturnType<typeof usersById.get>>) {
    return matchesSearch(normalizedContextQuery, user.id, user.name, user.role);
  }

  function teamMatchesQuery(team: (typeof teams)[number]) {
    return matchesSearch(normalizedContextQuery, team.id, displayTeam(team), team.status);
  }

  function templateMatchesQuery(item: (typeof resourcesTemplates)[number]) {
    return matchesSearch(normalizedContextQuery, item.id, item.name, item.description, item.source?.name, item.role);
  }

  function skillMatchesQuery(item: (typeof resourcesSkills)[number]) {
    return matchesSearch(normalizedContextQuery, item.name, item.description);
  }

  function modelProviderMatchesQuery(provider: ModelProvider) {
    return matchesSearch(
      normalizedContextQuery,
      provider.id,
      provider.display_name,
      provider.kind,
      provider.message,
      provider.models.join(" "),
    );
  }

  function renderThreadRows() {
    const matchingThreadGroups = threadGroups
      .map((group) => ({
        ...group,
        threads: group.threads.filter((thread) => threadMatchesQuery(group, thread)),
      }))
      .filter((group) => group.threads.length || conversationMatchesQuery(group.conversation));
    if (!matchingThreadGroups.length) {
      return <div className={styles.empty}>{t("noThreads")}</div>;
    }
    return matchingThreadGroups.map((group) =>
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
      const visibleChannels = channels.filter(conversationMatchesQuery);
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
          {visibleChannels.length ? (
            visibleChannels.map((conversation) => (
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
            <div className={styles.empty}>{t("noChannels")}</div>
          )}
        </WorkspaceGroup>
      );
    }
    if (id === MessageSectionIds.directMessages) {
      const visibleDirectMessages = directMessages.filter(conversationMatchesQuery);
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
          {visibleDirectMessages.length ? (
            visibleDirectMessages.map((conversation) => (
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
            <div className={styles.empty}>{t("noDirectMessages")}</div>
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
        className={classNames(
          rowStyles.row,
          styles.modelProviderRow,
          activePane.type === WorkspacePaneTypes.modelProvider && activePane.id === provider.id && rowStyles.active,
        )}
        onClick={() => onSelectModelProvider(provider)}
      >
        <span className={classNames(rowStyles.icon, styles.modelProviderIconShell)}>
          <img
            className={styles.modelProviderIconImage}
            src={modelProviderAvatarPath(provider)}
            alt=""
            aria-hidden="true"
          />
        </span>
        <span className={rowStyles.main}>
          <span className={rowStyles.titleLine}>
            <span className={classNames(rowStyles.title, "truncate")}>{provider.display_name || provider.id}</span>
            <span className={classNames(rowStyles.statusDot, rowStyles[tone])} aria-hidden="true"></span>
          </span>
          <span className={classNames(rowStyles.meta, "truncate")}>{metaParts.join(" · ") || provider.kind}</span>
        </span>
      </button>
    );
  }

  function renderModelProviderSection(presentation: "group" | "flat" = "group") {
    const providers = modelProviders?.providers ?? [];
    const builtins = (modelProviders?.builtinProviders ?? []).filter(modelProviderMatchesQuery);
    const custom = (modelProviders?.customProviders ?? []).filter(modelProviderMatchesQuery);
    const visibleProviderCount = builtins.length + custom.length;
    const flat = presentation === "flat";
    return (
      <WorkspaceGroup
        id="models"
        title={t("resourcesModelProvidersSection")}
        count={providers.length}
        collapsed={flat ? false : Boolean(collapsedWorkspaceGroups.models)}
        onToggle={() => onToggleWorkspaceGroup("models")}
        onAdd={onCreateModelProvider}
        addLabel={t("modelProviderAdd")}
        addIcon={<Plus aria-hidden="true" size={16} />}
        presentation={presentation}
      >
        {!modelProvidersLoaded ? <div className={styles.empty}>{t("profileLoadingModels")}</div> : null}
        {modelProvidersLoaded && builtins.map(renderModelProviderRow)}
        {modelProvidersLoaded && custom.length ? <div className={styles.providerDivider} /> : null}
        {modelProvidersLoaded && custom.map(renderModelProviderRow)}
        {modelProvidersLoaded && !visibleProviderCount ? (
          <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
        ) : null}
      </WorkspaceGroup>
    );
  }

  function renderAgentSection(id: SectionId, presentation: "group" | "flat" = "group") {
    const flat = presentation === "flat";
    if (id === AgentSectionIds.agents) {
      const visibleWorkerAgents = workerAgentItems.filter(agentMatchesQuery);
      return (
        <WorkspaceGroup
          key={id}
          id="agents"
          title={t("computerAgentsSection")}
          count={workerAgentItems.length}
          collapsed={flat ? false : Boolean(collapsedWorkspaceGroups.agents)}
          onToggle={() => onToggleWorkspaceGroup("agents")}
          onAdd={onCreateAgent}
          addLabel={t("createAgent")}
          presentation={presentation}
          {...(flat ? {} : sectionDragProps(SectionPanels.agents, id))}
        >
          {visibleWorkerAgents.length ? (
            visibleWorkerAgents.map((item) => (
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
            <div className={styles.empty}>
              {workerAgentItems.length ? t("workspaceSearchNoResults") : t("noAgents")}
            </div>
          )}
        </WorkspaceGroup>
      );
    }
    if (id === AgentSectionIds.humans) {
      const currentUser = usersById.get(currentUserID);
      const humanUsers = (currentUser ? [currentUser] : []).filter(userMatchesQuery);
      return (
        <WorkspaceGroup
          key={id}
          id="humans"
          title={t("humanSection")}
          count={humanUsers.length}
          collapsed={flat ? false : Boolean(collapsedWorkspaceGroups.humans)}
          onToggle={() => onToggleWorkspaceGroup("humans")}
          presentation={presentation}
          {...(flat ? {} : sectionDragProps(SectionPanels.agents, id))}
        >
          {humanUsers.length ? (
            humanUsers.map((user) => (
              <WorkspaceHumanRow
                key={user.id}
                user={user}
                active={activePane.type === WorkspacePaneTypes.human && activePane.id === user.id}
                t={t}
                onSelect={onSelectHuman}
                onPreview={onPreviewUser}
              />
            ))
          ) : (
            <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
          )}
        </WorkspaceGroup>
      );
    }
    if (id === AgentSectionIds.notifications) {
      const visibleNotificationAgents = notificationAgentItems.filter(agentMatchesQuery);
      return (
        <WorkspaceGroup
          key={id}
          id="notifications"
          title={t("notificationsSection")}
          count={notificationAgentItems.length}
          collapsed={flat ? false : Boolean(collapsedWorkspaceGroups.notifications)}
          onToggle={() => onToggleWorkspaceGroup("notifications")}
          onAdd={onCreateNotificationParticipant}
          addLabel={t("createNotificationBot")}
          presentation={presentation}
          {...(flat ? {} : sectionDragProps(SectionPanels.agents, id))}
        >
          {visibleNotificationAgents.length ? (
            visibleNotificationAgents.map((item) => (
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
          ) : notificationAgentItems.length ? (
            <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
          ) : (
            <div className={styles.empty}>{t("noNotificationBots")}</div>
          )}
        </WorkspaceGroup>
      );
    }
    if (id === AgentSectionIds.teams) {
      const visibleTeams = teams.filter(teamMatchesQuery);
      return (
        <WorkspaceGroup
          key={id}
          id="teams"
          title={t("teamsSection")}
          count={teams.length}
          collapsed={flat ? false : Boolean(collapsedWorkspaceGroups.teams)}
          onToggle={() => onToggleWorkspaceGroup("teams")}
          onAdd={onOpenCreateTeam}
          addLabel={t("teamCreate")}
          presentation={presentation}
          {...(flat ? {} : sectionDragProps(SectionPanels.agents, id))}
        >
          {visibleTeams.length ? (
            visibleTeams.map((team) => {
              const memberCount = team.member_agent_ids.length + (team.lead_agent_id ? 1 : 0);
              return (
                <button
                  key={team.id}
                  className={classNames(
                    rowStyles.row,
                    styles.teamRow,
                    activePane.type === WorkspacePaneTypes.team && activePane.id === team.id && rowStyles.active,
                  )}
                  onClick={() => onSelectTeam?.(team)}
                >
                  <span className={rowStyles.icon}>
                    <UsersIcon />
                  </span>
                  <span className={rowStyles.main}>
                    <span className={classNames(rowStyles.title, "truncate")}>{displayTeam(team)}</span>
                    <span className={classNames(rowStyles.meta, "truncate")}>
                      {t("teamMembersCount", { count: memberCount })} · {team.status}
                    </span>
                  </span>
                </button>
              );
            })
          ) : (
            <div className={styles.empty}>{teams.length ? t("workspaceSearchNoResults") : t("noTeams")}</div>
          )}
        </WorkspaceGroup>
      );
    }
    const computerVisible = matchesSearch(
      normalizedContextQuery,
      t("localComputer"),
      t("computersSection"),
      t("computerAgentsSection"),
    );
    return (
      <WorkspaceGroup
        key={id}
        id="computers"
        title={t("computersSection")}
        count={1}
        collapsed={flat ? false : Boolean(collapsedWorkspaceGroups.computers)}
        onToggle={() => onToggleWorkspaceGroup("computers")}
        presentation={presentation}
        {...(flat ? {} : sectionDragProps(SectionPanels.agents, id))}
      >
        {computerVisible ? (
          <WorkspaceComputerRow
            title={t("localComputer")}
            active={activePane.type === WorkspacePaneTypes.computer}
            subtitle={`${t("computerAgentsSection")} ${agentItems.length}`}
            onSelect={onSelectComputer}
          />
        ) : (
          <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
        )}
      </WorkspaceGroup>
    );
  }

  function renderHubTemplateSection(presentation: "group" | "flat" = "group") {
    const flat = presentation === "flat";
    const visibleTemplates = resourcesTemplates.filter(templateMatchesQuery);
    const renderedTemplates = flat ? visibleTemplates : visibleTemplates.slice(0, 6);
    return (
      <WorkspaceGroup
        id="hub-templates"
        title={t("resourcesTemplatesSection")}
        count={resourcesTemplates.length}
        collapsed={flat ? false : Boolean(collapsedWorkspaceGroups["hub-templates"])}
        onToggle={() => onToggleWorkspaceGroup("hub-templates")}
        presentation={presentation}
      >
        {resourcesError ? (
          <div className={styles.empty}>{resourcesError}</div>
        ) : resourcesLoaded && resourcesTemplates.length === 0 && resourcesSkills.length === 0 ? (
          <div className={styles.empty}>{t("resourcesEmpty")}</div>
        ) : resourcesLoaded && resourcesTemplates.length > 0 && !visibleTemplates.length ? (
          <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
        ) : (
          renderedTemplates.map((item) => (
            <button
              key={item.id}
              className={classNames(
                rowStyles.row,
                styles.hubTemplateRow,
                resourcesPaneActive &&
                  selectedHubTemplateId === item.id &&
                  selectedHubResourceType === "template" &&
                  rowStyles.active,
              )}
              onClick={() => onSelectHubTemplate(item)}
            >
              <span className={rowStyles.icon}>
                <ModelsIcon />
              </span>
              <span className={rowStyles.main}>
                <span className={classNames(rowStyles.title, "truncate")}>{item.name || item.id}</span>
                <span className={classNames(rowStyles.meta, "truncate")}>
                  {item.description || item.source?.name || item.id}
                </span>
              </span>
              <span className={styles.templateSourceBadge}>
                <span className={styles.templateSourceBadgeDot} aria-hidden="true"></span>
                {localizeTemplateSourceTag(item.source?.name, locale)}
              </span>
            </button>
          ))
        )}
      </WorkspaceGroup>
    );
  }

  function renderHubSkillSection(presentation: "group" | "flat" = "group") {
    const flat = presentation === "flat";
    const visibleSkills = resourcesSkills.filter(skillMatchesQuery);
    return (
      <WorkspaceGroup
        id="hub-skills"
        title={t("resourcesSkillsLabel")}
        count={resourcesSkills.length}
        collapsed={flat ? false : Boolean(collapsedWorkspaceGroups["hub-skills"])}
        onToggle={() => onToggleWorkspaceGroup("hub-skills")}
        onAdd={() => setResolvedSkillUploadOpen(true)}
        addLabel={t("resourcesSkillUpload")}
        addIcon={<Plus size={15} strokeWidth={2.2} aria-hidden="true" />}
        presentation={presentation}
      >
        {visibleSkills.length ? (
          visibleSkills.map((item) => (
            <button
              key={item.name}
              className={classNames(
                rowStyles.row,
                styles.hubTemplateRow,
                styles.hubSkillRow,
                resourcesPaneActive &&
                  selectedHubSkillName === item.name &&
                  selectedHubResourceType === "skill" &&
                  rowStyles.active,
              )}
              onClick={() => onSelectHubSkill(item)}
            >
              <span className={rowStyles.icon}>
                <FileCode2 size={16} strokeWidth={2} aria-hidden="true" />
              </span>
              <span className={rowStyles.main}>
                <span className={classNames(rowStyles.title, "truncate")}>{item.name}</span>
                <span className={classNames(rowStyles.meta, "truncate")}>{item.description || item.name}</span>
              </span>
            </button>
          ))
        ) : resourcesSkillsError ? (
          <div className={styles.empty}>{resourcesSkillsError}</div>
        ) : resourcesSkills.length ? (
          <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
        ) : flat || (resourcesLoaded && resourcesTemplates.length === 0) ? (
          <div className={styles.empty}>{t("resourcesSkillsEmpty")}</div>
        ) : null}
      </WorkspaceGroup>
    );
  }

  function renderSkillUploadDialog() {
    return (
      <SkillUploadDialog
        open={resolvedSkillUploadOpen}
        onOpenChange={setResolvedSkillUploadOpen}
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
    );
  }

  function renderContextSectionPanel(sectionId: WorkspaceContextSectionId) {
    if (isAgentSectionId(sectionId)) {
      return (
        <div
          className={classNames(styles.panel, styles.flatPanel)}
          role="tabpanel"
          aria-label={contextSectionLabel(sectionId)}
        >
          {renderAgentSection(sectionId, "flat")}
        </div>
      );
    }
    if (isHubSectionId(sectionId)) {
      return (
        <div
          className={classNames(styles.panel, styles.flatPanel)}
          role="tabpanel"
          aria-label={contextSectionLabel(sectionId)}
        >
          {sectionId === HubSectionIds.templates ? renderHubTemplateSection("flat") : null}
          {sectionId === HubSectionIds.skills ? renderHubSkillSection("flat") : null}
          {sectionId === HubSectionIds.models ? renderModelProviderSection("flat") : null}
          {renderSkillUploadDialog()}
        </div>
      );
    }
    return null;
  }

  function isAgentSectionId(value: WorkspaceContextSectionId): value is AgentSectionId {
    return (
      value === AgentSectionIds.agents ||
      value === AgentSectionIds.humans ||
      value === AgentSectionIds.notifications ||
      value === AgentSectionIds.teams ||
      value === AgentSectionIds.computers
    );
  }

  function isHubSectionId(value: WorkspaceContextSectionId): value is HubSectionId {
    return value === HubSectionIds.templates || value === HubSectionIds.skills || value === HubSectionIds.models;
  }

  function contextSectionLabel(sectionId: WorkspaceContextSectionId) {
    if (sectionId === AgentSectionIds.agents) {
      return t("computerAgentsSection");
    }
    if (sectionId === AgentSectionIds.humans) {
      return t("humanSection");
    }
    if (sectionId === AgentSectionIds.notifications) {
      return t("notificationsSection");
    }
    if (sectionId === AgentSectionIds.teams) {
      return t("teamsSection");
    }
    if (sectionId === AgentSectionIds.computers) {
      return t("computersSection");
    }
    if (sectionId === HubSectionIds.templates) {
      return t("resourcesTemplatesSection");
    }
    if (sectionId === HubSectionIds.skills) {
      return t("resourcesSkillsLabel");
    }
    if (sectionId === HubSectionIds.models) {
      return t("resourcesModelProvidersSection");
    }
    return t("messagesTab");
  }

  const contextPanel = contextSectionId ? renderContextSectionPanel(contextSectionId) : null;

  return (
    <>
      {contextPanel ? (
        contextPanel
      ) : workspaceTab === WorkspaceTabs.messages ? (
        <div className={styles.panel} role="tabpanel" aria-label={t("messagesTab")}>
          {sectionOrders.messages.map(renderMessageSection)}
        </div>
      ) : workspaceTab === WorkspaceTabs.threads ? (
        <div className={styles.panel} role="tabpanel" aria-label={t("threadsTab")}>
          {threadGroups.length ? (
            threadGroups
              .map((group) => ({
                ...group,
                threads: group.threads.filter((thread) => threadMatchesQuery(group, thread)),
              }))
              .filter((group) => group.threads.length || conversationMatchesQuery(group.conversation))
              .map((group) => {
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
            <div className={styles.empty}>{t("noThreads")}</div>
          )}
        </div>
      ) : workspaceTab === WorkspaceTabs.hub ? (
        <div className={styles.panel} role="tabpanel" aria-label={t("resourcesTab")}>
          {renderHubTemplateSection()}
          {renderHubSkillSection()}
          {renderModelProviderSection()}
          {renderSkillUploadDialog()}
        </div>
      ) : workspaceTab === WorkspaceTabs.tasks ? (
        <div className={styles.panel} role="tabpanel" aria-label={t("tasksTab")}>
          <WorkspaceGroup
            id="tasks"
            title={t("tasksTab")}
            count={taskCount + scheduledTaskCount}
            collapsed={Boolean(collapsedWorkspaceGroups.tasks)}
            onToggle={() => onToggleWorkspaceGroup("tasks")}
            onAdd={
              activeTaskBoardView === "scheduled" && onOpenCreateScheduledTask
                ? onOpenCreateScheduledTask
                : onOpenCreateTask
            }
            addLabel={
              activeTaskBoardView === "scheduled" && onOpenCreateScheduledTask
                ? t("scheduledTaskCreate")
                : t("taskCreate")
            }
            addIcon={<Plus size={15} strokeWidth={2.2} aria-hidden="true" />}
          >
            {matchesSearch(normalizedContextQuery, t("normalTasksTab")) ? (
              <button
                type="button"
                className={classNames(
                  rowStyles.row,
                  styles.taskSectionRow,
                  activeTaskBoardView === "tasks" && rowStyles.active,
                )}
                onClick={() => {
                  onSelectTaskBoardView("tasks");
                  onSelectTask();
                }}
              >
                <span className={rowStyles.main}>
                  <span className={classNames(rowStyles.title, "truncate")}>{t("normalTasksTab")}</span>
                </span>
                <span className={rowStyles.rowCount}>{taskCount}</span>
              </button>
            ) : null}
            {matchesSearch(normalizedContextQuery, t("scheduledTasksTab")) ? (
              <button
                type="button"
                className={classNames(
                  rowStyles.row,
                  styles.taskSectionRow,
                  activeTaskBoardView === "scheduled" && rowStyles.active,
                )}
                onClick={() => {
                  onSelectTaskBoardView("scheduled");
                  onSelectTask();
                }}
              >
                <span className={rowStyles.main}>
                  <span className={classNames(rowStyles.title, "truncate")}>{t("scheduledTasksTab")}</span>
                </span>
                <span className={rowStyles.rowCount}>{scheduledTaskCount}</span>
              </button>
            ) : null}
            {!matchesSearch(normalizedContextQuery, t("normalTasksTab"), t("scheduledTasksTab")) ? (
              <div className={styles.empty}>{t("workspaceSearchNoResults")}</div>
            ) : null}
          </WorkspaceGroup>
        </div>
      ) : (
        <div className={styles.panel} role="tabpanel" aria-label={t("agentsTab")}>
          {sectionOrders.agents.map((sectionId) => renderAgentSection(sectionId))}
        </div>
      )}
      {agentsError ? <div className="form-error agent-error">{agentsError}</div> : null}
    </>
  );
}
