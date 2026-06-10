import { get, post } from "@/api/client";
import {
  normalizeTask,
  normalizeTaskList,
  normalizeTeam,
  normalizeTeamEventList,
  normalizeTeamList,
} from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTeam, WorkspaceTeamEvent } from "@/models/tasks";

export type CreateTeamPayload = {
  channel?: string;
  lead_agent_id?: string;
  lead_participant_id?: string;
  member_agent_ids?: string[];
  member_participant_ids?: string[];
  room_id?: string;
  title?: string;
};

export type CreateWorkspaceTaskPayload = {
  assign_to?: string;
  body?: string;
  created_by?: string;
  priority?: number;
  team_id: string;
  title: string;
};

export async function fetchGlobalTasks(): Promise<WorkspaceTask[]> {
  return normalizeTaskList(await get<unknown>("/api/v1/tasks"));
}

export async function fetchTeams(): Promise<WorkspaceTeam[]> {
  return normalizeTeamList(await get<unknown>("/api/v1/teams"));
}

export async function fetchTeamEvents(teamID: string): Promise<WorkspaceTeamEvent[]> {
  const normalizedTeamID = String(teamID || "").trim();
  if (!normalizedTeamID) {
    return [];
  }
  return normalizeTeamEventList(await get<unknown>(`/api/v1/teams/${encodeURIComponent(normalizedTeamID)}/events`));
}

export type PlanWorkspaceTaskPayload = {
  team_id: string;
  task_id: string;
  actor_id?: string;
  auto_start?: boolean;
};

export type PlanWorkspaceTaskResponse = {
  task: WorkspaceTask;
  created_tasks: WorkspaceTask[];
  already_planned: boolean;
  started: boolean;
  scheduled_tasks: number;
};

export type StartWorkspaceTaskPayload = {
  team_id: string;
  task_id: string;
  actor_id?: string;
};

export type StartWorkspaceTaskResponse = {
  task: WorkspaceTask;
  scheduled_tasks: number;
};

export async function createTeamRequest(payload: CreateTeamPayload): Promise<WorkspaceTeam> {
  const request: Record<string, unknown> = {
    channel: payload.channel || "csgclaw",
    room_id: payload.room_id,
    title: payload.title,
  };
  if (payload.lead_agent_id !== undefined) {
    request.lead_agent_id = payload.lead_agent_id;
  }
  if (payload.lead_participant_id !== undefined) {
    request.lead_participant_id = payload.lead_participant_id;
  }
  if (payload.member_agent_ids !== undefined) {
    request.member_agent_ids = payload.member_agent_ids;
  }
  if (payload.member_participant_ids !== undefined) {
    request.member_participant_ids = payload.member_participant_ids;
  }

  const team = normalizeTeam(
    await post<unknown>("/api/v1/teams", request),
  );
  if (!team) {
    throw new Error("Invalid team response");
  }
  return team;
}

export async function createWorkspaceTask(payload: CreateWorkspaceTaskPayload): Promise<WorkspaceTask> {
  const response = await post<unknown>(`/api/v1/teams/${encodeURIComponent(payload.team_id)}/tasks/batch`, {
    created_by: payload.created_by || undefined,
    tasks: [
      {
        assign_to: payload.assign_to || undefined,
        body: payload.body || undefined,
        priority: payload.priority || undefined,
        title: payload.title,
      },
    ],
  });
  const task = normalizeCreatedTask(response);
  if (!task) {
    throw new Error("Invalid task response");
  }
  return task;
}

export async function planWorkspaceTask(payload: PlanWorkspaceTaskPayload): Promise<PlanWorkspaceTaskResponse> {
  const response = await post<unknown>(
    `/api/v1/teams/${encodeURIComponent(payload.team_id)}/tasks/${encodeURIComponent(payload.task_id)}/plan`,
    {
      actor_id: payload.actor_id || undefined,
      auto_start: Boolean(payload.auto_start),
    },
  );
  const parsed = response as Record<string, unknown>;
  const task = normalizeTask(parsed.task);
  if (!task) {
    throw new Error("Invalid plan task response");
  }
  return {
    task,
    created_tasks: normalizeTaskList(parsed.created_tasks),
    already_planned: typeof parsed.already_planned === "boolean" ? parsed.already_planned : false,
    started: typeof parsed.started === "boolean" ? parsed.started : false,
    scheduled_tasks: numberValue(parsed.scheduled_tasks),
  };
}

export async function startWorkspaceTask(payload: StartWorkspaceTaskPayload): Promise<StartWorkspaceTaskResponse> {
  const response = await post<unknown>(
    `/api/v1/teams/${encodeURIComponent(payload.team_id)}/tasks/${encodeURIComponent(payload.task_id)}/start`,
    {
      actor_id: payload.actor_id || undefined,
    },
  );
  const parsed = response as Record<string, unknown>;
  const task = normalizeTask(parsed.task);
  if (!task) {
    throw new Error("Invalid start task response");
  }
  return {
    task,
    scheduled_tasks: numberValue(parsed.scheduled_tasks),
  };
}

function normalizeCreatedTask(input: unknown): WorkspaceTask | null {
  if (!input || typeof input !== "object") {
    return null;
  }
  const item = input as Record<string, unknown>;
  if (Array.isArray(item.tasks)) {
    return normalizeTask(item.tasks[0]);
  }
  return normalizeTask(input);
}

function numberValue(value: unknown): number {
  const parsed = typeof value === "number" ? value : Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}
