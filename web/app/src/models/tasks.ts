export type WorkspaceTask = {
  id: string;
  team_id: string;
  team_title: string;
  room_id: string;
  room_title: string;
  parent_id: string;
  title: string;
  body: string;
  status: string;
  assigned_to: string;
  claimed_by: string;
  priority: number;
  depends_on: string[];
  plan_summary: string;
  dispatched_at: string;
  result: string;
  error: string;
  created_at: string;
  updated_at: string;
};

export type WorkspaceTeamEvent = {
  seq: number;
  team_id: string;
  room_id: string;
  type: string;
  actor_id: string;
  task_id: string;
  target_id: string;
  summary: string;
  created_at: string;
};

export type WorkspaceTeam = {
  id: string;
  room_id: string;
  channel: string;
  title: string;
  lead_agent_id: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type WorkspaceTaskGroup = {
  task: WorkspaceTask;
  children: WorkspaceTask[];
};

export type WorkspaceTaskColumn = {
  status: string;
  tasks: WorkspaceTask[];
};

export const TASK_BOARD_STATUSES = ["pending", "assigned", "in_progress", "blocked", "completed", "failed"] as const;

export function normalizeTaskList(input: unknown): WorkspaceTask[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input
    .map(normalizeTask)
    .filter((item): item is WorkspaceTask => Boolean(item))
    .sort(compareTasks);
}

export function normalizeTeamList(input: unknown): WorkspaceTeam[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input
    .map(normalizeTeam)
    .filter((item): item is WorkspaceTeam => Boolean(item))
    .sort((left, right) => displayTeam(left).localeCompare(displayTeam(right)) || left.id.localeCompare(right.id));
}

export function normalizeTeamEventList(input: unknown): WorkspaceTeamEvent[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input
    .map(normalizeTeamEvent)
    .filter((item): item is WorkspaceTeamEvent => Boolean(item))
    .sort((left, right) => left.seq - right.seq || left.created_at.localeCompare(right.created_at));
}

export function normalizeTeam(input: unknown): WorkspaceTeam | null {
  if (!input || typeof input !== "object") {
    return null;
  }
  const item = input as Record<string, unknown>;
  const id = text(item.id);
  const roomID = text(item.room_id);
  if (!id || !roomID) {
    return null;
  }
  return {
    id,
    room_id: roomID,
    channel: text(item.channel) || "csgclaw",
    title: text(item.title),
    lead_agent_id: text(item.lead_agent_id) || text(item.lead_participant_id),
    status: text(item.status) || "active",
    created_at: text(item.created_at),
    updated_at: text(item.updated_at),
  };
}

export function normalizeTeamEvent(input: unknown): WorkspaceTeamEvent | null {
  if (!input || typeof input !== "object") {
    return null;
  }
  const item = input as Record<string, unknown>;
  const type = text(item.type);
  const teamID = text(item.team_id);
  if (!type || !teamID) {
    return null;
  }
  return {
    seq: numberValue(item.seq),
    team_id: teamID,
    room_id: text(item.room_id),
    type,
    actor_id: text(item.actor_id),
    task_id: text(item.task_id),
    target_id: text(item.target_id),
    summary: text(item.summary),
    created_at: text(item.created_at),
  };
}

export function normalizeTask(input: unknown): WorkspaceTask | null {
  if (!input || typeof input !== "object") {
    return null;
  }
  const item = input as Record<string, unknown>;
  const id = text(item.id);
  const teamID = text(item.team_id);
  const roomID = text(item.room_id);
  if (!id || !teamID || !roomID) {
    return null;
  }
  return {
    id,
    team_id: teamID,
    team_title: text(item.team_title),
    room_id: roomID,
    room_title: text(item.room_title),
    parent_id: text(item.parent_id),
    title: text(item.title) || id,
    body: text(item.body),
    status: text(item.status) || "pending",
    assigned_to: text(item.assigned_to),
    claimed_by: text(item.claimed_by),
    priority: numberValue(item.priority),
    depends_on: stringArray(item.depends_on),
    plan_summary: text(item.plan_summary),
    dispatched_at: text(item.dispatched_at),
    result: text(item.result),
    error: text(item.error),
    created_at: text(item.created_at),
    updated_at: text(item.updated_at),
  };
}

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

/** Locale-neutral absolute timestamp for task/team detail surfaces. */
export function formatTaskUpdatedAt(value: string, _locale?: string): string {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) {
    return "-";
  }
  const year = date.getFullYear();
  const month = pad2(date.getMonth() + 1);
  const day = pad2(date.getDate());
  const hours = pad2(date.getHours());
  const minutes = pad2(date.getMinutes());
  return `${year}.${month}.${day} ${hours}:${minutes}`;
}

export function displayTaskTeam(task: WorkspaceTask): string {
  return task.team_title || task.team_id;
}

export function displayTaskRoom(task: WorkspaceTask): string {
  return task.room_title || task.room_id;
}

export function taskExecutionRoomID(
  task: WorkspaceTask,
  children: readonly WorkspaceTask[],
  teams: readonly WorkspaceTeam[],
): string {
  const team = teams.find((item) => item.id === task.team_id);
  const teamRoomID = team?.room_id || "";
  if (task.room_id && task.room_id !== teamRoomID) {
    return task.room_id;
  }
  const child = children.find((item) => item.room_id && item.room_id !== teamRoomID);
  return child?.room_id || task.room_id;
}

