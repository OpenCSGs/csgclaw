import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createWorkspaceTask,
  fetchGlobalTasks,
  fetchTeamEvents,
  fetchTeams,
  planWorkspaceTask,
  startWorkspaceTask,
} from "@/api/tasks";
import {
  createScheduledTask,
  deleteScheduledTask,
  fetchScheduledTaskRuns,
  fetchScheduledTasks,
  runScheduledTaskNow,
  updateScheduledTask,
  type CreateScheduledTaskPayload,
  type UpdateScheduledTaskPayload,
} from "@/api/scheduledTasks";
import { WorkspacePaneTypes } from "@/models/routing";
import { errorMessage, type ApiError } from "@/api/client";
import { rootTaskForTask, type WorkspaceTask, rootTasks, shouldPollTransitionalTasks } from "@/models/tasks";
import type { WorkspaceScheduledTask, WorkspaceScheduledTaskRun } from "@/models/scheduledTasks";
import { workspaceQueryKeys } from "./workspaceQueries";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { NavigatePaneOptions } from "./types";

const TASKS_QUERY_KEY = ["workspace", "tasks"] as const;
const SCHEDULED_TASKS_QUERY_KEY = ["workspace", "scheduled-tasks"] as const;
const TASK_BOARD_POLL_DELAY_MS = 3000;
const TASK_BOARD_POLL_STATUSES = new Set(["pending", "assigned", "in_progress", "running"]);
const TASK_TAB_REVALIDATE_STALE_MS = 5000;
const teamEventsQueryKey = (teamID: string) => ["workspace", "team-events", teamID] as const;
const scheduledTaskRunsQueryKey = (taskID: string) => ["workspace", "scheduled-task-runs", taskID] as const;
const scheduledTaskRunsAllQueryKey = (taskIDsKey: string) =>
  ["workspace", "scheduled-task-runs-all", taskIDsKey] as const;
export type TaskBoardView = "tasks" | "scheduled";

type UseTaskControllerArgs = {
  activePane: { type?: string; id?: string };
  agents: AgentLike[];
  rooms?: readonly Pick<IMConversation, "id">[];
  t: TranslateFn;
  onSelectConversation: (id: string) => void;
  onSelectTask: (taskID?: string, options?: NavigatePaneOptions) => void;
};

