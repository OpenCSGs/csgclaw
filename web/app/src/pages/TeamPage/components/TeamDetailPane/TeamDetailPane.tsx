import { useState } from "react";
import type { ReactNode } from "react";
import { ExternalLink, ListChecks, Plus, Users } from "lucide-react";
import { TaskSubtaskIndicator } from "@/components/business";
import { AgentIcon, UsersIcon } from "@/components/ui/Icons";
import {
  Button,
  DialogBody,
  DialogClose,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import { toggleSelection } from "@/shared/lib/collections";
import { isAgentRunning } from "@/models/agents";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMUser, TranslateFn, UsersById } from "@/models/conversations";
import {
  displayTeam,
  formatTaskUpdatedAt,
  resolveTaskSidebarPhase,
  rootTasks,
  taskChildren,
  teamStatusLabel,
} from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTeam } from "@/models/tasks";

type UsersLookup = UsersById | Record<string, IMUser | undefined>;
type VoidOrPromise = void | Promise<void>;

export type TeamDetailPaneProps = {
  agents?: AgentLike[];
  onAddAgentsToTeam?: (teamID: string, agentIDs: string[]) => VoidOrPromise;
  onOpenRoom?: (roomID: string) => VoidOrPromise;
  onSelectAgent?: (agent: AgentLike) => void;
  onSelectTask?: (taskID: string) => void;
  room?: IMConversation | null;
  tasks?: WorkspaceTask[];
  team?: WorkspaceTeam | null;
  teamActionBusy?: boolean;
  teamActionError?: string;
  teamsLoading?: boolean;
  t?: TranslateFn;
  usersById?: UsersLookup;
};

type ActiveTeamTab = "members" | "records";

