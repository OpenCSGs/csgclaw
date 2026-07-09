import type { ScheduledTaskRecurrence } from "@/models/scheduledTasks";

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
  date: string;
  expiresDate: string;
  prompt: string;
  recurrence: ScheduledTaskRecurrence;
  time: string;
  title: string;
};

export type ScheduledTaskFormFieldErrors = {
  agentID?: string;
  date?: string;
  prompt?: string;
  time?: string;
  title?: string;
};
