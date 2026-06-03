import { get } from "@/api/client";
import { normalizeTaskList } from "@/models/tasks";
import type { WorkspaceTask } from "@/models/tasks";

export async function fetchGlobalTasks(): Promise<WorkspaceTask[]> {
  return normalizeTaskList(await get<unknown>("/api/v1/tasks"));
}
