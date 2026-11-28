import type { TranslateFn } from "@/models/conversations";
import {
  computeNextRunAt,
  emptyScheduleDetail,
  MAX_SCHEDULED_TASK_INTERVAL_MINUTES,
  type ScheduledTaskRecurrence,
  type ScheduledTaskScheduleDetail,
  scheduledTaskScheduleSummary,
  scheduledTaskWallTime,
  scheduledTaskWallTimeISO,
  splitIntervalMinutes,
  SCHEDULED_TASK_INTERVAL_UNIT_MINUTES,
  type WorkspaceScheduledTask,
} from "@/models/scheduledTasks";
import type { ScheduledTaskFormDraft, ScheduledTaskFormFieldErrors } from "../taskDialogTypes";
import { clockText, localDateTimeInput, parseLocalDateTime } from "./calendarUtils";

export function recurrencePatch(
  recurrence: ScheduledTaskRecurrence,
  draft: ScheduledTaskFormDraft,
): Partial<ScheduledTaskFormDraft> {
  const now = new Date();
  switch (recurrence) {
    case "once":
      return {
        onceAt:
          draft.onceAt ||
          localDateTimeInput(new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours() + 1, 0, 0, 0)),
        startsDate: "",
        expiresDate: "",
      };
    case "weekly":
    case "biweekly":
      return { weekdays: draft.weekdays.length ? draft.weekdays : [now.getDay()] };
    case "monthly":
      return { monthDays: draft.monthDays.length ? draft.monthDays : [now.getDate()] };
    case "yearly":
      return {
        month: draft.month || now.getMonth() + 1,
        day: draft.day || now.getDate(),
      };
    default:
      return {};
  }
}

export function scheduleDetailFromDraft(draft: ScheduledTaskFormDraft): ScheduledTaskScheduleDetail {
  return {
    ...emptyScheduleDetail(),
    ...(draft.recurrence === "interval"
      ? {
          interval_minutes: draft.intervalValue * SCHEDULED_TASK_INTERVAL_UNIT_MINUTES[draft.intervalUnit],
        }
      : {
          clock: draft.time,
          time_zone: draft.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone,
        }),
    weekdays: draft.recurrence === "weekly" || draft.recurrence === "biweekly" ? draft.weekdays : [],
    month_days: draft.recurrence === "monthly" ? draft.monthDays : [],
    month: draft.recurrence === "yearly" ? draft.month : 0,
    day: draft.recurrence === "yearly" ? draft.day : 0,
  };
}

export function scheduleDraftBounds(draft: ScheduledTaskFormDraft, previous?: WorkspaceScheduledTask) {
  const original = previous ? draftScheduleFromTask(previous) : undefined;
  if (draft.recurrence === "once") return { startsAt: "", expiresAt: "" };
  return {
    startsAt:
      original && draft.startsDate === original.startsDate
        ? (previous?.starts_at ?? "")
        : scheduledTaskWallTimeISO(draft.startsDate, "00:00:00", draft.timeZone),
    expiresAt:
      original && previous?.recurrence !== "once" && draft.expiresDate === original.expiresDate
        ? (previous?.expires_at ?? "")
        : scheduledTaskWallTimeISO(draft.expiresDate, "23:59:59.999", draft.timeZone),
  };
}

export function nextRunISOFromDraft(
  draft: ScheduledTaskFormDraft,
  now = new Date(),
  startsAt = scheduleDraftBounds(draft).startsAt,
  biweeklyAnchor?: Date,
): string {
  return (
    computeNextRunAt({
      recurrence: draft.recurrence,
      schedule: scheduleDetailFromDraft(draft),
      onceAt: draft.onceAt,
      clock: draft.time,
      startsAt: startsAt ? new Date(startsAt) : null,
      biweeklyAnchor,
      now,
    })?.toISOString() ?? ""
  );
}

export function scheduleDraftChanged(draft: ScheduledTaskFormDraft, task: WorkspaceScheduledTask): boolean {
  const previous = draftScheduleFromTask(task);
  return (
    draft.recurrence !== previous.recurrence ||
    (draft.recurrence === "once"
      ? draft.onceAt !== previous.onceAt
      : draft.startsDate !== previous.startsDate ||
        JSON.stringify(scheduleDetailFromDraft(draft)) !== JSON.stringify(scheduleDetailFromDraft(previous)))
  );
}

