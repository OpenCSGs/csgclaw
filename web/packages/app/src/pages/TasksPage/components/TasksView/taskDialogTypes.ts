import type { ScheduledTaskIntervalUnit, ScheduledTaskRecurrence } from "@/models/scheduledTasks";

export const TASK_TITLE_MAX_LENGTH = 80;

export type TaskCreateDraft = {
  assignee: string;
  title: string;
  description: string;
};

export type TaskCreateFieldErrors = {
  assignment?: string;
  title?: string;
};

export type ScheduledTaskFormDraft = {
  agentID: string;
  prompt: string;
  title: string;
  recurrence: ScheduledTaskRecurrence;
  onceAt: string;
  time: string;
  timeZone?: string;
  weekdays: number[];
  monthDays: number[];
  month: number;
  day: number;
  intervalValue: number;
  intervalUnit: ScheduledTaskIntervalUnit;
  startsDate: string;
  expiresDate: string;
};

export type ScheduledTaskFormFieldErrors = {
  agentID?: string;
  interval?: string;
  monthDays?: string;
  effectiveRange?: string;
  onceAt?: string;
  prompt?: string;
  time?: string;
  title?: string;
  weekdays?: string;
};
