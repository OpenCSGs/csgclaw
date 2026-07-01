import { useState } from "react";
import type { ReactNode } from "react";
import { ListChecks, Trash2, UserPlus, Users } from "lucide-react";
import { TaskSubtaskIndicator } from "@/components/business";
import { Button } from "@/components/ui";
import { AgentIcon, UsersIcon } from "@/components/ui/Icons";
import { isAgentRunning } from "@/models/agents";
import type { AgentLike } from "@/models/agents";
import type { IMUser, TranslateFn, UsersById } from "@/models/conversations";
import {
  displayTeam,
  formatTaskUpdatedAt,
  resolveTaskSidebarPhase,
  rootTasks,
  taskChildren,
  teamStatusLabel,
} from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTeam } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TeamDetailPane.module.css";

type UsersLookup = UsersById | Record<string, IMUser | undefined>;

export type TeamDetailPaneProps = {
  agents?: AgentLike[];
  onDeleteTeam?: (team: WorkspaceTeam) => void | Promise<boolean>;
  onManageMembers?: (team: WorkspaceTeam) => void;
  onSelectAgent?: (agent: AgentLike) => void;
  onSelectTask?: (taskID: string) => void;
  teamActionBusy?: boolean;
  teamActionError?: string;
  tasks?: WorkspaceTask[];
  team?: WorkspaceTeam | null;
  teamsLoading?: boolean;
  t?: TranslateFn;
  usersById?: UsersLookup;
};

type ActiveTeamTab = "members" | "records";

