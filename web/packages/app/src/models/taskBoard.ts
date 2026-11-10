import type { AgentLike } from "@/models/agents";
import type { TranslateFn, UsersById } from "@/models/conversations";
import { displayTaskAssignmentTarget, type WorkspaceTask } from "@/models/tasks";

export function agentNameLookup(agents: readonly AgentLike[], usersById?: UsersById): ReadonlyMap<string, string> {
  const lookup = new Map<string, string>();
  agents.forEach((agent) => {
    const name = String(agent.name || agent.user_name || agent.id || "").trim();
    const ids = [agent.id, agent.user_id].map((value) => String(value || "").trim()).filter(Boolean);
    ids.forEach((id) => {
      if (name && !lookup.has(id)) {
        lookup.set(id, name);
      }
    });
  });
  usersById?.forEach((user, id) => {
    if (!lookup.has(id) && user.name) lookup.set(id, user.name);
  });
  return lookup;
}

export function displayAgentByID(agentID: string, lookup: ReadonlyMap<string, string>, fallbackName?: string): string {
  const id = String(agentID || "").trim();
  const name = String(fallbackName || "").trim();
  return id ? name || lookup.get(id) || id : name;
}

export type TaskActiveWorker = {
  label: string;
  name: string;
  tone: "working";
};

export function taskActiveWorker(
  task: WorkspaceTask,
  childTasks: readonly WorkspaceTask[],
  t: TranslateFn,
  agentNames: ReadonlyMap<string, string>,
): TaskActiveWorker | null {
  const activeChild = childTasks
    .filter((child) => !isTerminalTaskStatus(child.status))
    .filter((child) => taskWorkerName(child, agentNames))
    .sort(
      (left, right) =>
        activeWorkerStatusRank(left.status) - activeWorkerStatusRank(right.status) ||
        right.updated_at.localeCompare(left.updated_at),
    )[0];
  if (activeChild) {
    const workerName = taskWorkerName(activeChild, agentNames);
    if (!workerName) {
      return null;
    }
    return {
      name: workerName,
      label: t("taskActiveWorkerWorking"),
      tone: "working",
    };
  }
  const parentWorkerName = taskWorkerName(task, agentNames);
  if (parentWorkerName && !isTerminalTaskStatus(task.status)) {
    return { name: parentWorkerName, label: t("taskActiveWorkerWorking"), tone: "working" };
  }
  return null;
}

function activeWorkerStatusRank(status: string): number {
  if (status === "in_progress" || status === "running") {
    return 0;
  }
  if (status === "assigned") {
    return 1;
  }
  if (status === "pending") {
    return 2;
  }
  return 3;
}

function taskWorkerName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  const name = displayTaskWorkerName(task, agentNames);
  return isDisplayableWorkerName(name) ? name : "";
}

function displayTaskAssignedAgentName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  const assignedID = task.assigned_to || (task.assignment_type === "agent" ? task.assignment_id : "");
  return task.assigned_to_agent_name || displayAgentByID(assignedID, agentNames);
}

export function displayTaskClaimedAgentName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  return task.claimed_by_agent_name || displayAgentByID(task.claimed_by, agentNames);
}

export function displayTaskWorkerName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  return displayTaskClaimedAgentName(task, agentNames) || displayTaskAssignedAgentName(task, agentNames);
}

export function displayTaskAssignmentTargetName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  if (task.parent_id || task.assignment_type === "agent") {
    return displayTaskAssignedAgentName(task, agentNames);
  }
  return displayTaskAssignmentTarget(task);
}

function isDisplayableWorkerName(name: string): boolean {
  const normalized = name.trim().toLowerCase();
  return Boolean(normalized) && !["manager", "planner", "u-manager"].includes(normalized);
}

function isTerminalTaskStatus(status: string): boolean {
  return ["completed", "done", "failed", "cancelled", "canceled"].includes(status);
}
