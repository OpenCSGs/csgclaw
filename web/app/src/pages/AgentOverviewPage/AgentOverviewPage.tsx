import { resolveTeamAvatarMembers, teamMemberIDs, teamStatusLabel } from "@/models/tasks";
import { Fragment, lazy, Suspense, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ExternalLink, List, LoaderCircle, LogIn, Plus, Search } from "lucide-react";
import { apiErrorCode } from "@/api/client";
import { Button, LinkButton, PanelHeader, EmptyState, TextInput } from "@/components/ui";
import { AgentAvatarContent, AgentAvatarImage } from "@/components/business/AgentAvatar";
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { useWorkspaceHubTemplateQuery, useWorkspaceHubTemplatesQuery } from "@/hooks/workspace/workspaceQueries";
import type { WorkspaceControllerReady } from "@/hooks/workspace/workspaceControllerContract";
import { isVisibleInHubTemplateList } from "@/models/hubWorkspace";
import { agenticHubTemplatesURL } from "@/models/agenticHub";
import { isAuthenticated } from "@/models/auth";
import { templateAvatar, templateInGroup } from "@/models/templateGroups";
import { agentAvailabilityStatusLabel, agentRoleLabel, isAgentAvailable, type AgentLike } from "@/models/agents";
import { AgentPlaza, AgentPlazaLayout, CloudAgentDetail } from "./components/AgentPlaza";
import { useCloudAgents } from "@/hooks/workspace/useCloudAgents";
import { filterCloudAgents } from "@/models/cloudAgents";
import {
  agentTemplatesSearch,
  agentPlazaSearch,
  cloudAgentSearch,
  pathForNewCloudTask,
  pathForPane,
  WorkspacePaneTypes,
} from "@/models/routing";
import { defaultAgentAvatar } from "@/shared/avatarOptions";

import styles from "./AgentOverviewPage.module.css";
import { useAgentViewBindings } from "@/hooks/workspace/useAgentViewBindings";
import { AgentOverviewGuide } from "./components";
import { useAgentOverviewListState } from "./useAgentOverviewListState";

const AgentView = lazy(() =>
  import("@/components/business/AgentDetails").then((module) => ({ default: module.AgentView })),
);
const TeamDetailPane = lazy(() =>
  import("@/components/business/TeamDetailPane").then((module) => ({ default: module.TeamDetailPane })),
);
const ConversationView = lazy(() =>
  import("@/components/business/ConversationWorkspace").then((module) => ({ default: module.ConversationView })),
);
const HubDetailPane = lazy(() =>
  import("@/components/business/HubDetailPane").then((module) => ({ default: module.HubDetailPane })),
);

function CloudSourceTemplate({
  repoPath,
  hubViewProps,
  renderProfileIntro,
  instanceModel,
}: {
  repoPath: string;
  hubViewProps: WorkspaceControllerReady["hubViewProps"];
  renderProfileIntro: (showModel: boolean) => ReactNode;
  instanceModel?: string;
}) {
  const query = useWorkspaceHubTemplateQuery(repoPath);
  const template = query.data;
  const { onSelectTemplateInPlace, t } = hubViewProps;
  useEffect(() => {
    onSelectTemplateInPlace({ id: repoPath });
  }, [onSelectTemplateInPlace, repoPath]);
  if (query.isError || (template && (template.id !== repoPath || template.source?.kind !== "remote"))) {
    return (
      <>
        {renderProfileIntro(true)}
        <EmptyState
          title={t("agentPlazaTemplateFailed")}
          actions={
            <Button variant="tertiaryGray" size="sm" loading={query.isFetching} onClick={() => void query.refetch()}>
              {t("retry")}
            </Button>
          }
        />
      </>
    );
  }
  const selected = hubViewProps.hub.detailPaneProps;
  if (!template || selected.selectedTemplateId !== repoPath || selected.selectedTemplate?.id !== repoPath) {
    return (
      <>
        {renderProfileIntro(true)}
        <EmptyState title={t("agentPlazaTemplateLoading")} />
      </>
    );
  }
  return (
    <Suspense fallback={renderProfileIntro(true)}>
      <HubDetailPane
        {...hubViewProps}
        inlineTemplateDetail
        hideInlineTemplateHeader
        inlineTemplateProfileIntro={renderProfileIntro(false)}
        inlineTemplateInstanceModel={instanceModel}
      />
    </Suspense>
  );
}