export function TeamDetailPane({
  t = (key) => key,
  team = null,
  teamsLoading = false,
  agents = [],
  usersById = new Map<string, IMUser>(),
  tasks = [],
  teamActionBusy = false,
  teamActionError = "",
  onDeleteTeam = () => {},
  onManageMembers = () => {},
  onSelectAgent = () => {},
  onSelectTask = () => {},
}: TeamDetailPaneProps) {
  const [activeTab, setActiveTab] = useState<ActiveTeamTab>("members");

  if (!team) {
    return (
      <section className={classNames("entity-pane", "team-detail-pane", styles.teamDetailPane)}>
        <div className="empty-state shell-empty-state">
          <strong>{teamsLoading ? t("teamsLoading") : t("teamDetailMissing")}</strong>
          <span>{t("teamDetailMissingHint")}</span>
        </div>
      </section>
    );
  }

  const memberIDs = teamMemberIDs(team);
  const members = memberIDs.map((memberID) => memberDisplay(memberID, agents, usersById, team.lead_agent_id));
  const parentTasks = rootTasks(tasks);
  const locale = document.documentElement.lang || "en";

  return (
    <section className={classNames("entity-pane", "team-detail-pane", styles.teamDetailPane)}>
      <header className={classNames("entity-header", styles.contentWidth, styles.teamDetailHeader)}>
        <div className={classNames("entity-avatar", styles.teamDetailAvatar)}>
          <UsersIcon />
        </div>
        <div className={classNames("entity-heading", styles.teamDetailHeading)}>
          <div className={classNames("entity-title-row", styles.teamDetailTitleRow)}>
            <h1 className={styles.teamDetailTitle}>{displayTeam(team)}</h1>
            <span
              className={classNames("status-pill", "online", styles.teamDetailStatus, styles.teamDetailStatusOnline)}
            >
              {teamStatusLabel(team.status, t)}
            </span>
          </div>
          <p className={styles.teamDetailSubtitle}>{t("teamDetailSubtitle")}</p>
        </div>
        <div className={classNames("entity-toolbar", styles.teamDetailToolbar)} aria-label={t("teamPanelManage")}>
          <Button
            className={styles.teamDetailToolbarButton}
            variant="secondaryGray"
            size="md"
            disabled={teamActionBusy}
            onClick={() => onManageMembers(team)}
          >
            <UserPlus size={16} aria-hidden="true" />
            {t("teamManageMembers")}
          </Button>
          <Button
            className={styles.teamDetailToolbarButton}
            variant="outlineDanger"
            size="md"
            disabled={teamActionBusy}
            onClick={() => void onDeleteTeam(team)}
          >
            <Trash2 size={16} aria-hidden="true" />
            {t("teamDelete")}
          </Button>
        </div>
      </header>

      {teamActionError ? (
        <div className={classNames("form-error", styles.contentWidth, styles.teamDetailActionError)} role="alert">
          {teamActionError}
        </div>
      ) : null}

      <div className={classNames(styles.contentWidth, styles.teamDetailLayout)}>
        <aside className={classNames(styles.panelSurface, styles.teamDetailSummary)}>
          <div className={styles.teamProfileBlock}>
            <div className={styles.teamProfileIcon}>
              <Users size={34} aria-hidden="true" />
            </div>
            <div>
              <h2>{displayTeam(team)}</h2>
              <p>{team.id}</p>
            </div>
          </div>
          <div className={styles.teamDetailFields}>
            <DetailField label={t("teamLeadLabel")} value={memberName(team.lead_agent_id, agents, usersById)} />
            <DetailField label={t("teamMembersLabel")} value={String(members.length)} />
            <DetailField label={t("teamCreatedLabel")} value={formatTaskUpdatedAt(team.created_at, locale)} />
            <DetailField label={t("teamUpdatedLabel")} value={formatTaskUpdatedAt(team.updated_at, locale)} />
          </div>
        </aside>

        <div className={classNames(styles.panelSurface, styles.teamDetailMain)}>
          <div className={styles.teamDetailTabs} role="tablist" aria-label={t("teamDetailTabsLabel")}>
            <button
              type="button"
              className={activeTab === "members" ? styles.active : ""}
              role="tab"
              id="team-detail-tab-members"
              aria-controls="team-detail-panel-members"
              aria-selected={activeTab === "members"}
              onClick={() => setActiveTab("members")}
            >
              <Users size={15} aria-hidden="true" />
              {t("teamMembersTab")}
            </button>
            <button
              type="button"
              className={activeTab === "records" ? styles.active : ""}
              role="tab"
              id="team-detail-tab-records"
              aria-controls="team-detail-panel-records"
              aria-selected={activeTab === "records"}
              onClick={() => setActiveTab("records")}
            >
              <ListChecks size={15} aria-hidden="true" />
              {t("teamRecordsTab")}
            </button>
          </div>
          <div className={styles.teamDetailPanels}>
            {activeTab === "members" ? (
              <section
                className={classNames(styles.panelSurface, styles.teamDetailPanel)}
                role="tabpanel"
                id="team-detail-panel-members"
                aria-labelledby="team-detail-tab-members"
              >
                <div className={styles.teamDetailPanelHead}>
                  <div>
                    <h2>{t("teamMembersTab")}</h2>
                    <p>{t("teamMembersCount", { count: members.length })}</p>
                  </div>
                </div>
                <div className={classNames(styles.itemList, styles.teamMemberList)}>
                  {members.length ? (
                    members.map((member) => {
                      const memberAgent = member.agent;
                      return memberAgent ? (
                        <button
                          key={member.id}
                          type="button"
                          className={classNames(styles.listRow, styles.teamMemberRow)}
                          onClick={() => onSelectAgent(memberAgent)}
                        >
                          <MemberRowContent member={member} t={t} />
                        </button>
                      ) : (
                        <div
                          key={member.id}
                          className={classNames(styles.listRow, styles.teamMemberRow, styles.teamMemberStatic)}
                        >
                          <MemberRowContent member={member} t={t} />
                        </div>
                      );
                    })
                  ) : (
                    <div className="workspace-empty">{t("teamNoMembers")}</div>
                  )}
                </div>
              </section>
            ) : null}

            {activeTab === "records" ? (
              <section
                className={classNames(styles.panelSurface, styles.teamDetailPanel)}
                role="tabpanel"
                id="team-detail-panel-records"
                aria-labelledby="team-detail-tab-records"
              >
                <div className={styles.teamDetailPanelHead}>
                  <div>
                    <h2>{t("teamRecordsTab")}</h2>
                    <p>{t("teamTaskRecordsCount", { count: parentTasks.length })}</p>
                  </div>
                </div>
                <div className={classNames(styles.itemList, styles.teamTaskList)}>
                  {parentTasks.length ? (
                    parentTasks.map((task) => {
                      const children = taskChildren(tasks, task.id);
                      const phase = resolveTaskSidebarPhase(task, children);
                      return (
                        <button
                          key={task.id}
                          type="button"
                          className={classNames(styles.listRow, styles.teamTaskRow)}
                          onClick={() => onSelectTask?.(task.id)}
                        >
                          <span
                            className={classNames(
                              "task-sidebar-status",
                              `task-sidebar-status-${task.status}`,
                              styles.teamTaskStatus,
                            )}
                            aria-hidden="true"
                          />
                          <span className={classNames(styles.rowMain, styles.teamTaskMain)}>
                            <span className={styles.teamTaskTitleLine}>
                              <strong>{task.title}</strong>
                              <TaskSubtaskIndicator subtasks={children} phase={phase} t={t} compact />
                            </span>
                            <span>{task.id}</span>
                          </span>
                        </button>
                      );
                    })
                  ) : (
                    <div className="workspace-empty">{t("teamNoTasks")}</div>
                  )}
                </div>
              </section>
            ) : null}
          </div>
        </div>
      </div>
    </section>
  );
}

