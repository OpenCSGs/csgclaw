export type ScheduledTaskRecurrence = "once" | "daily" | "weekly" | "monthly";

export type WorkspaceScheduledTask = {
  id: string;
  title: string;
  agent_id: string;
  agent_name?: string;
  prompt: string;
  recurrence: ScheduledTaskRecurrence;
  enabled: boolean;
  next_run_at: string;
  last_run_at: string;
  expires_at: string;
  created_at: string;
  updated_at: string;
};

export type WorkspaceScheduledTaskRun = {
  id: string;
  scheduled_task_id: string;
  triggered_at: string;
  status: string;
  task_id: string;
  error: string;
};

export function normalizeScheduledTaskList(input: unknown): WorkspaceScheduledTask[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input
    .map(normalizeScheduledTask)
    .filter((item): item is WorkspaceScheduledTask => Boolean(item))
    .sort((left, right) => left.next_run_at.localeCompare(right.next_run_at) || left.created_at.localeCompare(right.created_at));
}

export function normalizeScheduledTaskRunList(input: unknown): WorkspaceScheduledTaskRun[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input
    .map(normalizeScheduledTaskRun)
    .filter((item): item is WorkspaceScheduledTaskRun => Boolean(item))
    .sort((left, right) => right.triggered_at.localeCompare(left.triggered_at));
}

export function normalizeScheduledTask(input: unknown): WorkspaceScheduledTask | null {
  if (!input || typeof input !== "object") {
    return null;
  }
  const item = input as Record<string, unknown>;
  const id = text(item.id);
  if (!id) {
    return null;
  }
  return {
    id,
    title: text(item.title) || id,
    agent_id: text(item.agent_id),
    agent_name: text(item.agent_name),
    prompt: text(item.prompt),
    recurrence: normalizeRecurrence(item.recurrence),
    enabled: booleanValue(item.enabled, true),
    next_run_at: timeText(item.next_run_at),
    last_run_at: timeText(item.last_run_at),
    expires_at: timeText(item.expires_at),
    created_at: text(item.created_at),
    updated_at: text(item.updated_at),
  };
}

export function normalizeScheduledTaskRun(input: unknown): WorkspaceScheduledTaskRun | null {
  if (!input || typeof input !== "object") {
    return null;
  }
  const item = input as Record<string, unknown>;
  const id = text(item.id);
  if (!id) {
    return null;
  }
  return {
    id,
    scheduled_task_id: text(item.scheduled_task_id),
    triggered_at: text(item.triggered_at),
    status: text(item.status),
    task_id: text(item.task_id),
    error: text(item.error),
  };
}

export function scheduledTaskRecurrenceLabel(value: string, t: (key: string) => string): string {
  switch (value) {
    case "daily":
      return t("scheduledTaskRecurrenceDaily");
    case "weekly":
      return t("scheduledTaskRecurrenceWeekly");
    case "monthly":
      return t("scheduledTaskRecurrenceMonthly");
    default:
      return t("scheduledTaskRecurrenceOnce");
  }
}

function normalizeRecurrence(value: unknown): ScheduledTaskRecurrence {
  switch (text(value)) {
    case "daily":
    case "weekly":
    case "monthly":
      return text(value) as ScheduledTaskRecurrence;
    default:
      return "once";
  }
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function timeText(value: unknown): string {
  const raw = text(value);
  return raw.startsWith("0001-01-01") ? "" : raw;
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  return typeof value === "boolean" ? value : fallback;
}