export function useTaskController({
  activePane,
  agents,
  rooms,
  t,
  onSelectConversation,
  onSelectTask,
}: UseTaskControllerArgs) {
  const queryClient = useQueryClient();
  const [showCreateTaskModal, setShowCreateTaskModal] = useState(false);
  const [showCreateScheduledTaskModal, setShowCreateScheduledTaskModal] = useState(false);
  const [taskBoardView, setTaskBoardView] = useState<TaskBoardView>("tasks");
  const [editingScheduledTaskID, setEditingScheduledTaskID] = useState("");
  const [createTaskBusy, setCreateTaskBusy] = useState(false);
  const [createTaskError, setCreateTaskError] = useState("");
  const [createScheduledTaskBusy, setCreateScheduledTaskBusy] = useState(false);
  const [createScheduledTaskError, setCreateScheduledTaskError] = useState("");
  const [editScheduledTaskBusy, setEditScheduledTaskBusy] = useState(false);
  const [editScheduledTaskError, setEditScheduledTaskError] = useState("");
  const [scheduledTaskActionID, setScheduledTaskActionID] = useState("");
  const [scheduledTaskActionError, setScheduledTaskActionError] = useState("");
  const [selectedScheduledTaskID, setSelectedScheduledTaskID] = useState("");
  const [planTaskBusy, setPlanTaskBusy] = useState(false);
  const [startTaskBusy, setStartTaskBusy] = useState(false);
  const [planningTaskID, setPlanningTaskID] = useState("");
  const [startingTaskID, setStartingTaskID] = useState("");
  const [taskActionError, setTaskActionError] = useState("");
  const [parentDetailTaskID, setParentDetailTaskID] = useState("");
  const lastTaskTabRevalidateAttemptAt = useRef(0);
  const scheduledTaskViewActive = activePane.type === WorkspacePaneTypes.task && taskBoardView === "scheduled";
  const tasksQuery = useQuery({
    queryKey: TASKS_QUERY_KEY,
    queryFn: fetchGlobalTasks,
  });
  const scheduledTasksQuery = useQuery({
    queryKey: SCHEDULED_TASKS_QUERY_KEY,
    queryFn: fetchScheduledTasks,
    refetchInterval: scheduledTaskViewActive ? TASK_BOARD_POLL_DELAY_MS : false,
  });
  const { refetch: refetchScheduledTasks } = scheduledTasksQuery;
  const { dataUpdatedAt: tasksDataUpdatedAt, isFetching: tasksFetching, refetch: refetchTasks } = tasksQuery;
  const teamsQuery = useQuery({
    queryKey: workspaceQueryKeys.teams(),
    queryFn: fetchTeams,
  });

  const tasks = useMemo(() => tasksQuery.data ?? [], [tasksQuery.data]);
  const scheduledTasks = useMemo(() => scheduledTasksQuery.data ?? [], [scheduledTasksQuery.data]);
  const visibleScheduledTaskID = selectedScheduledTaskID || scheduledTasks[0]?.id || "";
  const scheduledTaskRunsCacheKey = useMemo(() => scheduledTaskRunsMetadataCacheKey(scheduledTasks), [scheduledTasks]);
  const scheduledTaskRunsQuery = useQuery({
    queryKey: scheduledTaskRunsQueryKey(visibleScheduledTaskID),
    queryFn: () => fetchScheduledTaskRuns(visibleScheduledTaskID),
    enabled: scheduledTaskViewActive && Boolean(visibleScheduledTaskID),
  });
  const scheduledTaskRuns = useMemo(() => scheduledTaskRunsQuery.data ?? [], [scheduledTaskRunsQuery.data]);
  const scheduledTaskRunQueries = useQuery({
    queryKey: scheduledTaskRunsAllQueryKey(scheduledTaskRunsCacheKey),
    queryFn: async () => {
      const runs = await Promise.all(scheduledTasks.map((item) => fetchScheduledTaskRuns(item.id)));
      return runs.flat();
    },
    enabled: scheduledTaskViewActive && scheduledTasks.length > 0,
    refetchInterval: (query) =>
      shouldPollScheduledTaskRuns(scheduledTaskViewActive, query.state.data ?? [], tasks)
        ? TASK_BOARD_POLL_DELAY_MS
        : false,
  });
  const allScheduledTaskRuns = useMemo(
    () => mergeScheduledTaskRuns(scheduledTaskRuns, scheduledTaskRunQueries.data ?? []),
    [scheduledTaskRuns, scheduledTaskRunQueries.data],
  );
  const visibleScheduledTaskRuns = useMemo(
    () => allScheduledTaskRuns.filter((run) => run.scheduled_task_id === visibleScheduledTaskID),
    [allScheduledTaskRuns, visibleScheduledTaskID],
  );
  const scheduledGeneratedTaskIDs = useMemo(
    () => new Set(allScheduledTaskRuns.map((run) => run.task_id).filter(Boolean)),
    [allScheduledTaskRuns],
  );
  const missingScheduledGeneratedTaskIDs = useMemo(() => {
    const taskIDs = new Set(tasks.map((task) => task.id));
    return Array.from(scheduledGeneratedTaskIDs)
      .filter((taskID) => !taskIDs.has(taskID))
      .sort();
  }, [scheduledGeneratedTaskIDs, tasks]);
  const boardTasks = useMemo(
    () =>
      tasks.filter((task) => {
        const rootTaskID = rootTaskForTask(tasks, task)?.id || task.id;
        return !scheduledGeneratedTaskIDs.has(rootTaskID) && !isSchedulerGeneratedTask(task);
      }),
    [scheduledGeneratedTaskIDs, tasks],
  );
  const teams = useMemo(() => teamsQuery.data ?? [], [teamsQuery.data]);
  const parentTasks = useMemo(() => rootTasks(boardTasks), [boardTasks]);
  const selectedTaskID = activePane.type === WorkspacePaneTypes.task ? String(activePane.id || "") : "";
  const selectedTask = useMemo(() => tasks.find((item) => item.id === selectedTaskID) ?? null, [selectedTaskID, tasks]);
  const activeRootTask = useMemo(() => rootTaskForTask(tasks, selectedTask), [selectedTask, tasks]);
  const visibleRootTask = activeRootTask ?? parentTasks[0] ?? null;
  const activeEventsTeamID =
    activeRootTask?.assignment_type === "team"
      ? activeRootTask.team_id
      : selectedTask?.assignment_type === "team"
        ? selectedTask.team_id
        : "";
  const shouldPollActiveTaskBoard = useMemo(() => shouldPollTaskBoard(tasks, activeRootTask), [activeRootTask, tasks]);
  const shouldPollScheduledGeneratedTasks = useMemo(
    () =>
      scheduledTaskViewActive &&
      Array.from(scheduledGeneratedTaskIDs).some((taskID) => {
        const task = tasks.find((item) => item.id === taskID) ?? null;
        const root = rootTaskForTask(tasks, task);
        return shouldPollTaskBoard(tasks, root);
      }),
    [scheduledGeneratedTaskIDs, scheduledTaskViewActive, tasks],
  );
  const shouldPollTasks = useMemo(
    () => shouldPollActiveTaskBoard || shouldPollScheduledGeneratedTasks || shouldPollTransitionalTasks(tasks),
    [shouldPollActiveTaskBoard, shouldPollScheduledGeneratedTasks, tasks],
  );
  const taskEventsQuery = useQuery({
    queryKey: teamEventsQueryKey(activeEventsTeamID),
    queryFn: () => fetchTeamEvents(activeEventsTeamID),
    enabled: Boolean(activeEventsTeamID),
    refetchInterval: shouldPollActiveTaskBoard ? TASK_BOARD_POLL_DELAY_MS : false,
  });
  const taskEvents = useMemo(() => taskEventsQuery.data ?? [], [taskEventsQuery.data]);
  const refreshScheduledTaskState = useCallback(async () => {
    const taskResult = await refetchScheduledTasks();
    const nextScheduledTasks = taskResult.data ?? scheduledTasks;
    const nextRunsCacheKey = scheduledTaskRunsMetadataCacheKey(nextScheduledTasks);
    const nextRuns = nextScheduledTasks.length
      ? (await Promise.all(nextScheduledTasks.map((item) => fetchScheduledTaskRuns(item.id)))).flat()
      : [];
    queryClient.setQueryData<WorkspaceScheduledTaskRun[]>(scheduledTaskRunsAllQueryKey(nextRunsCacheKey), nextRuns);
    const nextVisibleScheduledTaskID = selectedScheduledTaskID || nextScheduledTasks[0]?.id || "";
    if (nextVisibleScheduledTaskID) {
      queryClient.setQueryData<WorkspaceScheduledTaskRun[]>(
        scheduledTaskRunsQueryKey(nextVisibleScheduledTaskID),
        nextRuns.filter((run) => run.scheduled_task_id === nextVisibleScheduledTaskID),
      );
    }
  }, [queryClient, refetchScheduledTasks, scheduledTasks, selectedScheduledTaskID]);

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

  useEffect(() => {
    if (taskBoardView !== "scheduled") {
      return undefined;
    }
    if (missingScheduledGeneratedTaskIDs.length === 0) {
      return undefined;
    }
    let cancelled = false;
    const timer = window.setTimeout(async () => {
      if (cancelled) {
        return;
      }
      await refetchTasks();
    }, TASK_BOARD_POLL_DELAY_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [missingScheduledGeneratedTaskIDs, refetchTasks, taskBoardView]);

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

  async function createTask(draft: {
    agent_id?: string;
    assignment_id?: string;
    assignment_type?: "team" | "agent";
    team_id?: string;
    title: string;
    body?: string;
  }): Promise<void> {
    if (createTaskBusy) {
      return;
    }
    setCreateTaskBusy(true);
    setCreateTaskError("");
    try {
      const created = await createWorkspaceTask(draft);
      await tasksQuery.refetch();
      if (created.assignment_type === "team" && created.team_id) {
        await queryClient.invalidateQueries({ queryKey: teamEventsQueryKey(created.team_id) });
      }
      await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrap() });
      setShowCreateTaskModal(false);
      onSelectTask(created.id);
      if (created.assignment_type === "team") {
        await autoPlanAndStartTask(created.id, created.team_id);
      }
    } catch (err) {
      setCreateTaskError(errorMessage(err, t("taskCreateFailed")));
    } finally {
      setCreateTaskBusy(false);
    }
  }

  async function createScheduledTaskDraft(payload: CreateScheduledTaskPayload): Promise<void> {
    if (createScheduledTaskBusy) {
      return;
    }
    setCreateScheduledTaskBusy(true);
    setCreateScheduledTaskError("");
    try {
      const created = await createScheduledTask(payload);
      setShowCreateScheduledTaskModal(false);
      setTaskBoardView("scheduled");
      setSelectedScheduledTaskID(created.id);
      await scheduledTasksQuery.refetch();
    } catch (err) {
      setCreateScheduledTaskError(errorMessage(err, t("scheduledTaskCreateFailed")));
    } finally {
      setCreateScheduledTaskBusy(false);
    }
  }

  async function toggleScheduledTask(taskID: string, enabled: boolean): Promise<void> {
    if (!taskID) {
      return;
    }
    setScheduledTaskActionID(taskID);
    setScheduledTaskActionError("");
    try {
      await updateScheduledTask(taskID, { enabled });
      await scheduledTasksQuery.refetch();
    } catch (err) {
      setScheduledTaskActionError(errorMessage(err, t("scheduledTaskUpdateFailed")));
    } finally {
      setScheduledTaskActionID("");
    }
  }

  async function editScheduledTaskDraft(taskID: string, payload: UpdateScheduledTaskPayload): Promise<void> {
    if (!taskID || editScheduledTaskBusy) {
      return;
    }
    setEditScheduledTaskBusy(true);
    setEditScheduledTaskError("");
    try {
      const updated = await updateScheduledTask(taskID, payload);
      setEditingScheduledTaskID("");
      setSelectedScheduledTaskID(updated.id);
      await scheduledTasksQuery.refetch();
      await queryClient.invalidateQueries({ queryKey: scheduledTaskRunsQueryKey(taskID) });
    } catch (err) {
      setEditScheduledTaskError(errorMessage(err, t("scheduledTaskUpdateFailed")));
    } finally {
      setEditScheduledTaskBusy(false);
    }
  }

  async function removeScheduledTask(taskID: string): Promise<void> {
    if (!taskID) {
      return;
    }
    setScheduledTaskActionID(taskID);
    setScheduledTaskActionError("");
    try {
      await deleteScheduledTask(taskID);
      setSelectedScheduledTaskID("");
      await scheduledTasksQuery.refetch();
      await queryClient.removeQueries({ queryKey: scheduledTaskRunsQueryKey(taskID) });
    } catch (err) {
      setScheduledTaskActionError(errorMessage(err, t("scheduledTaskDeleteFailed")));
    } finally {
      setScheduledTaskActionID("");
    }
  }

  async function runScheduledTask(taskID: string): Promise<void> {
    if (!taskID) {
      return;
    }
    setScheduledTaskActionID(taskID);
    setScheduledTaskActionError("");
    try {
      const run = await runScheduledTaskNow(taskID);
      queryClient.setQueryData<WorkspaceScheduledTaskRun[]>(scheduledTaskRunsQueryKey(taskID), (current = []) =>
        mergeScheduledTaskRuns([run], current),
      );
      queryClient.setQueryData<WorkspaceScheduledTaskRun[]>(
        scheduledTaskRunsAllQueryKey(scheduledTaskRunsMetadataCacheKey(scheduledTasks)),
        (current = []) => mergeScheduledTaskRuns([run], current),
      );
      await refreshScheduledTaskState();
      const taskResult = await tasksQuery.refetch();
      if (taskResult.data) {
        queryClient.setQueryData<WorkspaceTask[]>(TASKS_QUERY_KEY, (current) =>
          mergeWorkspaceTaskList(current ?? [], taskResult.data ?? []),
        );
      }
      if (run.task_id) {
        onSelectTask(run.task_id);
      }
    } catch (err) {
      setScheduledTaskActionError(
        isScheduledTaskAlreadyTriggeredError(err)
          ? t("scheduledTaskAlreadyTriggered")
          : errorMessage(err, t("scheduledTaskRunFailed")),
      );
    } finally {
      setScheduledTaskActionID("");
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
    if (target.assignment_type !== "team") {
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
    if (target.assignment_type !== "team") {
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
    scheduledTaskCount: scheduledTasks.length,
    taskBoardView,
    setTaskBoardView,
    planningTaskID,
    startingTaskID,
    openParentTaskDetail,
    openCreateTaskModal: () => {
      setCreateTaskError("");
      setShowCreateScheduledTaskModal(false);
      setShowCreateTaskModal(true);
    },
    openCreateScheduledTaskModal: () => {
      setCreateScheduledTaskError("");
      setShowCreateTaskModal(false);
      setShowCreateScheduledTaskModal(true);
    },
    taskViewProps: {
      t,
      agents,
      tasks,
      taskBoardTasks: boardTasks,
      taskEvents,
      teams,
      scheduledTasks,
      scheduledTaskRuns: visibleScheduledTaskRuns,
      selectedScheduledTaskID: visibleScheduledTaskID,
      activeView: taskBoardView,
      selectedTask,
      selectedTaskID,
      loading: tasksQuery.isLoading || scheduledTasksQuery.isLoading,
      error:
        (tasksQuery.isError ? errorMessage(tasksQuery.error, t("tasksLoadFailed")) : "") ||
        (scheduledTasksQuery.isError ? errorMessage(scheduledTasksQuery.error, t("scheduledTasksLoadFailed")) : "") ||
        (teamsQuery.isError ? errorMessage(teamsQuery.error, t("teamsLoadFailed")) : ""),
      createTaskBusy,
      createTaskError,
      createScheduledTaskBusy,
      createScheduledTaskError,
      editScheduledTaskBusy,
      editScheduledTaskError,
      planTaskBusy,
      planningTaskID,
      startTaskBusy,
      startingTaskID,
      taskActionError,
      scheduledTaskActionID,
      scheduledTaskActionError,
      rooms,
      showCreateTaskModal,
      showCreateScheduledTaskModal,
      editingScheduledTaskID,
      parentDetailTaskID,
      onCloseCreateTaskModal: () => setShowCreateTaskModal(false),
      onCloseCreateScheduledTaskModal: () => setShowCreateScheduledTaskModal(false),
      onCloseEditScheduledTaskModal: () => setEditingScheduledTaskID(""),
      onOpenCreateTaskModal: () => {
        setCreateTaskError("");
        setShowCreateScheduledTaskModal(false);
        setShowCreateTaskModal(true);
      },
      onOpenCreateScheduledTaskModal: () => {
        setCreateScheduledTaskError("");
        setShowCreateTaskModal(false);
        setShowCreateScheduledTaskModal(true);
      },
      onSelectTaskBoardView: setTaskBoardView,
      onOpenEditScheduledTaskModal: (taskID: string) => {
        setEditScheduledTaskError("");
        setEditingScheduledTaskID(taskID);
      },
      onCloseParentTaskDetail: () => setParentDetailTaskID(""),
      onCreateTask: createTask,
      onCreateScheduledTask: createScheduledTaskDraft,
      onEditScheduledTask: editScheduledTaskDraft,
      onDeleteScheduledTask: removeScheduledTask,
      onPlanTask: planTask,
      onStartTask: startTask,
      onRunScheduledTask: runScheduledTask,
      onToggleScheduledTask: toggleScheduledTask,
      onSelectScheduledTask: setSelectedScheduledTaskID,
      onRefresh: () => {
        void tasksQuery.refetch();
        void refreshScheduledTaskState();
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

function isScheduledTaskAlreadyTriggeredError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const status = "status" in error ? (error as ApiError).status : 0;
  const message = "message" in error ? String((error as ApiError).message || "").toLowerCase() : "";
  return status === 409 || message.includes("scheduled task already has an active generated task");
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

function shouldPollScheduledTaskRuns(
  scheduledTaskViewActive: boolean,
  runs: readonly WorkspaceScheduledTaskRun[],
  tasks: readonly WorkspaceTask[],
): boolean {
  return scheduledTaskViewActive && runs.some((run) => scheduledTaskRunNeedsPolling(run, tasks));
}

function scheduledTaskRunNeedsPolling(run: WorkspaceScheduledTaskRun, tasks: readonly WorkspaceTask[]): boolean {
  const status = String(run.status || "")
    .trim()
    .toLowerCase();
  if (["failed", "completed", "done", "canceled", "cancelled"].includes(status)) {
    return false;
  }

  const taskID = String(run.task_id || "").trim();
  if (!taskID) {
    return true;
  }
  const task = tasks.find((item) => item.id === taskID) ?? null;
  if (!task) {
    return true;
  }
  const root = rootTaskForTask(tasks, task);
  return shouldPollTaskBoard(tasks, root);
}

function isSchedulerGeneratedTask(task: WorkspaceTask): boolean {
  return String(task.created_by || "").trim() === "scheduler";
}

function mergeWorkspaceTaskList(current: readonly WorkspaceTask[], next: readonly WorkspaceTask[]): WorkspaceTask[] {
  const currentByID = new Map(current.map((task) => [task.id, task]));
  return next.map((task) => {
    const existing = currentByID.get(task.id);
    return existing && workspaceTasksEqual(existing, task) ? existing : task;
  });
}

function mergeScheduledTaskRuns(
  current: readonly WorkspaceScheduledTaskRun[],
  next: readonly WorkspaceScheduledTaskRun[],
): WorkspaceScheduledTaskRun[] {
  const byID = new Map<string, WorkspaceScheduledTaskRun>();
  [...current, ...next].forEach((run) => {
    if (run.id) {
      byID.set(run.id, run);
    }
  });
  return Array.from(byID.values()).sort((left, right) => right.triggered_at.localeCompare(left.triggered_at));
}

function scheduledTaskRunsMetadataCacheKey(tasks: readonly WorkspaceScheduledTask[]): string {
  return tasks
    .map((item) => [item.id, item.last_run_at, item.updated_at, item.next_run_at, item.enabled ? "1" : "0"].join(":"))
    .join("|");
}

function workspaceTasksEqual(left: WorkspaceTask, right: WorkspaceTask): boolean {
  return (
    left.id === right.id &&
    left.assignment_type === right.assignment_type &&
    left.assignment_id === right.assignment_id &&
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