type DetailFieldProps = {
  label: string;
  value: ReactNode;
};

function DetailField({ label, value }: DetailFieldProps) {
  return (
    <div className={classNames("entity-field", styles.teamDetailField)}>
      <span className={classNames(styles.fieldText, styles.teamDetailLabel)}>{label}</span>
      <span className={classNames(styles.fieldText, styles.teamDetailValue)}>{value}</span>
    </div>
  );
}

type TeamMemberDisplay = {
  agent: AgentLike | null;
  id: string;
  initials: string;
  leader: boolean;
  name: string;
  running: boolean;
};

function MemberRowContent({ member, t }: { member: TeamMemberDisplay; t: TranslateFn }) {
  return (
    <>
      <span className={classNames(styles.teamMemberAvatar, member.agent && styles.agent)}>
        {member.agent ? <AgentIcon /> : member.initials}
      </span>
      <span className={classNames(styles.rowMain, styles.teamMemberMain)}>
        <span className={styles.teamMemberTitleLine}>
          <strong>{member.name}</strong>
          {member.leader ? <span className="mini-badge warn">{t("teamLeadBadge")}</span> : null}
          {member.agent ? (
            <span className={`workspace-status-dot ${member.running ? "online" : ""}`} aria-hidden="true" />
          ) : null}
        </span>
        <span>{member.agent ? t("teamMemberAgent") : t("teamMemberHuman")}</span>
      </span>
    </>
  );
}

function teamMemberIDs(team: WorkspaceTeam): string[] {
  const ids = new Set(team.member_agent_ids ?? []);
  if (team.lead_agent_id) {
    // The lead agent ID (e.g., "u-manager") may differ from the room participant ID
    // (e.g., "manager"). Remove the participant ID form to avoid double-counting.
    const altID = team.lead_agent_id.startsWith("u-") ? team.lead_agent_id.slice(2) : "u-" + team.lead_agent_id;
    ids.delete(altID);
    ids.add(team.lead_agent_id);
  }
  return Array.from(ids);
}

function memberDisplay(
  memberID: string,
  agents: readonly AgentLike[],
  usersById: UsersLookup,
  leadAgentID: string,
): TeamMemberDisplay {
  const agent = agents.find((item) => item.id === memberID) ?? null;
  const user = lookupUser(usersById, memberID);
  const name = agent?.name || user?.name || memberID;
  return {
    id: memberID,
    agent,
    initials: initialsForName(name),
    leader: memberID === leadAgentID,
    name,
    running: agent ? isAgentRunning(agent) : false,
  };
}

function memberName(memberID: string, agents: readonly AgentLike[], usersById: UsersLookup): string {
  if (!memberID) {
    return "-";
  }
  return memberDisplay(memberID, agents, usersById, "").name;
}

function lookupUser(usersById: UsersLookup, memberID: string): IMUser | undefined {
  if (usersById instanceof Map) {
    return usersById.get(memberID);
  }
  return usersById[memberID];
}

function initialsForName(name: string): string {
  const parts = String(name || "")
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  if (parts.length >= 2) {
    return `${parts[0][0] || ""}${parts[1][0] || ""}`.toUpperCase();
  }
  return (parts[0] || "?").slice(0, 2).toUpperCase();
}
