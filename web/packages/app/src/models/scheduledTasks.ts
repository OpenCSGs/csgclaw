import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";

export type ScheduledTaskRecurrence = "interval" | "once" | "daily" | "weekly" | "biweekly" | "monthly" | "yearly";

export type ScheduledTaskIntervalUnit = "minute" | "hour" | "day";

export const SCHEDULED_TASK_INTERVAL_UNIT_MINUTES: Record<ScheduledTaskIntervalUnit, number> = {
  minute: 1,
  hour: 60,
  day: 1440,
};

export const MAX_SCHEDULED_TASK_INTERVAL_MINUTES = 999 * SCHEDULED_TASK_INTERVAL_UNIT_MINUTES.day;

export const SCHEDULED_TASK_WEEKDAY_ORDER = [1, 2, 3, 4, 5, 6, 0] as const;

export type ScheduledTaskScheduleDetail = {
  interval_minutes: number;
  clock?: string;
  time_zone?: string;
  weekdays: number[];
  month_days: number[];
  month: number;
  day: number;
};

export function emptyScheduleDetail(): ScheduledTaskScheduleDetail {
  return { interval_minutes: 60, weekdays: [], month_days: [], month: 0, day: 0 };
}

export type WorkspaceScheduledTask = {
  id: string;
  title: string;
  agent_id: string;
  agent_name?: string;
  agent_state?: string;
  prompt: string;
  recurrence: ScheduledTaskRecurrence;
  schedule?: ScheduledTaskScheduleDetail;
  enabled: boolean;
  next_run_at: string;
  last_run_at: string;
  starts_at?: string;
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
    .sort(
      (left, right) =>
        left.next_run_at.localeCompare(right.next_run_at) || left.created_at.localeCompare(right.created_at),
    );
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
    agent_state: text(item.agent_state),
    prompt: text(item.prompt),
    recurrence: normalizeRecurrence(item.recurrence),
    ...(item.schedule_detail ? { schedule: normalizeScheduleDetail(item.schedule_detail) } : {}),
    enabled: booleanValue(item.enabled, true),
    next_run_at: timeText(item.next_run_at),
    last_run_at: timeText(item.last_run_at),
    ...(item.starts_at ? { starts_at: timeText(item.starts_at) } : {}),
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

export function scheduledTaskRecurrenceLabel(value: string, t: TranslateFn): string {
  switch (value) {
    case "interval":
      return t("scheduledTaskRecurrenceInterval");
    case "daily":
      return t("scheduledTaskRecurrenceDaily");
    case "weekly":
      return t("scheduledTaskRecurrenceWeekly");
    case "biweekly":
      return t("scheduledTaskRecurrenceBiweekly");
    case "monthly":
      return t("scheduledTaskRecurrenceMonthly");
    case "yearly":
      return t("scheduledTaskRecurrenceYearly");
    default:
      return t("scheduledTaskRecurrenceOnce");
  }
}

export function scheduledTaskScheduleSummary(
  input: { recurrence: ScheduledTaskRecurrence; schedule?: ScheduledTaskScheduleDetail | null },
  t: TranslateFn,
): string {
  const schedule = input.schedule ?? emptyScheduleDetail();
  const days = schedule.weekdays.length
    ? formatList(
        [...schedule.weekdays].sort(weekdaySort).map((weekday) => weekdayLabel(weekday, t)),
        t,
      )
    : "";
  const monthDays = schedule.month_days.length
    ? formatList(
        [...schedule.month_days].sort((a, b) => a - b).map((day) => t("scheduledTaskMonthDay", { day })),
        t,
      )
    : "";
  switch (input.recurrence) {
    case "interval": {
      const { value, unit } = splitIntervalMinutes(schedule.interval_minutes);
      return t("scheduledTaskIntervalSummary", { value, unit: intervalUnitLabel(unit, t) });
    }
    case "weekly":
    case "biweekly": {
      const base = scheduledTaskRecurrenceLabel(input.recurrence, t);
      return days ? `${base} ${days}` : base;
    }
    case "monthly": {
      const base = scheduledTaskRecurrenceLabel(input.recurrence, t);
      return monthDays ? `${base} ${monthDays}` : base;
    }
    case "yearly": {
      const base = scheduledTaskRecurrenceLabel(input.recurrence, t);
      return schedule.month && schedule.day
        ? `${base} ${t("scheduledTaskYearDay", { month: schedule.month, day: schedule.day })}`
        : base;
    }
    default:
      return scheduledTaskRecurrenceLabel(input.recurrence, t);
  }
}

export function intervalUnitLabel(unit: ScheduledTaskIntervalUnit, t: TranslateFn): string {
  switch (unit) {
    case "minute":
      return t("scheduledTaskIntervalMinute");
    case "day":
      return t("scheduledTaskIntervalDay");
    default:
      return t("scheduledTaskIntervalHour");
  }
}

export function splitIntervalMinutes(minutes: number): { value: number; unit: ScheduledTaskIntervalUnit } {
  const safe = Math.max(1, Math.round(minutes || 0));
  if (safe % SCHEDULED_TASK_INTERVAL_UNIT_MINUTES.day === 0) {
    return { value: safe / SCHEDULED_TASK_INTERVAL_UNIT_MINUTES.day, unit: "day" };
  }
  if (safe % SCHEDULED_TASK_INTERVAL_UNIT_MINUTES.hour === 0) {
    return { value: safe / SCHEDULED_TASK_INTERVAL_UNIT_MINUTES.hour, unit: "hour" };
  }
  return { value: safe, unit: "minute" };
}

export function weekdayLabel(weekday: number, t: TranslateFn): string {
  switch (weekday) {
    case 1:
      return t("scheduledTaskWeekdayMon");
    case 2:
      return t("scheduledTaskWeekdayTue");
    case 3:
      return t("scheduledTaskWeekdayWed");
    case 4:
      return t("scheduledTaskWeekdayThu");
    case 5:
      return t("scheduledTaskWeekdayFri");
    case 6:
      return t("scheduledTaskWeekdaySat");
    default:
      return t("scheduledTaskWeekdaySun");
  }
}

function weekdaySort(left: number, right: number): number {
  const order = SCHEDULED_TASK_WEEKDAY_ORDER as readonly number[];
  return order.indexOf(left) - order.indexOf(right);
}

function formatList(items: string[], t: TranslateFn): string {
  return items.join(t("scheduledTaskListSeparator"));
}

export function computeNextRunAt(input: {
  recurrence: ScheduledTaskRecurrence;
  schedule: ScheduledTaskScheduleDetail;
  onceAt?: string;
  clock?: string;
  now?: Date;
  startsAt?: Date | null;
  biweeklyAnchor?: Date;
}): Date | null {
  const now = input.now ?? new Date();
  if (input.recurrence === "once") {
    const parsed = parseLocalDateTime(input.onceAt ?? "");
    return parsed && parsed.getTime() > 0 ? parsed : null;
  }
  const startsAt = input.startsAt?.getTime() ?? -Infinity;
  if (input.recurrence === "interval") {
    const minutes = input.schedule.interval_minutes;
    if (!Number.isInteger(minutes) || minutes < 1 || minutes > MAX_SCHEDULED_TASK_INTERVAL_MINUTES) return null;
    return new Date(Math.max(startsAt, now.getTime() + minutes * 60_000));
  }
  const { hour, minute } = parseClock(input.clock ?? "");
  if (hour === null || minute === null) return null;
  const formatter = scheduleDateTimeFormatter(input.schedule.time_zone);
  const from = wallTime(new Date(Math.max(now.getTime(), startsAt)), formatter);
  from.setUTCHours(0, 0, 0, 0);
  const { weekdays, month_days, month, day } = input.schedule;
  const anchorWeek =
    input.recurrence === "biweekly" && input.biweeklyAnchor
      ? mondayOfWeek(wallTime(input.biweeklyAnchor, formatter))
      : undefined;
  // Allow a second week when the selected clock falls in a daylight-saving gap.
  const horizon =
    input.recurrence === "yearly" ? 8 * 366 : input.recurrence === "monthly" ? 62 : anchorWeek === undefined ? 14 : 28;
  for (let offset = 0; offset <= horizon; offset += 1) {
    const dayOnCalendar = new Date(from.getTime());
    dayOnCalendar.setUTCDate(from.getUTCDate() + offset);
    dayOnCalendar.setUTCHours(hour, minute);
    const matchesDate =
      input.recurrence === "daily" ||
      ((input.recurrence === "weekly" || input.recurrence === "biweekly") &&
        weekdays.includes(dayOnCalendar.getUTCDay())) ||
      (input.recurrence === "monthly" && month_days.includes(dayOnCalendar.getUTCDate())) ||
      (input.recurrence === "yearly" &&
        month === dayOnCalendar.getUTCMonth() + 1 &&
        day === dayOnCalendar.getUTCDate());
    if (
      !matchesDate ||
      (anchorWeek !== undefined && (mondayOfWeek(dayOnCalendar) - anchorWeek) % (14 * 86_400_000) !== 0)
    ) {
      continue;
    }
    const date = resolveWallTime(dayOnCalendar, formatter);
    if (date > now && date.getTime() >= startsAt && wallTime(date, formatter).getTime() === dayOnCalendar.getTime()) {
      return date;
    }
  }
  return null;
}

function mondayOfWeek(wall: Date): number {
  return Date.UTC(wall.getUTCFullYear(), wall.getUTCMonth(), wall.getUTCDate() - ((wall.getUTCDay() + 6) % 7));
}

function scheduleDateTimeFormatter(timeZone?: string): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat("en-US", {
    timeZone: timeZone === "Local" ? undefined : timeZone,
    calendar: "iso8601",
    numberingSystem: "latn",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
}

function wallTime(date: Date, formatter: Intl.DateTimeFormat): Date {
  const parts = Object.fromEntries(formatter.formatToParts(date).map(({ type, value }) => [type, value]));
  return new Date(
    `${parts.year.padStart(4, "0")}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}:${parts.second}Z`,
  );
}

function resolveWallTime(wall: Date, formatter: Intl.DateTimeFormat): Date {
  const instant = wall.getTime();
  const day = 86_400_000;
  const candidates = [-day, day].map((delta) => {
    const sample = new Date(instant + delta);
    const offset = wallTime(sample, formatter).getTime() - sample.getTime() + sample.getUTCMilliseconds();
    return instant - offset;
  });
  const matches = candidates.filter(
    (value) => wallTime(new Date(value), formatter).getTime() + wall.getUTCMilliseconds() === instant,
  );
  // Use the earlier instant when the clock repeats, and move forward across gaps.
  return new Date(matches.length ? Math.min(...matches) : Math.max(...candidates));
}

export function scheduledTaskWallTime(instant: Date, timeZone?: string): string {
  return Number.isNaN(instant.getTime()) ? "" : wallTime(instant, scheduleDateTimeFormatter(timeZone)).toISOString();
}

export function scheduledTaskWallTimeISO(date: string, time: string, timeZone?: string): string {
  const wall = new Date(`${date}T${time}Z`);
  return Number.isNaN(wall.getTime()) ? "" : resolveWallTime(wall, scheduleDateTimeFormatter(timeZone)).toISOString();
}

export function normalizeScheduleDetail(value: unknown): ScheduledTaskScheduleDetail {
  const empty = emptyScheduleDetail();
  if (!value || typeof value !== "object") {
    return empty;
  }
  const item = value as Record<string, unknown>;
  const weekdays = numberList(item.weekdays, 0, 6);
  const monthDays = numberList(item.month_days ?? item.monthDays, 1, 31);
  const month = numberValue(item.month, 1, 12);
  const day = numberValue(item.day, 1, 31);
  return {
    interval_minutes:
      numberValue(item.interval_minutes ?? item.intervalMinutes, 1, MAX_SCHEDULED_TASK_INTERVAL_MINUTES) ||
      empty.interval_minutes,
    ...(text(item.clock) ? { clock: text(item.clock) } : {}),
    ...(text(item.time_zone) ? { time_zone: text(item.time_zone) } : {}),
    weekdays,
    month_days: monthDays,
    month: month ?? 0,
    day: day ?? 0,
  };
}

function numberList(value: unknown, min: number, max: number): number[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const seen = new Set<number>();
  value.forEach((entry) => {
    const parsed = numberValue(entry, min, max);
    if (parsed !== undefined) {
      seen.add(parsed);
    }
  });
  return [...seen];
}

function numberValue(value: unknown, min: number, max: number): number | undefined {
  const parsed = typeof value === "number" ? value : Number.parseInt(text(value), 10);
  return Number.isInteger(parsed) && parsed >= min && parsed <= max ? parsed : undefined;
}

export function parseLocalDateTime(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(value || "");
  if (!match) {
    return null;
  }
  const [, year, month, day, hour, minute] = match;
  const parsed = new Date(Number(year), Number(month) - 1, Number(day), Number(hour), Number(minute), 0, 0);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

export function parseClock(value: string): { hour: number | null; minute: number | null } {
  const match = /^(\d{1,2}):(\d{2})$/.exec(value || "");
  if (!match) {
    return { hour: null, minute: null };
  }
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (hour > 23 || minute > 59) {
    return { hour: null, minute: null };
  }
  return { hour, minute };
}

function normalizeRecurrence(value: unknown): ScheduledTaskRecurrence {
  switch (text(value)) {
    case "interval":
    case "daily":
    case "weekly":
    case "biweekly":
    case "monthly":
    case "yearly":
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

export function scheduledTaskExecutors(agents: readonly AgentLike[]): AgentLike[] {
  return agents
    .filter((agent) => String(agent.id || "").trim())
    .slice()
    .sort(
      (a, b) =>
        Number(b.role === "manager") - Number(a.role === "manager") || executorName(a).localeCompare(executorName(b)),
    );
}
export function executorName(agent: AgentLike): string {
  return String(agent.name || agent.user_name || agent.id || "").trim();
}
export function scheduledTaskMention(text: string, caret: number) {
  const match = /@([^\s@]*)$/.exec(text.slice(0, caret));
  return match ? { query: match[1], start: caret - match[0].length, end: caret } : null;
}
