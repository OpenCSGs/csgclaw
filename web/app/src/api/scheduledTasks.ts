import { del, get, patch, post } from "@/api/client";
import {
  normalizeScheduledTask,
  normalizeScheduledTaskList,
  normalizeScheduledTaskRun,
  normalizeScheduledTaskRunList,
} from "@/models/scheduledTasks";
import type {
  ScheduledTaskRecurrence,
  WorkspaceScheduledTask,
  WorkspaceScheduledTaskRun,
} from "@/models/scheduledTasks";

export type CreateScheduledTaskPayload = {
  title: string;
  agent_id: string;
  prompt: string;
  recurrence: ScheduledTaskRecurrence;
  first_run_at: string;
  expires_at?: string;
  enabled?: boolean;
};

export type UpdateScheduledTaskPayload = Partial<{
  title: string;
  agent_id: string;
  prompt: string;
  recurrence: ScheduledTaskRecurrence;
  next_run_at: string;
  expires_at: string | null;
  enabled: boolean;
}>;

export async function fetchScheduledTasks(): Promise<WorkspaceScheduledTask[]> {
  return normalizeScheduledTaskList(await get<unknown>("/api/v1/scheduled-tasks"));
}

export async function fetchScheduledTaskRuns(taskID: string): Promise<WorkspaceScheduledTaskRun[]> {
  const normalizedTaskID = String(taskID || "").trim();
  if (!normalizedTaskID) {
    return [];
  }
  return normalizeScheduledTaskRunList(
    await get<unknown>(`/api/v1/scheduled-tasks/${encodeURIComponent(normalizedTaskID)}/runs`),
  );
}

export async function createScheduledTask(payload: CreateScheduledTaskPayload): Promise<WorkspaceScheduledTask> {
  const task = normalizeScheduledTask(await post<unknown>("/api/v1/scheduled-tasks", payload));
  if (!task) {
    throw new Error("Invalid scheduled task response");
  }
  return task;
}

export async function updateScheduledTask(
  taskID: string,
  payload: UpdateScheduledTaskPayload,
): Promise<WorkspaceScheduledTask> {
  const normalizedTaskID = String(taskID || "").trim();
  if (!normalizedTaskID) {
    throw new Error("Scheduled task ID is required");
  }
  const task = normalizeScheduledTask(
    await patch<unknown>(`/api/v1/scheduled-tasks/${encodeURIComponent(normalizedTaskID)}`, payload),
  );
  if (!task) {
    throw new Error("Invalid scheduled task response");
  }
  return task;
}

export async function deleteScheduledTask(taskID: string): Promise<void> {
  const normalizedTaskID = String(taskID || "").trim();
  if (!normalizedTaskID) {
    throw new Error("Scheduled task ID is required");
  }
  await del<void>(`/api/v1/scheduled-tasks/${encodeURIComponent(normalizedTaskID)}`);
}

export async function runScheduledTaskNow(taskID: string): Promise<WorkspaceScheduledTaskRun> {
  const normalizedTaskID = String(taskID || "").trim();
  if (!normalizedTaskID) {
    throw new Error("Scheduled task ID is required");
  }
  const run = normalizeScheduledTaskRun(
    await post<unknown>(`/api/v1/scheduled-tasks/${encodeURIComponent(normalizedTaskID)}/run-now`, {}),
  );
  if (!run) {
    throw new Error("Invalid scheduled task run response");
  }
  return run;
}