export function TeamDetailPane({
  t = (key) => key,
  team = null,
  teamsLoading = false,
  room = null,
  agents = [],
  usersById = new Map<string, IMUser>(),
  tasks = [],
  onOpenRoom = () => {},
  onSelectAgent = () => {},
  onSelectTask = () => {},
  teamActionBusy = false,
  teamActionError = "",
  onAddAgentsToTeam,
}: TeamDetailPaneProps) {
  const [activeTab, setActiveTab] = useState<ActiveTeamTab>("members");
  const [memberDialogOpen, setMemberDialogOpen] = useState(false);
  const [selectedMemberIDs, setSelectedMemberIDs] = useState<string[]>([]);

  if (!team) {
    return (
      <section className="entity-pane team-detail-pane">
        <div className="empty-state shell-empty-state">
          <strong>{teamsLoading ? t("teamsLoading") : t("teamDetailMissing")}</strong>
          <span>{t("teamDetailMissingHint")}</span>
        </div>
      </section>
    );
  }

  const memberIDs = teamMemberIDs(team, room);
  const members = memberIDs.map((memberID) => memberDisplay(memberID, agents, usersById, team.lead_bot_id));
  const parentTasks = rootTasks(tasks);
  const locale = document.documentElement.lang || "en";
  const teamAgents = agents.filter((agent): agent is AgentLike & { id: string } => Boolean(agent?.id));
  const teamAgentIDs = teamAgents.map((agent) => String(agent.id));
  const roomMemberIDs = new Set(room?.members ?? []);
  const existingTeamAgentIDs = teamAgentIDs.filter(
    (agentID) => roomMemberIDs.has(agentID) || agentID === team.lead_bot_id,
  );
  const allAgentsSelected = teamAgentIDs.length > 0 && teamAgentIDs.every((id) => selectedMemberIDs.includes(id));
  const selectedAgentCount = selectedMemberIDs.filter((id) => teamAgentIDs.includes(id)).length;
  const hasNewMembersToAdd = selectedMemberIDs.some((id) => teamAgentIDs.includes(id) && !roomMemberIDs.has(id));

  function openAddMembersDialog() {
    setSelectedMemberIDs(existingTeamAgentIDs);
    setMemberDialogOpen(true);
  }

  async function addMembersToTeam() {
    if (!team?.id || !team.room_id || !hasNewMembersToAdd || teamActionBusy) {
      return;
    }
    await onAddAgentsToTeam?.(team.id, selectedMemberIDs);
    setMemberDialogOpen(false);
  }

  return (
    <section className="entity-pane team-detail-pane">
      <header className="entity-header team-detail-header">
        <div className="entity-avatar team-detail-avatar">
          <UsersIcon />
        </div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>{displayTeam(team)}</h1>
            <span className="status-pill online">{teamStatusLabel(team.status, t)}</span>
          </div>
          <p>{t("teamDetailSubtitle")}</p>
        </div>
        <div className="entity-toolbar">
          <Button
            variant="secondaryGray"
            size="md"
            disabled={!team.room_id}
            onClick={() => (team.room_id ? onOpenRoom?.(team.room_id) : undefined)}
          >
            <ExternalLink size={16} aria-hidden="true" />
            {t("teamOpenRecordRoom")}
          </Button>
          <Button
            variant="secondaryGray"
            size="md"
            disabled={!team.room_id || teamActionBusy}
            onClick={openAddMembersDialog}
          >
            <Plus size={16} aria-hidden="true" />
            {t("teamAddMembers")}
          </Button>
        </div>
      </header>

      <DialogRoot open={memberDialogOpen} onOpenChange={setMemberDialogOpen}>
        <DialogContent className="team-members-dialog">
          <DialogHeader>
            <div>
              <DialogTitle>{t("teamAddMembers")}</DialogTitle>
              <DialogDescription>{t("teamManageMembersSubtitle")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <div className="field team-members-dialog-field">
              <span>{t("teamMembersLabel")}</span>
              <div className="selection-list team-members-dialog-list">
                {teamAgents.length ? (
                  <>
                    <label className="selection-item selection-all-item">
                      <input
                        type="checkbox"
                        checked={allAgentsSelected}
                        onChange={() => {
                          setSelectedMemberIDs((current) =>
                            allAgentsSelected
                              ? current.filter((id) => !teamAgentIDs.includes(id))
                              : Array.from(new Set([...current, ...teamAgentIDs])),
                          );
                        }}
                      />
                      <span>{t("allMembers")}</span>
                      <small>
                        {selectedAgentCount}/{teamAgentIDs.length}
                      </small>
                    </label>
                    {teamAgents.map((agent) => {
                      const agentID = String(agent.id);
                      const inTeam = roomMemberIDs.has(agentID) || agentID === team.lead_bot_id;
                      return (
                        <label key={agentID} className="selection-item">
                          <input
                            type="checkbox"
                            checked={selectedMemberIDs.includes(agentID)}
                            onChange={() => setSelectedMemberIDs((current) => toggleSelection(current, agentID))}
                          />
                          <span>{agent.name || agentID}</span>
                          <small>{inTeam ? t("teamMemberInRoom") : agent.role || "-"}</small>
                        </label>
                      );
                    })}
                  </>
                ) : (
                  <div className="selection-empty">{t("teamNoMembersHint")}</div>
                )}
              </div>
            </div>
            {teamActionError ? <div className="form-error team-members-dialog-error">{teamActionError}</div> : null}
          </DialogBody>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="secondaryGray" size="md">
                {t("cancel")}
              </Button>
            </DialogClose>
            <Button
              variant="primary"
              size="md"
              loading={teamActionBusy}
              loadingLabel={t("teamSaving")}
              disabled={teamActionBusy || !hasNewMembersToAdd}
              onClick={addMembersToTeam}
            >
              {t("teamSaveMembers")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>

      <div className="team-detail-layout">
        <aside className="team-detail-summary">
          <div className="team-profile-block">
            <div className="team-profile-icon">
              <Users size={34} aria-hidden="true" />
            </div>
            <div>
              <h2>{displayTeam(team)}</h2>
              <p>{team.id}</p>
            </div>
          </div>
          <div className="team-detail-fields">
            <DetailField label={t("teamLeadLabel")} value={memberName(team.lead_bot_id, agents, usersById)} />
            <DetailField label={t("teamMembersLabel")} value={String(members.length)} />
            <DetailField label={t("teamChannelLabel")} value={team.channel || "csgclaw"} />
            <DetailField label={t("teamRecordRoomLabel")} value={room?.title || team.room_id || "-"} />
            <DetailField label={t("teamCreatedLabel")} value={formatTaskUpdatedAt(team.created_at, locale)} />
            <DetailField label={t("teamUpdatedLabel")} value={formatTaskUpdatedAt(team.updated_at, locale)} />
          </div>
        </aside>

        <div className="team-detail-main">
          <div className="team-detail-tabs" role="tablist" aria-label={t("teamDetailTabsLabel")}>
            <button
              type="button"
              className={activeTab === "members" ? "active" : ""}
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
              className={activeTab === "records" ? "active" : ""}
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
          <div className="team-detail-panels">
            {activeTab === "members" ? (
              <section
                className="team-detail-panel"
                role="tabpanel"
                id="team-detail-panel-members"
                aria-labelledby="team-detail-tab-members"
              >
                <div className="team-detail-panel-head">
                  <div>
                    <h2>{t("teamMembersTab")}</h2>
                    <p>{t("teamMembersCount", { count: members.length })}</p>
                  </div>
                </div>
                <div className="team-member-list">
                  {members.length ? (
                    members.map((member) => {
                      const memberAgent = member.agent;
                      return memberAgent ? (
                        <button
                          key={member.id}
                          type="button"
                          className="team-member-row"
                          onClick={() => onSelectAgent(memberAgent)}
                        >
                          <MemberRowContent member={member} t={t} />
                        </button>
                      ) : (
                        <div key={member.id} className="team-member-row team-member-static">
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
                className="team-detail-panel"
                role="tabpanel"
                id="team-detail-panel-records"
                aria-labelledby="team-detail-tab-records"
              >
                <div className="team-detail-panel-head">
                  <div>
                    <h2>{t("teamRecordsTab")}</h2>
                    <p>{t("teamTaskRecordsCount", { count: parentTasks.length })}</p>
                  </div>
                </div>
                <div className="team-task-list">
                  {parentTasks.length ? (
                    parentTasks.map((task) => {
                      const children = taskChildren(tasks, task.id);
                      const phase = resolveTaskSidebarPhase(task, children);
                      return (
                        <button
                          key={task.id}
                          type="button"
                          className="team-task-row"
                          onClick={() => onSelectTask?.(task.id)}
                        >
                          <span
                            className={`task-sidebar-status task-sidebar-status-${task.status}`}
                            aria-hidden="true"
                          />
                          <span className="team-task-main">
                            <span className="team-task-title-line">
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
    <div className="entity-field team-detail-field">
      <span className="team-detail-label">{label}</span>
      <span className="team-detail-value">{value}</span>
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
      <span className={`team-member-avatar ${member.agent ? "agent" : ""}`}>
        {member.agent ? <AgentIcon /> : member.initials}
      </span>
      <span className="team-member-main">
        <span className="team-member-title-line">
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

function teamMemberIDs(team: WorkspaceTeam, room: IMConversation | null | undefined): string[] {
  const ids = new Set(room?.members ?? []);
  if (team.lead_bot_id) {
    ids.add(team.lead_bot_id);
  }
  return Array.from(ids);
}

function memberDisplay(
  memberID: string,
  agents: readonly AgentLike[],
  usersById: UsersLookup,
  leadBotID: string,
): TeamMemberDisplay {
  const agent = agents.find((item) => item.id === memberID) ?? null;
  const user = lookupUser(usersById, memberID);
  const name = agent?.name || user?.name || memberID;
  return {
    id: memberID,
    agent,
    initials: initialsForName(name),
    leader: memberID === leadBotID,
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
