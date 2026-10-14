import { resolveTeamAvatarMembers } from "@/models/tasks";
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ChevronRight, List, Plus, Search } from "lucide-react";
import { Button } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { useWorkspaceHubTemplatesQuery } from "@/hooks/workspace/workspaceQueries";
import { isVisibleInHubTemplateList } from "@/models/hubWorkspace";
import { agentRoleLabel, type AgentLike } from "@/models/agents";

import styles from "./AgentOverviewPage.module.css";

const AgentPage = lazy(() => import("@/pages/AgentPage").then((module) => ({ default: module.AgentPage })));
const TeamPage = lazy(() => import("@/pages/TeamPage").then((module) => ({ default: module.TeamPage })));
const HubDetailPane = lazy(() =>
  import("@/components/business/HubDetailPane").then((module) => ({ default: module.HubDetailPane })),
);

export function AgentOverviewPage() {
  const controller = useWorkspaceControllerContext();
  const location = useLocation();
  const navigate = useNavigate();
  const requestedTab = new URLSearchParams(location.search).get("tab");
  const templatesTab = requestedTab === "templates" || requestedTab === "market";
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("all");
  const [templateFilter, setTemplateFilter] = useState("official");
  const [hasSelectedTemplate, setHasSelectedTemplate] = useState(false);
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
  if (!controller.ready || !controller.sidebarProps) return null;
  const props = controller.sidebarProps;
  const { t } = props;
  const selectedTemplateID = hasSelectedTemplate ? controller.hubViewProps?.hub.detailPaneProps.selectedTemplateId : "";
  const matches = (name: string) => name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase());
  const agents = props.workerAgentItems.filter((agent) => matches(agent.name || agent.id || ""));
  const teams = props.teams.filter((team) => matches(team.title));
  const shownTemplates = templates.filter(
    (item) =>
      matches([item.name, item.id, item.description, item.runtime_kind].filter(Boolean).join(" ")) &&
      (templateFilter === "official"
        ? item.source?.name === "official" || item.source?.kind === "builtin"
        : item.source?.name !== "official" && item.source?.kind !== "builtin"),
  );
  const activeID = controller.activePane.id;
  function selectTab(value: string) {
    const params = new URLSearchParams(location.search);
    if (value === "mine") params.delete("tab");
    else params.set("tab", "templates");
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
      <header className={styles.header}>
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
            <Button size="sm" onClick={() => void props.onCreateAgent()}>
              <Plus size={14} />
              {t("createAgent")}
            </Button>
          </div>
        ) : null}
      </header>
      <div className={styles.split}>
        {listOpen ? (
          <button type="button" className={styles.listBackdrop} aria-label={t("close")} onClick={closeList} />
        ) : null}
        <aside id="agent-overview-list" className={`${styles.list} ${listOpen ? styles.listOpen : ""}`}>
          <label className={styles.search}>
            <Search size={15} />
            <input
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
                onClick={() => (templatesTab ? setTemplateFilter(value) : setFilter(value))}
              >
                {t(`taskEntryOverview_${value}`)}
              </button>
            ))}
          </nav>
          {templatesTab ? (
            <>
              {templatesQuery.isLoading ? <p role="status">{t("loading")}</p> : null}
              {templatesQuery.isError ? (
                <p role="alert">
                  {t("resourcesLoadFailed")}
                  <Button onClick={() => void templatesQuery.refetch()}>{t("retry")}</Button>
                </p>
              ) : null}
              {shownTemplates.map((item) => (
                <button
                  type="button"
                  key={item.id}
                  className={`${styles.row} ${selectedTemplateID === item.id ? styles.active : ""}`}
                  onClick={() => {
                    controller.hubViewProps?.onSelectTemplateInPlace(item);
                    setHasSelectedTemplate(true);
                    closeList();
                  }}
                >
                  <span className={styles.identity}>
                    <strong>{item.name}</strong>
                    <small>{item.description}</small>
                  </span>
                  <ChevronRight size={14} />
                </button>
              ))}
              {!templatesQuery.isLoading && !templatesQuery.isError && !shownTemplates.length ? (
                <p className={styles.emptyList}>{t("workspaceSearchNoResults")}</p>
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
        </aside>
        <div className={styles.detail}>
          {templatesTab ? (
            selectedTemplateID ? (
              <Suspense fallback={null}>
                <HubDetailPane {...controller.hubViewProps} inlineTemplateDetail />
              </Suspense>
            ) : (
              <div className={styles.empty}>
                <h2>{t("iaAgentsMarketTab")}</h2>
                <p>{t("iaAgentsSelectTemplate")}</p>
              </div>
            )
          ) : activeID && controller.activePane.type === "agent" ? (
            <Suspense fallback={null}>
              <AgentPage />
            </Suspense>
          ) : activeID && controller.activePane.type === "team" ? (
            <Suspense fallback={null}>
              <TeamPage />
            </Suspense>
          ) : (
            <div className={styles.empty}>
              <h2>{t("iaAgentsMineTab")}</h2>
              <p>{t("iaAgentsSelectAgent")}</p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