export function AgentOverviewPage() {
  const controller = useWorkspaceControllerContext();
  const agentViewProps = useAgentViewBindings();
  const location = useLocation();
  const navigate = useNavigate();
  const searchParams = new URLSearchParams(location.search);
  const requestedTab = searchParams.get("tab");
  const plazaTab = requestedTab === "plaza";
  const cloudID = searchParams.get("cloud") || "";
  const plaza = useCloudAgents(controller.sidebarProps?.authStatus);
  const selectedCloudAgent = plaza.agents.find((agent) => agent.id === cloudID);
  const templatesTab = requestedTab === "templates" || requestedTab === "market";
  const requestedTemplateID = templatesTab ? searchParams.get("template") || "" : "";
  const { query, filter, setListState } = useAgentOverviewListState();
  const activeID = controller.ready ? controller.activePane.id : "";
  const templateFilter = searchParams.get("filter") === "personal" ? "personal" : "official";
  const [listOpen, setListOpen] = useState(false);
  const listToggleRef = useRef<HTMLButtonElement>(null);
  const closeList = () => {
    setListOpen(false);
    if (listOpen) listToggleRef.current?.focus();
  };
  useEffect(() => {
    if (!listOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setListOpen(false);
        listToggleRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [listOpen]);
  const templatesQuery = useWorkspaceHubTemplatesQuery({ enabled: templatesTab });
  const templates = useMemo(
    () => (templatesQuery.data ?? []).filter(isVisibleInHubTemplateList),
    [templatesQuery.data],
  );
  const hubViewProps = controller.ready ? controller.hubViewProps : undefined;
  const currentTemplateID = hubViewProps?.hub.detailPaneProps.selectedTemplateId;
  const currentResourceType = hubViewProps?.hub.detailPaneProps.selectedResourceType;
  const onSelectTemplateInPlace = hubViewProps?.onSelectTemplateInPlace;
  const selectedTemplateRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!requestedTemplateID || (requestedTemplateID === currentTemplateID && currentResourceType === "template"))
      return;
    const template = templates.find((item) => item.id === requestedTemplateID);
    if (template) onSelectTemplateInPlace?.(template);
  }, [requestedTemplateID, currentTemplateID, currentResourceType, templates, onSelectTemplateInPlace]);
  useEffect(() => {
    selectedTemplateRef.current?.scrollIntoView({ block: "nearest" });
  }, [requestedTemplateID, templates, query]);
  if (!controller.ready || !controller.sidebarProps) return null;
  const props = controller.sidebarProps;
  const { t } = props;
  const authed = isAuthenticated(props.authStatus) && !props.authExpired;
  const selectedTemplateID = requestedTemplateID;
  const matches = (name: string) => name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase());
  const agents = props.workerAgentItems.filter((agent) => matches(agent.name || agent.id || ""));
  const teams = props.teams.filter((team) => matches(team.title));
  const templateUsername = props.authStatus?.authenticated ? props.authStatus.user_id : "";
  const filterCounts: Record<string, number> = {
    all: props.workerAgentItems.length + props.teams.length + plaza.agents.length,
    cloud: plaza.agents.length,
    agents: props.workerAgentItems.length,
    teams: props.teams.length,
    official: templates.filter((item) => templateInGroup(item, "official", templateUsername)).length,
    personal: templates.filter((item) => templateInGroup(item, "mine", templateUsername)).length,
  };
  const templatesURL = agenticHubTemplatesURL(
    props.authStatus?.authenticated ? props.authStatus.opencsg_base_url || props.authStatus.base_url : "",
  );
  const shownTemplates = templates.filter(
    (item) =>
      matches([item.display_name, item.name, item.id, item.description, item.runtime_kind].filter(Boolean).join(" ")) &&
      templateInGroup(item, templateFilter === "official" ? "official" : "mine", templateUsername),
  );
  const cloudTemplates = shownTemplates.filter((item) => item.source?.kind === "remote");
  const orderedTemplates =
    templateFilter === "personal"
      ? [...shownTemplates.filter((item) => item.source?.kind !== "remote"), ...cloudTemplates]
      : shownTemplates;
  const templatesLoading = templatesQuery.isLoading && !templates.length;
  const templatesFailed = templatesQuery.isError && !templates.length;
  const templateEmptyTitle = query.trim()
    ? t("workspaceSearchNoResults")
    : t(templateFilter === "official" ? "iaTemplatesOfficialEmpty" : "iaTemplatesPersonalEmpty");
  const templateEmptyHint = query.trim()
    ? t("iaTemplatesSearchHint")
    : t(templateFilter === "official" ? "iaTemplatesOfficialEmptyHint" : "iaTemplatesPersonalEmptyHint");
  function selectTab(value: string) {
    const params = new URLSearchParams(location.search);
    if (value === "mine") {
      params.delete("tab");
      params.delete("filter");
      params.delete("template");
    } else params.set("tab", value === "plaza" ? "plaza" : "templates");
    params.delete("cloud");
    navigate(
      {
        pathname: value === "plaza" ? pathForPane({ type: WorkspacePaneTypes.agent }) : location.pathname,
        search: params.toString(),
      },
      { state: { ...location.state, agentOverviewList: { query: "", filter: "all" } } },
    );
  }
  const chooseAgent = (item: AgentLike) => {
    closeList();
    props.onSelectAgent(item);
  };
  return (
    <section className={styles.page}>
      <PanelHeader className={styles.header}>
        {!plazaTab ? (
          <Button
            ref={listToggleRef}
            className={styles.listToggle}
            variant="secondaryGray"
            size="sm"
            aria-expanded={listOpen}
            aria-controls="agent-overview-list"
            onClick={() => setListOpen((open) => !open)}
          >
            <List size={16} />
            {t("iaAgentsToggleList")}
          </Button>
        ) : null}
        <div className={styles.tabs} role="tablist" aria-label={t("agentsTab")}>
          <button role="tab" aria-selected={!templatesTab && !plazaTab} onClick={() => selectTab("mine")}>
            {t("iaAgentsMineTab")} <small>{filterCounts.all}</small>
          </button>
          <button role="tab" aria-selected={templatesTab} onClick={() => selectTab("templates")}>
            {t("iaAgentsMarketTab")} {templatesQuery.data !== undefined ? <small>{templates.length}</small> : null}
          </button>
          <button role="tab" aria-selected={plazaTab} onClick={() => selectTab("plaza")}>
            {t("agentPlazaTitle")} <span className={styles.cloudTag}>{t("agentPlazaCloud")}</span>
          </button>
        </div>
        {!templatesTab && !plazaTab ? (
          <div className={styles.actions}>
            <Button variant="secondaryGray" size="sm" onClick={() => void props.onCreateCustomAgent()}>
              <Plus size={14} />
              {t("iaAgentsCreateAgent")}
            </Button>
            <Button variant="secondaryGray" size="sm" onClick={props.onOpenCreateTeam}>
              <Plus size={14} />
              {t("taskEntryCreateTeam")}
            </Button>
          </div>
        ) : null}
      </PanelHeader>
      {plaza.storageError ? (
        <p role="alert" className={styles.emptyList}>
          {t("agentPlazaStorageError")}
        </p>
      ) : null}
      {plazaTab && (!authed || apiErrorCode(plaza.error) === "relay_login_required") ? (
        <AgentPlazaLayout t={t}>
          <div className={styles.loginGate}>
            <LogIn size={20} strokeWidth={1.8} aria-hidden="true" />
            <p>{t("agentPlazaLoginRequired")}</p>
            <Button
              variant="primary"
              size="md"
              disabled={props.authBusy || props.authPending}
              onClick={() => {
                if (props.onRequireOpenCSGAuthentication?.() !== false) void props.onLogin();
              }}
            >
              {t("modelProviderOpenCSGAuthorize")}
            </Button>
          </div>
        </AgentPlazaLayout>
      ) : plazaTab && plaza.loading ? (
        <EmptyState title={t("agentPlazaLoading")} />
      ) : plazaTab && plaza.error ? (
        <EmptyState title={t("agentPlazaLoadFailed")} actions={<Button onClick={plaza.refresh}>{t("retry")}</Button>} />
      ) : plazaTab ? (
        <AgentPlaza
          agents={plaza.catalog}
          addedIDs={plaza.addedIDs}
          t={t}
          onAdd={(id) => {
            plaza.add(id);
          }}
          onOpen={(id) =>
            navigate({ pathname: pathForPane({ type: WorkspacePaneTypes.agent }), search: cloudAgentSearch(id) })
          }
        />
      ) : (
        <div className={styles.split}>
          {listOpen ? (
            <button type="button" className={styles.listBackdrop} aria-label={t("close")} onClick={closeList} />
          ) : null}
          <aside
            id="agent-overview-list"
            className={`${styles.list} ${listOpen ? styles.listOpen : ""}`}
            aria-busy={templatesTab && templatesLoading}
          >
            <div className={styles.listControls}>
              <label className={styles.search}>
                <Search size={15} />
                <TextInput
                  variant="plain"
                  size="sm"
                  aria-label={t(templatesTab ? "resourcesTemplateSearchPlaceholder" : "taskEntrySearchTarget")}
                  placeholder={t(templatesTab ? "resourcesTemplateSearchPlaceholder" : "taskEntrySearchTarget")}
                  value={query}
                  onChange={(event) => {
                    const query = event.target.value;
                    setListState((state) => ({ ...state, query }));
                  }}
                />
              </label>
              <nav className={styles.filters} aria-label={t("agentsTab")}>
                {(templatesTab ? ["official", "personal"] : ["all", "agents", "cloud", "teams"]).map((value) => (
                  <button
                    key={value}
                    type="button"
                    aria-pressed={(templatesTab ? templateFilter : filter) === value}
                    onClick={() => {
                      if (!templatesTab) {
                        setListState((state) => ({ ...state, filter: value }));
                        return;
                      }
                      const params = new URLSearchParams(location.search);
                      params.set("filter", value);
                      params.delete("template");
                      navigate({ search: params.toString() });
                    }}
                  >
                    {t(value === "agents" ? "agentPlazaLocalAgents" : `taskEntryOverview_${value}`)}{" "}
                    {!templatesTab || templatesQuery.data !== undefined ? <small>{filterCounts[value]}</small> : null}
                  </button>
                ))}
              </nav>
            </div>
            <div className={styles.listEntries}>
              {templatesTab ? (
                <>
                  {templatesLoading ? (
                    <div>
                      <p className={styles.listStatus}>{t("iaTemplatesLoading")}</p>
                      <div className={styles.skeletonList} aria-hidden="true">
                        {[0, 1, 2, 3].map((index) => (
                          <div key={index} className={styles.skeletonRow}>
                            <span />
                            <span />
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : templatesQuery.isFetching ? (
                    <p className={styles.listStatus} role="status">
                      {t("iaTemplatesRefreshing")}
                    </p>
                  ) : null}
                  {templatesQuery.isError ? (
                    <div className={styles.listError} role="alert">
                      <p>{t(templatesFailed ? "iaTemplatesLoadFailed" : "iaTemplatesRefreshFailed")}</p>
                      <Button size="sm" onClick={() => void templatesQuery.refetch()}>
                        {t("retry")}
                      </Button>
                    </div>
                  ) : null}
                  {orderedTemplates.map((item) => (
                    <Fragment key={item.id}>
                      {templateFilter === "personal" && item === cloudTemplates[0] ? (
                        <h3 className={styles.cloudTemplatesHeading}>
                          {t("iaTemplatesCloudOnly", { count: cloudTemplates.length })}
                        </h3>
                      ) : null}
                      <button
                        type="button"
                        ref={selectedTemplateID === item.id ? selectedTemplateRef : undefined}
                        aria-current={selectedTemplateID === item.id ? "true" : undefined}
                        className={`${styles.row} ${selectedTemplateID === item.id ? styles.active : ""}`}
                        onClick={() => {
                          controller.hubViewProps?.onSelectTemplateInPlace(item);
                          navigate({ search: agentTemplatesSearch(item.id || "", templateFilter) });
                          closeList();
                        }}
                      >
                        <span className={styles.avatar} aria-hidden="true">
                          <AgentAvatarImage avatar={templateAvatar(item) || defaultAgentAvatar()} />
                        </span>
                        <span className={styles.identity}>
                          <span className={styles.templateName}>
                            <strong>{item.display_name?.trim() || item.name || item.id}</strong>{" "}
                            {templateFilter === "personal" && item.source?.kind === "remote" ? (
                              <span className={styles.cloudTag}>{t("iaTemplateCloudTag")}</span>
                            ) : null}
                          </span>
                          <small>{item.description}</small>
                        </span>
                      </button>
                    </Fragment>
                  ))}
                  {templateFilter === "personal" && templatesURL ? (
                    <LinkButton
                      className={styles.manageTemplatesLink}
                      variant="linkColor"
                      size="sm"
                      href={templatesURL}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <ExternalLink size={14} aria-hidden="true" />
                      {t("iaTemplateManageOnHub")}
                    </LinkButton>
                  ) : null}
                  {!templatesQuery.isLoading && !templatesQuery.isError && !shownTemplates.length ? (
                    <p className={styles.emptyList}>{templateEmptyTitle}</p>
                  ) : null}
                </>
              ) : (
                <>
                  {filter === "all" || filter === "agents"
                    ? agents.map((item) => (
                        <button
                          type="button"
                          key={item.id}
                          className={`${styles.row} ${activeID === item.id ? styles.active : ""}`}
                          onClick={() => chooseAgent(item)}
                        >
                          <span className={styles.avatar}>
                            <AgentAvatarContent avatar={item.avatar} fallback={item.name?.slice(0, 1) || "A"} />
                          </span>
                          <span className={styles.identity}>
                            <span className={styles.rowName}>
                              <strong>{item.name}</strong>
                              <span
                                className={styles.agentStatus}
                                data-available={isAgentAvailable(item)}
                                role="img"
                                aria-label={agentAvailabilityStatusLabel(item, t)}
                                title={agentAvailabilityStatusLabel(item, t)}
                              />
                            </span>
                            <small>{agentRoleLabel(item, t)}</small>
                          </span>
                        </button>
                      ))
                    : null}
                  {filter === "all" || filter === "cloud"
                    ? filterCloudAgents(plaza.agents, query).map((item) => (
                        <button
                          type="button"
                          key={item.id}
                          aria-current={cloudID === item.id ? "true" : undefined}
                          className={`${styles.row} ${cloudID === item.id ? styles.active : ""}`}
                          onClick={() => {
                            closeList();
                            navigate({
                              pathname: pathForPane({ type: WorkspacePaneTypes.agent }),
                              search: cloudAgentSearch(item.id),
                            });
                          }}
                        >
                          <span className={styles.avatar}>
                            <AgentAvatarContent avatar={item.avatar} fallback={item.name.slice(0, 1)} />
                          </span>
                          <span className={styles.identity}>
                            <span className={styles.rowName}>
                              <strong>{item.name}</strong>
                              <span
                                className={styles.agentStatus}
                                data-available={item.status === "online"}
                                role="img"
                                aria-label={t(item.status === "online" ? "agentPlazaOnline" : "agentPlazaOffline")}
                              />
                              <span className={styles.cloudTag}>{t("agentPlazaCloud")}</span>
                            </span>
                            <small>{item.description}</small>
                          </span>
                        </button>
                      ))
                    : null}
                  {filter === "all" || filter === "teams"
                    ? teams.map((item) => (
                        <button
                          type="button"
                          key={item.id}
                          className={`${styles.row} ${activeID === item.id ? styles.active : ""}`}
                          onClick={() => {
                            closeList();
                            props.onSelectTeam(item);
                          }}
                        >
                          <RoomAvatar
                            members={resolveTeamAvatarMembers(item, props.workerAgentItems, props.usersById)}
                            size={28}
                            showCountBadge={false}
                          />
                          <span className={styles.identity}>
                            <span className={styles.rowName}>
                              <strong>{item.title}</strong>
                              <span className={styles.teamBadge}>{t("iaAgentsTeamBadge")}</span>
                            </span>
                            <small>
                              {t("iaAgentsTeamSummary", {
                                count: teamMemberIDs(item).length,
                                status: teamStatusLabel(item.status, t),
                              })}
                            </small>
                          </span>
                        </button>
                      ))
                    : null}
                </>
              )}
            </div>
          </aside>
          <div className={styles.detail}>
            {templatesTab ? (
              selectedTemplateID && selectedTemplateID === currentTemplateID && currentResourceType === "template" ? (
                <Suspense fallback={null}>
                  <HubDetailPane {...controller.hubViewProps} inlineTemplateDetail />
                </Suspense>
              ) : !templatesLoading && !templatesFailed && shownTemplates.length ? (
                <AgentOverviewGuide templates t={t} />
              ) : (
                <EmptyState
                  className={styles.empty}
                  icon={templatesLoading ? <LoaderCircle className={styles.loadingIcon} size={28} /> : undefined}
                  title={
                    templatesLoading
                      ? t("iaTemplatesLoading")
                      : templatesFailed
                        ? t("iaTemplatesLoadFailed")
                        : templateEmptyTitle
                  }
                  description={
                    templatesLoading
                      ? t("iaTemplatesLoadingHint")
                      : templatesFailed
                        ? t("iaTemplatesRetryHint")
                        : templateEmptyHint
                  }
                  actions={
                    templatesFailed ? (
                      <Button size="sm" onClick={() => void templatesQuery.refetch()}>
                        {t("retry")}
                      </Button>
                    ) : undefined
                  }
                />
              )
            ) : selectedCloudAgent ? (
              <CloudAgentDetail
                key={`${selectedCloudAgent.connectionID}:${selectedCloudAgent.id}`}
                agent={selectedCloudAgent}
                t={t}
                renderTemplate={(repoPath, renderProfileIntro) => (
                  <CloudSourceTemplate
                    repoPath={repoPath}
                    hubViewProps={controller.hubViewProps}
                    renderProfileIntro={renderProfileIntro}
                    instanceModel={selectedCloudAgent.modelID}
                  />
                )}
                onNewTask={() => navigate(pathForNewCloudTask(selectedCloudAgent.id))}
                onRemove={() => {
                  if (plaza.remove(selectedCloudAgent.id))
                    navigate({ pathname: pathForPane({ type: WorkspacePaneTypes.agent }), search: "" });
                }}
              />
            ) : cloudID ? (
              <EmptyState
                title={t("agentPlazaNotAdded")}
                actions={
                  <Button
                    onClick={() =>
                      navigate({
                        pathname: pathForPane({ type: WorkspacePaneTypes.agent }),
                        search: agentPlazaSearch(),
                      })
                    }
                  >
                    {t("agentPlazaTitle")}
                  </Button>
                }
              />
            ) : activeID && controller.activePane.type === "agent" ? (
              <Suspense fallback={null}>
                {agentViewProps ? (
                  <AgentView {...agentViewProps} />
                ) : (
                  <ConversationView {...controller.conversationViewProps} />
                )}
              </Suspense>
            ) : activeID && controller.activePane.type === "team" ? (
              <Suspense fallback={null}>
                <TeamDetailPane {...controller.teamViewProps} />
              </Suspense>
            ) : (
              <AgentOverviewGuide t={t} />
            )}
          </div>
        </div>
      )}
    </section>
  );
}
