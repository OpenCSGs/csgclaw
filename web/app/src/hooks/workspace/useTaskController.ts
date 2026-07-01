import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createWorkspaceTask,
  fetchGlobalTasks,
  fetchTeamEvents,
  fetchTeams,
  planWorkspaceTask,
  startWorkspaceTask,
} from "@/api/tasks";
import { WorkspacePaneTypes } from "@/models/routing";
import { ApiError, errorMessage } from "@/api/client";
import { rootTaskForTask, type WorkspaceTask, rootTasks, shouldPollTransitionalTasks } from "@/models/tasks";
import { workspaceQueryKeys } from "./workspaceQueries";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import type { NavigatePaneOptions } from "./types";

const TASKS_QUERY_KEY = ["workspace", "tasks"] as const;
const TASK_BOARD_POLL_DELAY_MS = 3000;
const TASK_BOARD_POLL_STATUSES = new Set(["pending", "assigned", "in_progress"]);
const TASK_TAB_REVALIDATE_STALE_MS = 5000;
const teamEventsQueryKey = (teamID: string) => ["workspace", "team-events", teamID] as const;

type UseTaskControllerArgs = {
  activePane: { type?: string; id?: string };
  agents: AgentLike[];
  t: TranslateFn;
  onSelectConversation: (id: string) => void;
  onSelectTask: (taskID?: string, options?: NavigatePaneOptions) => void;
};