export function taskUsesExecutionRoom(
  task: WorkspaceTask,
  teams: readonly WorkspaceTeam[],
  children: readonly WorkspaceTask[] = [],
): boolean {
  const team = teams.find((item) => item.id === task.team_id);
  if (!team) {
    return false;
  }
  const roomID = taskExecutionRoomID(task, children, teams);
  return Boolean(roomID) && roomID !== team.room_id;
}

export function displayTeam(team: WorkspaceTeam): string {
  return team.title || team.id;
}

type TranslateFn = (key: string) => string;

export function taskStatusLabel(status: unknown, t: TranslateFn): string {
  const normalized = String(status || "pending").trim();
  const key = `taskStatus.${normalized}`;
  const label = t(key);
  return label === key ? normalized : label;
}

export function taskStatusShortLabel(status: unknown, t: TranslateFn): string {
  const normalized = String(status || "pending").trim();
  const key = `taskStatusShort.${normalized}`;
  const label = t(key);
  return label === key ? taskStatusLabel(normalized, t) : label;
}

export function normalizeTaskStatus(status: unknown): string {
  const normalized = String(status || "pending").trim();
  return normalized || "pending";
}

export function teamStatusLabel(status: unknown, t: TranslateFn): string {
  const normalized = String(status || "active")
    .trim()
    .toLowerCase();
  if (normalized === "active") {
    return t("teamStatusActive");
  }
  if (normalized === "paused") {
    return t("teamStatusPaused");
  }
  if (normalized === "archived") {
    return t("teamStatusArchived");
  }
  return normalized || t("teamStatusActive");
}

export function rootTasks(tasks: readonly WorkspaceTask[]): WorkspaceTask[] {
  return tasks.filter((task) => !task.parent_id);
}

export function taskChildren(tasks: readonly WorkspaceTask[], parentID: string): WorkspaceTask[] {
  return tasks.filter((task) => task.parent_id === parentID).sort(compareTasks);
}

export type TaskSidebarPhase = "idle" | "planning" | "dispatching";

type ResolveTaskSidebarPhaseOptions = {
  planningTaskID?: string;
  startingTaskID?: string;
};

export function resolveTaskSidebarPhase(
  task: WorkspaceTask,
  children: readonly WorkspaceTask[],
  options: ResolveTaskSidebarPhaseOptions = {},
): TaskSidebarPhase {
  if (options.planningTaskID === task.id) {
    return "planning";
  }
  if (options.startingTaskID === task.id) {
    return "dispatching";
  }

  const status = normalizeTaskStatus(task.status);
  if (status === "failed" || status === "cancelled" || status === "completed") {
    return "idle";
  }

  if (children.length === 0) {
    if ((status === "pending" || status === "assigned") && !task.plan_summary.trim()) {
      return "planning";
    }
    return "idle";
  }

  return "idle";
}

export function shouldPollTransitionalTasks(tasks: readonly WorkspaceTask[]): boolean {
  return rootTasks(tasks).some((task) => {
    const children = taskChildren(tasks, task.id);
    return resolveTaskSidebarPhase(task, children) === "planning";
  });
}

export function groupTasksByParent(tasks: readonly WorkspaceTask[]): WorkspaceTaskGroup[] {
  const byID = new Map(tasks.map((task) => [task.id, task]));
  const childrenByParent = new Map<string, WorkspaceTask[]>();
  const roots: WorkspaceTask[] = [];

  for (const task of tasks) {
    if (!task.parent_id || !byID.has(task.parent_id)) {
      roots.push(task);
      continue;
    }
    const children = childrenByParent.get(task.parent_id) ?? [];
    children.push(task);
    childrenByParent.set(task.parent_id, children);
  }

  return roots.map((task) => ({
    task,
    children: (childrenByParent.get(task.id) ?? []).slice().sort(compareTasks),
  }));
}

export function boardColumnsForTask(tasks: readonly WorkspaceTask[], parentID: string): WorkspaceTaskColumn[] {
  const children = taskChildren(tasks, parentID);
  const defaultStatuses: readonly string[] = TASK_BOARD_STATUSES;
  const extraStatuses = Array.from(
    new Set(children.map((task) => task.status).filter((status) => !defaultStatuses.includes(status))),
  ).sort();
  return [...TASK_BOARD_STATUSES, ...extraStatuses].map((status) => ({
    status,
    tasks: children.filter((task) => task.status === status),
  }));
}

export function rootTaskForTask(
  tasks: readonly WorkspaceTask[],
  task: WorkspaceTask | null | undefined,
): WorkspaceTask | null {
  if (!task) {
    return null;
  }
  if (!task.parent_id) {
    return task;
  }
  return tasks.find((item) => item.id === task.parent_id) ?? task;
}

function compareTasks(left: WorkspaceTask, right: WorkspaceTask): number {
  const leftTime = Date.parse(left.updated_at || "");
  const rightTime = Date.parse(right.updated_at || "");
  if (Number.isFinite(leftTime) && Number.isFinite(rightTime) && leftTime !== rightTime) {
    return rightTime - leftTime;
  }
  if (left.priority !== right.priority) {
    return right.priority - left.priority;
  }
  if (left.team_id !== right.team_id) {
    return left.team_id.localeCompare(right.team_id);
  }
  return left.id.localeCompare(right.id);
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((item) => text(item)).filter(Boolean);
}

function numberValue(value: unknown): number {
  const parsed = typeof value === "number" ? value : Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}
