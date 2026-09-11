import { localIdentitiesMatch, type IMMessage } from "./conversations";
import type { WorkspaceTask, WorkspaceTaskGroup } from "./tasks";
import { pathForPane, WorkspacePaneTypes } from "./routing";

export function roomTaskGroups(tasks: readonly WorkspaceTask[], roomID: string): WorkspaceTaskGroup[] {
  const scoped = tasks.filter((task) => task.assignment_type === "room" && task.assignment_id === roomID);
  return scoped
    .filter((task) => !task.parent_id)
    .sort((left, right) => {
      const rank = (t: WorkspaceTask) => (roomTaskExecutionEnded(t) ? 2 : t.status === "queued" ? 1 : 0);
      return rank(left) - rank(right) || left.created_at.localeCompare(right.created_at);
    })
    .map((task) => ({
      task,
      children: scoped.filter((child) => child.parent_id === task.id).sort(comparePlanOrder),
    }));
}

function comparePlanOrder(left: WorkspaceTask, right: WorkspaceTask): number {
  return (
    left.created_at.localeCompare(right.created_at) ||
    left.id.length - right.id.length ||
    left.id.localeCompare(right.id)
  );
}

export function roomTaskExecutionEnded(task: WorkspaceTask): boolean {
  return ["completed", "failed", "cancelled", "canceled"].includes(task.status);
}

export function roomTaskParent(tasks: readonly WorkspaceTask[], id: string): WorkspaceTask | undefined {
  const byID = new Map(tasks.map((task) => [task.id, task]));
  let task = byID.get(id);
  const seen = new Set<string>();
  while (task?.parent_id) {
    if (seen.has(task.id)) return undefined;
    seen.add(task.id);
    const parent = byID.get(task.parent_id);
    if (
      !parent ||
      parent.assignment_type !== task.assignment_type ||
      parent.assignment_id !== task.assignment_id ||
      parent.room_id !== task.room_id
    )
      return undefined;
    task = parent;
  }
  return task;
}
export function roomTaskState(task: WorkspaceTask, tasks: readonly WorkspaceTask[]): string {
  if (!task.parent_id && task.status === "completed" && task.goal_outcome === "issues") return "issues";
  if (task.recovery_required) return "recovery";
  if (task.waiting_on_task_id && task.status === "blocked") return "waiting_resource";
  if (
    task.parent_id &&
    task.status === "pending" &&
    task.depends_on.some((id) => tasks.find((t) => t.id === id)?.status !== "completed")
  )
    return "waiting_dependency";
  if (task.status === "pending_review" && !task.parent_id) return "summarizing";
  return task.status;
}
export function roomTaskPath(roomID: string, taskID: string): string {
  return `${pathForPane({ type: WorkspacePaneTypes.conversation, id: roomID })}?task=${encodeURIComponent(taskID)}`;
}

export type RoomTaskEvent = {
  seq: number;
  task_id: string;
  actor_id: string;
  target_id?: string;
  type: string;
  summary: string;
  attempt?: number;
  created_at: string;
};

// Only the first persisted plan message owns the chat entry. Result, relay and
// report messages keep task metadata for coordination, not additional badges.
export function roomTaskMessageAnchors(messages: readonly IMMessage[], managerID: string): Map<string, string> {
  const anchors = new Map<string, string>();
  const seen = new Set<string>();
  for (const message of [...messages].sort(
    (a, b) => (a.created_at ?? "").localeCompare(b.created_at ?? "") || (a.id ?? "").localeCompare(b.id ?? ""),
  )) {
    if (!message.id) continue;
    const metadata = message.metadata;
    const delivery = metadata?.csgclaw;
    if (
      !localIdentitiesMatch(message.sender_id, managerID) ||
      !delivery ||
      typeof delivery !== "object" ||
      !("delivery_kind" in delivery) ||
      delivery.delivery_kind !== "task_planned"
    )
      continue;
    const id = metadata?.task_id;
    if (typeof id !== "string" || !id || seen.has(id)) continue;
    seen.add(id);
    anchors.set(message.id, id);
  }
  return anchors;
}