export function useTaskController({
  activePane,
  agents,
  t,
  onSelectConversation,
  onSelectTask,
}: UseTaskControllerArgs) {
  const queryClient = useQueryClient();
  const [showCreateTaskModal, setShowCreateTaskModal] = useState(false);
  const [createTaskBusy, setCreateTaskBusy] = useState(false);
  const [createTaskError, setCreateTaskError] = useState("");
  const [planTaskBusy, setPlanTaskBusy] = useState(false);
  const [startTaskBusy, setStartTaskBusy] = useState(false);
  const [planningTaskID, setPlanningTaskID] = useState("");
  const [startingTaskID, setStartingTaskID] = useState("");
  const [taskActionError, setTaskActionError] = useState("");
  const [parentDetailTaskID, setParentDetailTaskID] = useState("");
  const lastTaskTabRevalidateAttemptAt = useRef(0);
  const tasksQuery = useQuery({
    queryKey: TASKS_QUERY_KEY,
    queryFn: fetchGlobalTasks,
  });
  const { dataUpdatedAt: tasksDataUpdatedAt, isFetching: tasksFetching, refetch: refetchTasks } = tasksQuery;
  const teamsQuery = useQuery({
    queryKey: ["workspace", "teams"],
    queryFn: fetchTeams,
  });

  const tasks = useMemo(() => tasksQuery.data ?? [], [tasksQuery.data]);
  const teams = useMemo(() => teamsQuery.data ?? [], [teamsQuery.data]);
  const parentTasks = useMemo(() => rootTasks(tasks), [tasks]);
  const selectedTaskID = activePane.type === WorkspacePaneTypes.task ? String(activePane.id || "") : "";
  const selectedTask = useMemo(() => tasks.find((item) => item.id === selectedTaskID) ?? null, [selectedTaskID, tasks]);
  const activeRootTask = useMemo(() => rootTaskForTask(tasks, selectedTask), [selectedTask, tasks]);
  const visibleRootTask = activeRootTask ?? parentTasks[0] ?? null;
  const activeEventsTeamID = activeRootTask?.team_id || selectedTask?.team_id || "";
  const shouldPollActiveTaskBoard = useMemo(() => shouldPollTaskBoard(tasks, activeRootTask), [activeRootTask, tasks]);
  const shouldPollTasks = useMemo(
    () => shouldPollActiveTaskBoard || shouldPollTransitionalTasks(tasks),
    [shouldPollActiveTaskBoard, tasks],
  );
  const taskEventsQuery = useQuery({
    queryKey: teamEventsQueryKey(activeEventsTeamID),
    queryFn: () => fetchTeamEvents(activeEventsTeamID),
    enabled: Boolean(activeEventsTeamID),
    refetchInterval: shouldPollActiveTaskBoard ? TASK_BOARD_POLL_DELAY_MS : false,
  });
  const taskEvents = useMemo(() => taskEventsQuery.data ?? [], [taskEventsQuery.data]);

  useEffect(() => {
    if (activePane.type !== WorkspacePaneTypes.task || tasksFetching) {
      return;
    }
    const now = Date.now();
    const refreshedRecently = tasksDataUpdatedAt > 0 && now - tasksDataUpdatedAt < TASK_TAB_REVALIDATE_STALE_MS;
    const attemptedRecently = now - lastTaskTabRevalidateAttemptAt.current < TASK_TAB_REVALIDATE_STALE_MS;
    if (refreshedRecently || attemptedRecently) {
      return;
    }
    lastTaskTabRevalidateAttemptAt.current = now;
    void refetchTasks();
  }, [activePane.id, activePane.type, refetchTasks, tasksDataUpdatedAt, tasksFetching]);

  useEffect(() => {
    if (!selectedTask?.parent_id) {
      return;
    }
    onSelectTask(selectedTask.parent_id, { replace: true });
  }, [onSelectTask, selectedTask]);

  useEffect(() => {
    if (!shouldPollTasks) {
      return undefined;
    }
    let cancelled = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const nextTasks = await fetchGlobalTasks();
        if (cancelled) {
          return;
        }
        queryClient.setQueryData<WorkspaceTask[]>(TASKS_QUERY_KEY, (current) =>
          mergeWorkspaceTaskList(current ?? [], nextTasks),
        );
      } catch {
        return;
      }
      if (!cancelled) {
        timer = window.setTimeout(poll, TASK_BOARD_POLL_DELAY_MS);
      }
    };
    timer = window.setTimeout(poll, TASK_BOARD_POLL_DELAY_MS);
    return () => {
      cancelled = true;
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, [queryClient, shouldPollTasks]);

  function taskById(taskId: string) {
    return tasks.find((item) => item.id === taskId) ?? null;
  }

  function getPlanTarget(taskId?: string) {
    const target = taskById(String(taskId || "")) ?? activeRootTask;
    if (!target) {
      return null;
    }
    return rootTaskForTask(tasks, target);
  }

  function openParentTaskDetail(taskId?: string) {
    const targetID = String(taskId || visibleRootTask?.id || "").trim();
    if (!targetID) {
      return;
    }
    setParentDetailTaskID(targetID);
    onSelectTask(targetID);
  }

  async function createTask(draft: { team_id: string; title: string; body?: string }): Promise<void> {
    if (createTaskBusy) {
      return;
    }
    setCreateTaskBusy(true);
    setCreateTaskError("");
    try {
      const created = await createWorkspaceTask(draft);
      await tasksQuery.refetch();
      await queryClient.invalidateQueries({ queryKey: teamEventsQueryKey(created.team_id) });
      await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrap() });
      setShowCreateTaskModal(false);
      onSelectTask(created.id);
      await autoPlanAndStartTask(created.id, created.team_id);
    } catch (err) {
      setCreateTaskError(errorMessage(err, t("taskCreateFailed")));
    } finally {
      setCreateTaskBusy(false);
    }
  }

  async function autoPlanAndStartTask(taskID: string, teamID: string): Promise<void> {
    if (teamID && taskID) {
      try {
        setPlanningTaskID(taskID);
        setPlanTaskBusy(true);
        setStartTaskBusy(true);
        setTaskActionError("");
        const planned = await planWorkspaceTask({
          team_id: teamID,
          task_id: taskID,
          auto_start: true,
        });
        await tasksQuery.refetch();
        await queryClient.invalidateQueries({ queryKey: teamEventsQueryKey(teamID) });
        if (planned.started) {
          await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrap() });
        }
      } catch (err) {
        setTaskActionError(errorMessage(err, t("taskPlanFailed")));
      } finally {
        setPlanTaskBusy(false);
        setStartTaskBusy(false);
        setPlanningTaskID("");
        setStartingTaskID("");
      }
    }
  }

  async function planTask(taskId: string): Promise<void> {
    if (planTaskBusy || startTaskBusy) {
      return;
    }
    const target = getPlanTarget(taskId);
    if (!target) {
      setTaskActionError(t("taskPlanFailed"));
      return;
    }
    setPlanTaskBusy(true);
    setPlanningTaskID(target.id);
    setTaskActionError("");
    try {
      const planned = await planWorkspaceTask({
        team_id: target.team_id,
        task_id: target.id,
        auto_start: true,
      });
      await tasksQuery.refetch();
      await queryClient.invalidateQueries({ queryKey: teamEventsQueryKey(target.team_id) });
      if (planned.started) {
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrap() });
      }
    } catch (err) {
      setTaskActionError(errorMessage(err, t("taskPlanFailed")));
    } finally {
      setPlanTaskBusy(false);
      setPlanningTaskID("");
    }
  }

  async function finishStartTask(target: WorkspaceTask): Promise<void> {
    await tasksQuery.refetch();
    await queryClient.invalidateQueries({ queryKey: teamEventsQueryKey(target.team_id) });
    await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrap() });
  }

  async function startTask(taskId: string): Promise<void> {
    if (startTaskBusy || planTaskBusy) {
      return;
    }
    const target = getPlanTarget(taskId);
    if (!target) {
      setTaskActionError(t("taskStartFailed"));
      return;
    }
    setStartTaskBusy(true);
    setStartingTaskID(target.id);
    setTaskActionError("");
    try {
      await startWorkspaceTask({
        team_id: target.team_id,
        task_id: target.id,
      });
      await finishStartTask(target);
      return;
    } catch (err) {
      const needPlan = shouldAutoPlanForStartError(err);
      if (needPlan) {
        try {
          await planWorkspaceTask({
            team_id: target.team_id,
            task_id: target.id,
          });
          await queryClient.invalidateQueries({ queryKey: teamEventsQueryKey(target.team_id) });
          await startWorkspaceTask({
            team_id: target.team_id,
            task_id: target.id,
          });
          await finishStartTask(target);
          return;
        } catch (fallbackErr) {
          setTaskActionError(errorMessage(fallbackErr, t("taskStartFailed")));
          return;
        }
      }
      setTaskActionError(errorMessage(err, t("taskStartFailed")));
    } finally {
      setStartTaskBusy(false);
      setStartingTaskID("");
    }
  }

  return {
    tasks,
    rootTasks: parentTasks,
    rootTaskCount: parentTasks.length,
    planningTaskID,
    startingTaskID,
    openParentTaskDetail,
    openCreateTaskModal: () => {
      setCreateTaskError("");
      setShowCreateTaskModal(true);
    },
    taskViewProps: {
      t,
      agents,
      tasks,
      taskEvents,
      teams,
      selectedTask,
      selectedTaskID,
      loading: tasksQuery.isLoading,
      error:
        (tasksQuery.isError ? errorMessage(tasksQuery.error, t("tasksLoadFailed")) : "") ||
        (teamsQuery.isError ? errorMessage(teamsQuery.error, t("teamsLoadFailed")) : ""),
      createTaskBusy,
      createTaskError,
      planTaskBusy,
      planningTaskID,
      startTaskBusy,
      startingTaskID,
      taskActionError,
      showCreateTaskModal,
      parentDetailTaskID,
      onCloseCreateTaskModal: () => setShowCreateTaskModal(false),
      onCloseParentTaskDetail: () => setParentDetailTaskID(""),
      onCreateTask: createTask,
      onPlanTask: planTask,
      onStartTask: startTask,
      onRefresh: () => {
        void tasksQuery.refetch();
        if (activeEventsTeamID) {
          void taskEventsQuery.refetch();
        }
      },
      onSelectTask,
      onCloseTaskDetails: () => onSelectTask(),
      onOpenConversation: onSelectConversation,
      onViewParentDetail: openParentTaskDetail,
    },
  };
}

