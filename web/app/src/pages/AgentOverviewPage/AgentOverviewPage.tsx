import { resolveTeamAvatarMembers } from "@/models/tasks";
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ChevronRight, ExternalLink, List, LoaderCircle, Plus, Search } from "lucide-react";
import { Button, PanelHeader, EmptyState, TextInput } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { useWorkspaceHubTemplatesQuery } from "@/hooks/workspace/workspaceQueries";
import { isVisibleInHubTemplateList } from "@/models/hubWorkspace";
import { agenticHubTemplatesURL } from "@/models/agenticHub";
import { templateInGroup } from "@/models/templateGroups";
import { agentRoleLabel, type AgentLike } from "@/models/agents";
import { agentTemplatesSearch } from "@/models/routing";

import styles from "./AgentOverviewPage.module.css";
import { useAgentViewBindings } from "@/hooks/workspace/useAgentViewBindings";
import { AgentOverviewGuide } from "./components";

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

export function AgentOverviewPage() {
  const controller = useWorkspaceControllerContext();
  const agentViewProps = useAgentViewBindings();
  const location = useLocation();
  const navigate = useNavigate();
  const searchParams = new URLSearchParams(location.search);
  const requestedTab = searchParams.get("tab");
  const templatesTab = requestedTab === "templates" || requestedTab === "market";
  const requestedTemplateID = templatesTab ? searchParams.get("template") || "" : "";
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("all");
  const activeType = controller.ready ? controller.activePane.type : "";
  const activeID = controller.ready ? controller.activePane.id : "";
  useEffect(() => {
    if (!activeID || (activeType !== "agent" && activeType !== "team")) return;
    setFilter(activeType === "team" ? "teams" : "agents");
    setQuery("");
  }, [activeID, activeType]);
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
    setQuery("");
  }, [requestedTemplateID]);
  useEffect(() => {
    selectedTemplateRef.current?.scrollIntoView({ block: "nearest" });
  }, [requestedTemplateID, templates, query]);
  if (!controller.ready || !controller.sidebarProps) return null;
  const props = controller.sidebarProps;
  const { t } = props;
  const selectedTemplateID = requestedTemplateID;
  const matches = (name: string) => name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase());
  const agents = props.workerAgentItems.filter((agent) => matches(agent.name || agent.id || ""));
  const teams = props.teams.filter((team) => matches(team.title));
  const templateUsername = props.authStatus?.authenticated ? props.authStatus.user_id : "";
  const templatesURL = agenticHubTemplatesURL(
    props.authStatus?.authenticated ? props.authStatus.opencsg_base_url || props.authStatus.base_url : "",
  );
  const shownTemplates = templates.filter(
    (item) =>
      matches([item.display_name, item.name, item.id, item.description, item.runtime_kind].filter(Boolean).join(" ")) &&
      templateInGroup(item, templateFilter === "official" ? "official" : "mine", templateUsername),
  );
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
    } else params.set("tab", "templates");
    navigate({ search: params.toString() });
    setQuery("");
    setFilter("all");
  }
  const chooseAgent = (item: AgentLike) => {
    closeList();
    props.onSelectAgent(item);
  };
  return (
    <section className={styles.page}>
      <PanelHeader className={styles.header}>
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
        <div className={styles.tabs} role="tablist" aria-label={t("agentsTab")}>
          <button role="tab" aria-selected={!templatesTab} onClick={() => selectTab("mine")}>
            {t("iaAgentsMineTab")}
            <small>{props.workerAgentItems.length + props.teams.length}</small>
          </button>
          <button role="tab" aria-selected={templatesTab} onClick={() => selectTab("templates")}>
            {t("iaAgentsMarketTab")}
          </button>
        </div>
        {!templatesTab ? (
          <div className={styles.actions}>
            <Button variant="secondaryGray" size="sm" onClick={props.onOpenCreateTeam}>
              <Plus size={14} />
              {t("taskEntryCreateTeam")}
            </Button>
            <Button variant="secondaryGray" size="sm" onClick={() => void props.onCreateCustomAgent()}>
              <Plus size={14} />
              {t("iaAgentsCreateAgent")}
            </Button>
          </div>
        ) : null}
      </PanelHeader>
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
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            <nav className={styles.filters} aria-label={t("agentsTab")}>
              {(templatesTab ? ["official", "personal"] : ["all", "agents", "teams"]).map((value) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={(templatesTab ? templateFilter : filter) === value}
                  onClick={() => {
                    if (!templatesTab) {
                      setFilter(value);
                      return;
                    }
                    const params = new URLSearchParams(location.search);
                    params.set("filter", value);
                    params.delete("template");
                    navigate({ search: params.toString() });
                  }}
                >
                  {t(`taskEntryOverview_${value}`)}
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
                {shownTemplates.map((item) => (
                  <button
                    type="button"
                    key={item.id}
                    ref={selectedTemplateID === item.id ? selectedTemplateRef : undefined}
                    aria-current={selectedTemplateID === item.id ? "true" : undefined}
                    className={`${styles.row} ${selectedTemplateID === item.id ? styles.active : ""}`}
                    onClick={() => {
                      controller.hubViewProps?.onSelectTemplateInPlace(item);
                      navigate({ search: agentTemplatesSearch(item.id || "", templateFilter) });
                      closeList();
                    }}
                  >
                    <span className={styles.identity}>
                      <span className={styles.templateName}>
                        <strong>{item.display_name?.trim() || item.name || item.id}</strong>{" "}
                        {templateFilter === "personal" && item.source?.kind === "remote" ? (
                          <span className={styles.cloudTag}>{t("iaTemplateCloudTag")}</span>
                        ) : null}
                      </span>
                      <small>{item.description}</small>
                    </span>
                    <ChevronRight size={14} />
                  </button>
                ))}
                {templateFilter === "personal" && templatesURL ? (
                  <a
                    className={styles.manageTemplatesLink}
                    href={templatesURL}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <ExternalLink size={13} />
                    {t("iaTemplateManageOnHub")}
                  </a>
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
                          <strong>{item.name}</strong>
                          <small>{agentRoleLabel(item, t)}</small>
                        </span>
                        <ChevronRight size={14} />
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
                          <strong>{item.title}</strong>
                          <small>{t("taskEntryTeamMembers", { count: item.member_agent_ids.length })}</small>
                        </span>
                        <ChevronRight size={14} />
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
    </section>
  );
}
