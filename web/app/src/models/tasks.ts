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
  priority: number;
  depends_on: string[];
  updated_at: string;
};

export type WorkspaceTaskGroup = {
  task: WorkspaceTask;
  children: WorkspaceTask[];
};

export function normalizeTaskList(input: unknown): WorkspaceTask[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input
    .map(normalizeTask)
    .filter((item): item is WorkspaceTask => Boolean(item))
    .sort(compareTasks);
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
    priority: numberValue(item.priority),
    depends_on: stringArray(item.depends_on),
    updated_at: text(item.updated_at),
  };
}

export function formatTaskUpdatedAt(value: string, locale: string): string {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function displayTaskTeam(task: WorkspaceTask): string {
  return task.team_title || task.team_id;
}

export function displayTaskRoom(task: WorkspaceTask): string {
  return task.room_title || task.room_id;
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