function isApiError(value: unknown): value is ApiError {
  return Boolean(
    value && typeof value === "object" && "status" in value && typeof (value as ApiError).status === "number",
  );
}

function shouldAutoPlanForStartError(error: unknown): boolean {
  const message = errorMessage(error, "").toLowerCase();
  if (!isApiError(error) || error.status !== 409) {
    return false;
  }
  return message.includes("no subtasks") || message.includes("has no subtasks");
}

function shouldPollTaskBoard(tasks: readonly WorkspaceTask[], root: WorkspaceTask | null): boolean {
  if (!root) {
    return false;
  }
  if (TASK_BOARD_POLL_STATUSES.has(root.status)) {
    return true;
  }
  return tasks.some((task) => task.parent_id === root.id && TASK_BOARD_POLL_STATUSES.has(task.status));
}

function mergeWorkspaceTaskList(current: readonly WorkspaceTask[], next: readonly WorkspaceTask[]): WorkspaceTask[] {
  const currentByID = new Map(current.map((task) => [task.id, task]));
  return next.map((task) => {
    const existing = currentByID.get(task.id);
    return existing && workspaceTasksEqual(existing, task) ? existing : task;
  });
}

function workspaceTasksEqual(left: WorkspaceTask, right: WorkspaceTask): boolean {
  return (
    left.id === right.id &&
    left.team_id === right.team_id &&
    left.team_title === right.team_title &&
    left.execution_channel === right.execution_channel &&
    left.room_id === right.room_id &&
    left.room_title === right.room_title &&
    left.parent_id === right.parent_id &&
    left.title === right.title &&
    left.body === right.body &&
    left.status === right.status &&
    left.created_by === right.created_by &&
    left.created_by_agent_name === right.created_by_agent_name &&
    left.assigned_to === right.assigned_to &&
    left.assigned_to_agent_name === right.assigned_to_agent_name &&
    left.claimed_by === right.claimed_by &&
    left.claimed_by_agent_name === right.claimed_by_agent_name &&
    left.priority === right.priority &&
    stringArraysEqual(left.depends_on, right.depends_on) &&
    left.plan_summary === right.plan_summary &&
    left.dispatched_at === right.dispatched_at &&
    left.result === right.result &&
    left.error === right.error &&
    left.created_at === right.created_at &&
    left.updated_at === right.updated_at
  );
}

function stringArraysEqual(left: readonly string[], right: readonly string[]): boolean {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((item, index) => item === right[index]);
}