export function validateScheduleDraft(draft: ScheduledTaskFormDraft, t: TranslateFn): ScheduledTaskFormFieldErrors {
  const errors: ScheduledTaskFormFieldErrors = {};
  switch (draft.recurrence) {
    case "once":
      if (!parseLocalDateTime(draft.onceAt)) {
        errors.onceAt = t("scheduledTaskOnceAtRequired");
      }
      break;
    case "interval":
      if (
        !Number.isInteger(draft.intervalValue) ||
        draft.intervalValue < 1 ||
        draft.intervalValue * SCHEDULED_TASK_INTERVAL_UNIT_MINUTES[draft.intervalUnit] >
          MAX_SCHEDULED_TASK_INTERVAL_MINUTES
      ) {
        errors.interval = t("scheduledTaskIntervalRequired", {
          max: MAX_SCHEDULED_TASK_INTERVAL_MINUTES / SCHEDULED_TASK_INTERVAL_UNIT_MINUTES[draft.intervalUnit],
        });
      }
      break;
    case "weekly":
    case "biweekly":
      if (!draft.weekdays.length) {
        errors.weekdays = t("scheduledTaskWeekdaysRequired");
      }
      break;
    case "monthly":
      if (!draft.monthDays.length) {
        errors.monthDays = t("scheduledTaskMonthDaysRequired");
      }
      break;
    default:
      break;
  }
  if (draft.recurrence !== "once" && draft.startsDate && draft.expiresDate && draft.expiresDate < draft.startsDate) {
    errors.effectiveRange = t("scheduledTaskEffectiveRangeError");
  }
  if (
    (draft.recurrence === "daily" ||
      draft.recurrence === "weekly" ||
      draft.recurrence === "biweekly" ||
      draft.recurrence === "monthly" ||
      draft.recurrence === "yearly") &&
    !draft.time
  ) {
    errors.time = t("scheduledTaskTimeRequired");
  }
  return errors;
}

export function draftScheduleFromTask(task: WorkspaceScheduledTask): ScheduledTaskFormDraft {
  const schedule = task.schedule ?? emptyScheduleDetail();
  const timeZone = schedule.time_zone;
  const nextRunAt = new Date(task.next_run_at);
  const nextRun = Number.isNaN(nextRunAt.getTime()) ? null : new Date(scheduledTaskWallTime(nextRunAt, timeZone));
  const interval = splitIntervalMinutes(schedule.interval_minutes);
  const today = new Date();
  return {
    agentID: task.agent_id,
    prompt: task.prompt,
    title: task.title,
    recurrence: task.recurrence,
    onceAt: task.next_run_at ? localDateTimeInputFromISO(task.next_run_at) : "",
    time: schedule.clock || (nextRun ? clockText(nextRun.getUTCHours(), nextRun.getUTCMinutes()) : "09:00"),
    timeZone,
    weekdays: schedule.weekdays.length ? schedule.weekdays : nextRun ? [nextRun.getUTCDay()] : [],
    monthDays: schedule.month_days.length ? schedule.month_days : nextRun ? [nextRun.getUTCDate()] : [],
    month: schedule.month || (nextRun ? nextRun.getUTCMonth() + 1 : today.getMonth() + 1),
    day: schedule.day || (nextRun ? nextRun.getUTCDate() : today.getDate()),
    intervalValue: interval.value,
    intervalUnit: interval.unit,
    startsDate:
      task.recurrence !== "once" && task.starts_at
        ? scheduledTaskWallTime(new Date(task.starts_at), timeZone).slice(0, 10)
        : "",
    expiresDate:
      task.recurrence !== "once" && task.expires_at
        ? scheduledTaskWallTime(new Date(task.expires_at), timeZone).slice(0, 10)
        : "",
  };
}

export function draftScheduleSummary(draft: ScheduledTaskFormDraft, t: TranslateFn): string {
  return scheduledTaskScheduleSummary({ recurrence: draft.recurrence, schedule: scheduleDetailFromDraft(draft) }, t);
}

function localDateTimeInputFromISO(value: string): string {
  if (!value) {
    return "";
  }
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? "" : localDateTimeInput(parsed);
}
