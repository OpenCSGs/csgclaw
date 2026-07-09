import { useEffect, useMemo, useState } from "react";
import type { ComponentProps, Dispatch, ReactNode, SetStateAction } from "react";
import { Bot, CalendarClock, ChevronDown, Pencil, Play, Trash2, Users, X } from "lucide-react";
import {
  Button,
  type ButtonVariant,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Select,
} from "@/components/ui";
import { TaskStatusPill, TaskSubtaskIndicator } from "@/components/business";
import type { CreateWorkspaceTaskPayload } from "@/api/tasks";
import type { CreateScheduledTaskPayload, UpdateScheduledTaskPayload } from "@/api/scheduledTasks";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import {
  scheduledTaskRecurrenceLabel,
  type ScheduledTaskRecurrence,
  type WorkspaceScheduledTask,
  type WorkspaceScheduledTaskRun,
} from "@/models/scheduledTasks";
import {
  TASK_BOARD_STATUSES,
  displayTaskAssignedAgent,
  displayTaskAssignmentTarget,
  displayTaskClaimedAgent,
  displayTaskRoomTitle,
  displayTeam,
  formatTaskUpdatedAt,
  formatTaskUpdatedRelative,
  resolveTaskSidebarPhase,
  rootTaskForTask,
  rootTasks,
  taskChildren,
  taskExecutionRoomID,
  taskStatusLabel,
} from "@/models/tasks";
import type { TaskSidebarPhase, WorkspaceTask, WorkspaceTeam, WorkspaceTeamEvent } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TasksView.module.css";

const TASK_TITLE_MAX_LENGTH = 80;
const EMPTY_AGENTS: AgentLike[] = [];

function moduleSuffixStyle(prefix: string, suffix: string | undefined): string {
  if (!suffix) {
    return "";
  }
  const normalized = suffix.replace(/-+([a-zA-Z0-9_])/g, (_, char: string) => char.toUpperCase());
  const key = `${prefix}${normalized.charAt(0).toUpperCase()}${normalized.slice(1)}`;
  return styles[key] ?? "";
}

type TaskCreateDraft = {
  assignee: string;
  title: string;
  description: string;
};

type TaskCreateFieldErrors = {
  assignment?: string;
  title?: string;
};

type ScheduledTaskFormDraft = {
  agentID: string;
  date: string;
  expiresDate: string;
  prompt: string;
  recurrence: ScheduledTaskRecurrence;
  time: string;
  title: string;
};

type ScheduledTaskFormFieldErrors = {
  agentID?: string;
  date?: string;
  prompt?: string;
  time?: string;
  title?: string;
};

const emptyCreateDraft: TaskCreateDraft = {
  assignee: "",
  title: "",
  description: "",
};

const emptyScheduledTaskDraft: ScheduledTaskFormDraft = {
  agentID: "",
  date: "",
  expiresDate: "",
  prompt: "",
  recurrence: "once",
  time: "",
  title: "",
};

function truncateTaskTitle(value: string): string {
  const chars = Array.from(value);
  if (chars.length <= TASK_TITLE_MAX_LENGTH) {
    return value;
  }
  return `${chars.slice(0, TASK_TITLE_MAX_LENGTH - 3).join("")}...`;
}

type TaskAssignmentTarget = {
  id: string;
  type: "team" | "agent";
};

function taskAssignmentValue(type: TaskAssignmentTarget["type"], id: string): string {
  return `${type}:${id}`;
}

function parseTaskAssignmentValue(value: string): TaskAssignmentTarget | null {
  const [type, ...rest] = String(value || "").split(":");
  const id = rest.join(":").trim();
  if ((type === "team" || type === "agent") && id) {
    return { type, id };
  }
  return null;
}

function assignableAgents(agents: readonly AgentLike[]): AgentLike[] {
  return agents
    .filter((item) => String(item.id || "").trim())
    .filter(
      (item) =>
        String(item.role || "")
          .trim()
          .toLowerCase() !== "manager",
    )
    .slice()
    .sort((left, right) => displayAgent(left).localeCompare(displayAgent(right)));
}

function displayAgent(agent: AgentLike): string {
  return String(agent.name || agent.user_name || agent.id || "").trim();
}

function taskAssignmentOptions(teams: readonly WorkspaceTeam[], agents: readonly AgentLike[], t: TranslateFn) {
  const workerAgents = assignableAgents(agents);
  return [
    ...(teams.length
      ? [
          { value: "__group_teams", label: t("taskAssignmentTeamGroup"), disabled: true },
          ...teams.map((team) => ({
            value: taskAssignmentValue("team", team.id),
            label: displayTeam(team),
            description: team.lead_agent_id,
          })),
        ]
      : []),
    ...(workerAgents.length
      ? [
          { value: "__group_agents", label: t("taskAssignmentAgentGroup"), disabled: true },
          ...workerAgents.map((agent) => ({
            value: taskAssignmentValue("agent", String(agent.id || "")),
            label: displayAgent(agent),
            description: String(agent.id || ""),
          })),
        ]
      : []),
  ];
}

function scheduledTaskAgentOptions(agents: readonly AgentLike[]) {
  return assignableAgents(agents).map((agent) => ({
    value: String(agent.id || ""),
    label: displayAgent(agent),
    description: String(agent.id || ""),
  }));
}

function agentNameLookup(agents: readonly AgentLike[]): ReadonlyMap<string, string> {
  const lookup = new Map<string, string>();
  agents.forEach((agent) => {
    const name = displayAgent(agent);
    const ids = [agent.id, agent.user_id].map((value) => String(value || "").trim()).filter(Boolean);
    ids.forEach((id) => {
      if (name && !lookup.has(id)) {
        lookup.set(id, name);
      }
    });
  });
  return lookup;
}

function displayAgentByID(agentID: string, lookup: ReadonlyMap<string, string>, fallbackName?: string): string {
  const id = String(agentID || "").trim();
  const name = String(fallbackName || "").trim();
  return id ? name || lookup.get(id) || id : name;
}

function todayInputValue(now = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
}

function timeInputValue(now = new Date()): string {
  return `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
}

function localDateTimeISO(date: string, time: string): string {
  const parsed = new Date(`${date}T${time}:00`);
  return Number.isNaN(parsed.getTime()) ? "" : parsed.toISOString();
}

function nextDailyRunAtISO(time: string, now = new Date()): string {
  const candidate = new Date(`${todayInputValue(now)}T${time}:00`);
  if (Number.isNaN(candidate.getTime())) {
    return "";
  }
  if (candidate.getTime() <= now.getTime()) {
    candidate.setDate(candidate.getDate() + 1);
  }
  return candidate.toISOString();
}

function scheduledTaskNextRunAtISO(draft: ScheduledTaskFormDraft): string {
  if (draft.date) {
    return localDateTimeISO(draft.date, draft.time);
  }
  if (draft.recurrence === "daily" && draft.time) {
    return nextDailyRunAtISO(draft.time);
  }
  return "";
}

function dateInputFromISO(value: string): string {
  if (!value) {
    return "";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "";
  }
  return todayInputValue(parsed);
}

function timeInputFromISO(value: string): string {
  if (!value) {
    return "";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "";
  }
  return timeInputValue(parsed);
}

function scheduledTaskDraftFromTask(task: WorkspaceScheduledTask): ScheduledTaskFormDraft {
  return {
    agentID: task.agent_id,
    date: dateInputFromISO(task.next_run_at),
    expiresDate: dateInputFromISO(task.expires_at),
    prompt: task.prompt,
    recurrence: task.recurrence,
    time: timeInputFromISO(task.next_run_at),
    title: task.title,
  };
}

function scheduledTaskRunTimeChanged(current: string, next: string): boolean {
  if (!current || !next) {
    return current !== next;
  }
  const currentTime = new Date(current).getTime();
  const nextTime = new Date(next).getTime();
  if (Number.isNaN(currentTime) || Number.isNaN(nextTime)) {
    return current !== next;
  }
  return currentTime !== nextTime;
}

function scheduledTaskRunErrorLabel(error: string, t: TranslateFn): string {
  const message = String(error || "").trim();
  if (!message) {
    return "";
  }
  if (/^agent\s+"[^"]+"\s+not found$/i.test(message) || /^agent\s+not found$/i.test(message)) {
    return t("scheduledTaskRunAgentMissingError");
  }
  return message;
}

function isTerminalWorkspaceTaskStatus(status: string): boolean {
  return status === "completed" || status === "failed" || status === "cancelled";
}

type VoidOrPromise = void | Promise<void>;
type TaskBoardView = "tasks" | "scheduled";

export type TasksViewProps = {
  agents?: AgentLike[];
  createTaskBusy?: boolean;
  createTaskError?: string;
  error?: string;
  loading?: boolean;
  onCloseCreateTaskModal?: () => void;
  onCloseEditScheduledTaskModal?: () => void;
  onCloseParentTaskDetail?: () => void;
  onCloseTaskDetails?: () => VoidOrPromise;
  onCreateTask?: (payload: CreateWorkspaceTaskPayload) => VoidOrPromise;
  onCreateScheduledTask?: (payload: CreateScheduledTaskPayload) => VoidOrPromise;
  onDeleteScheduledTask?: (taskID: string) => VoidOrPromise;
  onEditScheduledTask?: (taskID: string, payload: UpdateScheduledTaskPayload) => VoidOrPromise;
  onOpenConversation?: (roomID: string) => VoidOrPromise;
  onOpenCreateTaskModal?: () => VoidOrPromise;
  onOpenCreateScheduledTaskModal?: () => VoidOrPromise;
  onOpenEditScheduledTaskModal?: (taskID: string) => void;
  onPlanTask?: (taskID: string) => VoidOrPromise;
  onRefresh?: () => VoidOrPromise;
  onRunScheduledTask?: (taskID: string) => VoidOrPromise;
  onSelectScheduledTask?: (taskID: string) => void;
  onSelectTaskBoardView?: (view: TaskBoardView) => void;
  onStartTask?: (taskID: string) => VoidOrPromise;
  onToggleScheduledTask?: (taskID: string, enabled: boolean) => VoidOrPromise;
  onViewParentDetail?: (taskID: string) => VoidOrPromise;
  parentDetailTaskID?: string;
  planTaskBusy?: boolean;
  planningTaskID?: string;
  rooms?: readonly Pick<IMConversation, "id">[];
  selectedTask?: WorkspaceTask | null;
  selectedScheduledTaskID?: string;
  showCreateTaskModal?: boolean;
  editingScheduledTaskID?: string;
  startTaskBusy?: boolean;
  startingTaskID?: string;
  taskActionError?: string;
  taskEvents?: WorkspaceTeamEvent[];
  taskBoardTasks?: WorkspaceTask[];
  tasks?: WorkspaceTask[];
  activeView?: TaskBoardView;
  createTaskModalView?: TaskBoardView;
  t?: TranslateFn;
  teams?: WorkspaceTeam[];
  scheduledTasks?: WorkspaceScheduledTask[];
  scheduledTaskRuns?: WorkspaceScheduledTaskRun[];
  createScheduledTaskBusy?: boolean;
  createScheduledTaskError?: string;
  editScheduledTaskBusy?: boolean;
  editScheduledTaskError?: string;
  scheduledTaskActionID?: string;
  scheduledTaskActionError?: string;
};

export function TasksView({
  t = (key) => key,
  agents = EMPTY_AGENTS,
  tasks = [],
  taskBoardTasks = tasks,
  taskEvents = [],
  teams = [],
  loading = false,
  error = "",
  taskActionError = "",
  planTaskBusy = false,
  startTaskBusy = false,
  createTaskBusy = false,
  createTaskError = "",
  createScheduledTaskBusy = false,
  createScheduledTaskError = "",
  editScheduledTaskBusy = false,
  editScheduledTaskError = "",
  showCreateTaskModal = false,
  activeView = "tasks",
  createTaskModalView = "tasks",
  editingScheduledTaskID = "",
  scheduledTasks = [],
  scheduledTaskRuns = [],
  selectedScheduledTaskID = "",
  scheduledTaskActionID = "",
  scheduledTaskActionError = "",
  parentDetailTaskID = "",
  planningTaskID = "",
  startingTaskID = "",
  rooms,
  onCloseCreateTaskModal,
  onCloseEditScheduledTaskModal,
  onCloseParentTaskDetail,
  onCloseTaskDetails,
  onCreateTask,
  onCreateScheduledTask,
  onDeleteScheduledTask,
  onEditScheduledTask,
  onOpenCreateTaskModal,
  onOpenCreateScheduledTaskModal,
  onRefresh = () => {},
  onRunScheduledTask = () => {},
  onSelectScheduledTask = () => {},
  onSelectTaskBoardView = () => {},
  onToggleScheduledTask = () => {},
  onOpenEditScheduledTaskModal,
  onOpenConversation = () => {},
}: TasksViewProps) {
  const [activeCreateView, setActiveCreateView] = useState<TaskBoardView>(createTaskModalView);
  const parentTasks = useMemo(() => rootTasks(taskBoardTasks), [taskBoardTasks]);
  const assignmentOptions = useMemo(() => taskAssignmentOptions(teams, agents, t), [agents, t, teams]);
  const scheduledAgentOptions = useMemo(() => scheduledTaskAgentOptions(agents), [agents]);
  const agentNames = useMemo(() => agentNameLookup(agents), [agents]);
  const selectedScheduledTask = useMemo(
    () => scheduledTasks.find((item) => item.id === selectedScheduledTaskID) ?? scheduledTasks[0] ?? null,
    [scheduledTasks, selectedScheduledTaskID],
  );
  const editingScheduledTask = useMemo(
    () => scheduledTasks.find((item) => item.id === editingScheduledTaskID) ?? null,
    [editingScheduledTaskID, scheduledTasks],
  );
  const hasActiveGeneratedTask = useMemo(() => {
    for (const run of scheduledTaskRuns) {
      if (!run.task_id || run.status === "failed") {
        continue;
      }
      const task = tasks.find((item) => item.id === run.task_id);
      if (!task || !isTerminalWorkspaceTaskStatus(task.status)) {
        return true;
      }
    }
    return false;
  }, [scheduledTaskRuns, tasks]);
  const [parentDialogTaskID, setParentDialogTaskID] = useState("");
  const dialogStateRootTask = useMemo(
    () => (parentDialogTaskID ? (parentTasks.find((item) => item.id === parentDialogTaskID) ?? null) : null),
    [parentDialogTaskID, parentTasks],
  );
  const parentDetailTask = useMemo(() => {
    if (!parentDetailTaskID) {
      return null;
    }
    const task = taskBoardTasks.find((item) => item.id === parentDetailTaskID) ?? null;
    return rootTaskForTask(taskBoardTasks, task) ?? task;
  }, [parentDetailTaskID, taskBoardTasks]);
  const parentDialogTask = parentDetailTask ?? dialogStateRootTask;
  const parentDialogChildTasks = useMemo(
    () => (parentDialogTask ? taskChildren(taskBoardTasks, parentDialogTask.id) : []),
    [parentDialogTask, taskBoardTasks],
  );
  const parentColumns = useMemo(() => boardColumnsForParentTasks(parentTasks), [parentTasks]);
  const [createDraft, setCreateDraft] = useState<TaskCreateDraft>(emptyCreateDraft);
  const [createFieldErrors, setCreateFieldErrors] = useState<TaskCreateFieldErrors>({});
  const [scheduledDraft, setScheduledDraft] = useState<ScheduledTaskFormDraft>(emptyScheduledTaskDraft);
  const [scheduledFieldErrors, setScheduledFieldErrors] = useState<ScheduledTaskFormFieldErrors>({});
  const [editScheduledDraft, setEditScheduledDraft] = useState<ScheduledTaskFormDraft>(emptyScheduledTaskDraft);
  const [editScheduledFieldErrors, setEditScheduledFieldErrors] = useState<ScheduledTaskFormFieldErrors>({});
  const [deletingScheduledTask, setDeletingScheduledTask] = useState<WorkspaceScheduledTask | null>(null);
  const [selectedGeneratedTaskID, setSelectedGeneratedTaskID] = useState("");
  const selectedGeneratedTask = useMemo(
    () => tasks.find((item) => item.id === selectedGeneratedTaskID) ?? null,
    [selectedGeneratedTaskID, tasks],
  );
  const selectedGeneratedChildTasks = useMemo(
    () => (selectedGeneratedTask ? taskChildren(tasks, selectedGeneratedTask.id) : []),
    [selectedGeneratedTask, tasks],
  );
  const [conversationOpenError, setConversationOpenError] = useState("");
  const availableRoomIDs = useMemo(() => (rooms ? new Set(rooms.map((room) => room.id)) : null), [rooms]);

  useEffect(() => {
    if (!showCreateTaskModal) {
      return;
    }
    setActiveCreateView(createTaskModalView);
    setCreateDraft(emptyCreateDraft);
    setCreateFieldErrors({});
  }, [createTaskModalView, showCreateTaskModal]);

  useEffect(() => {
    if (!showCreateTaskModal || activeCreateView !== "scheduled") {
      return;
    }
    const now = new Date();
    setScheduledDraft({
      ...emptyScheduledTaskDraft,
      date: todayInputValue(now),
      time: timeInputValue(now),
    });
    setScheduledFieldErrors({});
  }, [activeCreateView, showCreateTaskModal]);

  useEffect(() => {
    if (!editingScheduledTask) {
      return;
    }
    setEditScheduledDraft(scheduledTaskDraftFromTask(editingScheduledTask));
    setEditScheduledFieldErrors({});
  }, [editingScheduledTask]);

  useEffect(() => {
    setSelectedGeneratedTaskID(scheduledTaskRuns.find((run) => run.task_id)?.task_id ?? "");
  }, [scheduledTaskRuns, selectedScheduledTask?.id]);

  async function submitCreateTask() {
    const title = truncateTaskTitle(createDraft.title.trim());
    const description = createDraft.description.trim();
    const assignment = parseTaskAssignmentValue(createDraft.assignee);
    const nextFieldErrors: TaskCreateFieldErrors = {};
    if (!title) {
      nextFieldErrors.title = t("taskTitleRequired");
    }
    if (!assignment) {
      nextFieldErrors.assignment = t("taskAssignmentRequired");
    }
    if (!title || !assignment) {
      setCreateFieldErrors(nextFieldErrors);
      return;
    }
    setCreateFieldErrors({});
    const payload: CreateWorkspaceTaskPayload = {
      assignment_type: assignment.type,
      assignment_id: assignment.id,
      title,
      execution_channel: "csgclaw",
    };
    if (description) {
      payload.body = description;
    }
    if (assignment.type === "team") {
      payload.team_id = assignment.id;
    } else {
      payload.agent_id = assignment.id;
    }
    await onCreateTask?.(payload);
  }

  function clearCreateFieldError(field: keyof TaskCreateFieldErrors) {
    setCreateFieldErrors((current) => {
      if (!current[field]) {
        return current;
      }
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  function validateScheduledTaskDraft(draft: ScheduledTaskFormDraft): {
    errors: ScheduledTaskFormFieldErrors;
    nextRunAt: string;
    prompt: string;
    title: string;
  } {
    const title = truncateTaskTitle(draft.title.trim());
    const prompt = draft.prompt.trim();
    const nextRunAt = scheduledTaskNextRunAtISO(draft);
    const requiresDate = draft.recurrence !== "daily";
    const errors: ScheduledTaskFormFieldErrors = {};
    if (!title) {
      errors.title = t("taskTitleRequired");
    }
    if (!draft.agentID) {
      errors.agentID = t("scheduledTaskAgentRequired");
    }
    if (!prompt) {
      errors.prompt = t("scheduledTaskPromptRequired");
    }
    if ((requiresDate && !draft.date) || (draft.date && !nextRunAt)) {
      errors.date = t("scheduledTaskDateRequired");
    }
    if (!draft.time || (!nextRunAt && !errors.date)) {
      errors.time = t("scheduledTaskTimeRequired");
    }
    return { errors, nextRunAt, prompt, title };
  }

  async function submitCreateScheduledTask() {
    const result = validateScheduledTaskDraft(scheduledDraft);
    if (Object.keys(result.errors).length) {
      setScheduledFieldErrors(result.errors);
      return;
    }
    setScheduledFieldErrors({});
    const payload: CreateScheduledTaskPayload = {
      title: result.title,
      agent_id: scheduledDraft.agentID,
      prompt: result.prompt,
      recurrence: scheduledDraft.recurrence,
      first_run_at: result.nextRunAt,
      enabled: true,
    };
    if (scheduledDraft.expiresDate) {
      payload.expires_at = localDateTimeISO(scheduledDraft.expiresDate, "23:59");
    }
    await onCreateScheduledTask?.(payload);
    onSelectTaskBoardView("scheduled");
  }

  async function submitEditScheduledTask() {
    if (!editingScheduledTask) {
      return;
    }
    const result = validateScheduledTaskDraft(editScheduledDraft);
    if (Object.keys(result.errors).length) {
      setEditScheduledFieldErrors(result.errors);
      return;
    }
    setEditScheduledFieldErrors({});
    const nextRunChanged = scheduledTaskRunTimeChanged(editingScheduledTask.next_run_at, result.nextRunAt);
    const shouldReactivateEditedSchedule =
      !editingScheduledTask.enabled && (!editingScheduledTask.next_run_at || nextRunChanged);
    await onEditScheduledTask?.(editingScheduledTask.id, {
      title: result.title,
      agent_id: editScheduledDraft.agentID,
      prompt: result.prompt,
      recurrence: editScheduledDraft.recurrence,
      next_run_at: result.nextRunAt,
      expires_at: editScheduledDraft.expiresDate ? localDateTimeISO(editScheduledDraft.expiresDate, "23:59") : null,
      enabled: shouldReactivateEditedSchedule ? true : editingScheduledTask.enabled,
    });
  }

  function clearScheduledFieldError(field: keyof ScheduledTaskFormFieldErrors) {
    setScheduledFieldErrors((current) => {
      if (!current[field]) {
        return current;
      }
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  function clearEditScheduledFieldError(field: keyof ScheduledTaskFormFieldErrors) {
    setEditScheduledFieldErrors((current) => {
      if (!current[field]) {
        return current;
      }
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  function openRootTaskDetail(task: WorkspaceTask) {
    setParentDialogTaskID(task.id);
  }

  function closeRootTaskDetail() {
    setParentDialogTaskID("");
    onCloseParentTaskDetail?.();
    void onCloseTaskDetails?.();
  }

  function openRunTask(taskID: string) {
    setSelectedGeneratedTaskID(taskID);
    setConversationOpenError("");
  }

  function openTaskConversation(roomID: string) {
    const normalizedRoomID = String(roomID || "").trim();
    if (!normalizedRoomID) {
      return;
    }
    if (availableRoomIDs && !availableRoomIDs.has(normalizedRoomID)) {
      setConversationOpenError(t("taskConversationAgentDeleted"));
      return;
    }
    setConversationOpenError("");
    void onOpenConversation(normalizedRoomID);
  }

  async function confirmDeleteScheduledTask() {
    if (!deletingScheduledTask) {
      return;
    }
    await onDeleteScheduledTask?.(deletingScheduledTask.id);
    setDeletingScheduledTask(null);
  }

  return (
    <section className={classNames("entity-pane", "tasks-pane", styles.tasksPane)}>
      {error ? <div className="form-error">{error}</div> : null}
      {taskActionError ? (
        <div className={classNames("form-error", styles.tasksActionError)}>{taskActionError}</div>
      ) : null}
      {scheduledTaskActionError ? (
        <div className={classNames("form-error", styles.tasksActionError)}>{scheduledTaskActionError}</div>
      ) : null}
      {conversationOpenError ? (
        <div className={classNames("form-error", styles.tasksActionError)}>{conversationOpenError}</div>
      ) : null}
      {!error ? (
        <div className={styles.tasksBoardWorkbench} aria-busy={loading}>
          <div className={styles.tasksBoardPanel}>
            <div className={classNames(styles.headerRow, styles.justifyEnd, styles.tasksBoardHead)}>
              <div className={styles.tasksBoardHeading}>
                <h1>{t("mainTaskBoardTitle")}</h1>
              </div>
              <TaskActionStrip
                t={t}
                showConversation={false}
                showParentDetail={false}
                canPlanTask={false}
                canStartTask={false}
                planTaskBusy={planTaskBusy}
                startTaskBusy={startTaskBusy}
                onCreateTask={activeView === "tasks" ? onOpenCreateTaskModal : undefined}
                onCreateScheduledTask={activeView === "scheduled" ? onOpenCreateScheduledTaskModal : undefined}
                onRefresh={onRefresh}
              />
            </div>
            <div className={styles.taskContentLayout}>
              <div className={styles.taskContentMain}>
                {activeView === "tasks" ? (
                  <div className={styles.tasksKanbanScroll} role="region" aria-label={t("mainTaskBoardTitle")}>
                    <div className={styles.tasksKanban}>
                      {parentColumns.map((column) => (
                        <section
                          key={column.status}
                          className={classNames(
                            styles.taskBoardColumn,
                            moduleSuffixStyle("taskBoardColumn", column.status),
                          )}
                        >
                          <header className={classNames(styles.headerRow, styles.taskBoardColumnHead)}>
                            <span className={styles.taskBoardColumnTitle}>
                              <TaskBoardStatusIcon status={column.status} />
                              <span>{taskStatusLabel(column.status, t)}</span>
                              <strong>{column.tasks.length}</strong>
                            </span>
                          </header>
                          <div className={styles.taskBoardColumnBody}>
                            {column.tasks.length ? (
                              column.tasks.map((task) => {
                                const children = taskChildren(taskBoardTasks, task.id);
                                const phase = resolveTaskSidebarPhase(task, children, {
                                  planningTaskID,
                                  startingTaskID,
                                });
                                return (
                                  <ParentTaskBoardCard
                                    key={task.id}
                                    task={task}
                                    children={children}
                                    agentNames={agentNames}
                                    phase={phase}
                                    t={t}
                                    onSelect={() => openRootTaskDetail(task)}
                                  />
                                );
                              })
                            ) : (
                              <div className={styles.taskBoardEmpty}>{t("taskBoardColumnEmpty")}</div>
                            )}
                          </div>
                        </section>
                      ))}
                    </div>
                  </div>
                ) : (
                  <div className={styles.scheduledTaskLayout}>
                    <section className={styles.scheduledTaskList} aria-label={t("scheduledTasksTab")}>
                      <div className={styles.scheduledTaskListHead}>
                        <strong>{t("scheduledTasksTab")}</strong>
                      </div>
                      {scheduledTasks.length ? (
                        scheduledTasks.map((item) => (
                          <button
                            key={item.id}
                            type="button"
                            className={styles.scheduledTaskRow}
                            data-active={selectedScheduledTask?.id === item.id ? true : undefined}
                            onClick={() => onSelectScheduledTask(item.id)}
                          >
                            <span>
                              <CalendarClock size={15} aria-hidden="true" />
                              <strong>{item.title}</strong>
                            </span>
                            <small>
                              {scheduledTaskRecurrenceLabel(item.recurrence, t)} ·{" "}
                              {formatTaskUpdatedAt(item.next_run_at)}
                            </small>
                          </button>
                        ))
                      ) : (
                        <div className={styles.taskBoardEmpty}>{t("scheduledTasksEmpty")}</div>
                      )}
                    </section>
                    <section className={styles.scheduledTaskDetail} aria-label={t("scheduledTaskDetailTitle")}>
                      {selectedScheduledTask ? (
                        <>
                          <div className={styles.scheduledTaskDetailHead}>
                            <div>
                              <h2>{selectedScheduledTask.title}</h2>
                              <p>{scheduledTaskRecurrenceLabel(selectedScheduledTask.recurrence, t)}</p>
                            </div>
                            <div className={styles.scheduledTaskActions}>
                              <Button
                                variant="secondaryGray"
                                size="sm"
                                disabled={scheduledTaskActionID === selectedScheduledTask.id}
                                onClick={() => onOpenEditScheduledTaskModal?.(selectedScheduledTask.id)}
                              >
                                <Pencil size={14} aria-hidden="true" />
                                {t("scheduledTaskEdit")}
                              </Button>
                              <Button
                                variant="outlineDanger"
                                size="sm"
                                disabled={scheduledTaskActionID === selectedScheduledTask.id}
                                onClick={() => setDeletingScheduledTask(selectedScheduledTask)}
                              >
                                <Trash2 size={14} aria-hidden="true" />
                                {t("scheduledTaskDelete")}
                              </Button>
                              {selectedScheduledTask.enabled || selectedScheduledTask.next_run_at ? (
                                <Button
                                  variant="secondaryGray"
                                  size="sm"
                                  disabled={scheduledTaskActionID === selectedScheduledTask.id}
                                  onClick={() =>
                                    onToggleScheduledTask(selectedScheduledTask.id, !selectedScheduledTask.enabled)
                                  }
                                >
                                  {selectedScheduledTask.enabled ? t("scheduledTaskDisable") : t("scheduledTaskEnable")}
                                </Button>
                              ) : (
                                <Button variant="secondaryGray" size="sm" disabled>
                                  {t("scheduledTaskCompleted")}
                                </Button>
                              )}
                              <Button
                                variant="primary"
                                size="sm"
                                disabled={scheduledTaskActionID === selectedScheduledTask.id || hasActiveGeneratedTask}
                                onClick={() => onRunScheduledTask(selectedScheduledTask.id)}
                              >
                                <Play size={14} aria-hidden="true" />
                                {hasActiveGeneratedTask ? t("scheduledTaskActiveTask") : t("scheduledTaskRunNow")}
                              </Button>
                            </div>
                          </div>
                          <dl className={styles.scheduledTaskMeta}>
                            <div>
                              <dt>{t("scheduledTaskAgentLabel")}</dt>
                              <dd title={selectedScheduledTask.agent_id}>
                                {displayAgentByID(
                                  selectedScheduledTask.agent_id,
                                  agentNames,
                                  selectedScheduledTask.agent_name,
                                )}
                              </dd>
                            </div>
                            <div>
                              <dt>{t("scheduledTaskNextRunLabel")}</dt>
                              <dd>{formatTaskUpdatedAt(selectedScheduledTask.next_run_at)}</dd>
                            </div>
                            <div>
                              <dt>{t("scheduledTaskLastRunLabel")}</dt>
                              <dd>
                                {selectedScheduledTask.last_run_at
                                  ? formatTaskUpdatedAt(selectedScheduledTask.last_run_at)
                                  : "-"}
                              </dd>
                            </div>
                          </dl>
                          <div className={styles.scheduledTaskPrompt}>{selectedScheduledTask.prompt}</div>
                          <div className={styles.scheduledTaskRuns}>
                            <h3>{t("scheduledTaskRunsTitle")}</h3>
                            {scheduledTaskRuns.length ? (
                              <div className={styles.scheduledTaskRunList}>
                                {scheduledTaskRuns.map((run) => (
                                  <div key={run.id} className={styles.scheduledTaskRunRow}>
                                    <span>{formatTaskUpdatedAt(run.triggered_at)}</span>
                                    <strong>
                                      {run.status === "failed"
                                        ? t("scheduledTaskRunFailedStatus")
                                        : t("scheduledTaskRunTriggeredStatus")}
                                    </strong>
                                    {run.task_id ? (
                                      <button
                                        type="button"
                                        data-active={selectedGeneratedTaskID === run.task_id ? true : undefined}
                                        onClick={() => openRunTask(run.task_id)}
                                      >
                                        {run.task_id}
                                      </button>
                                    ) : null}
                                    {run.error ? (
                                      <small title={run.error}>{scheduledTaskRunErrorLabel(run.error, t)}</small>
                                    ) : null}
                                  </div>
                                ))}
                              </div>
                            ) : (
                              <div className={styles.taskBoardEmpty}>{t("scheduledTaskRunsEmpty")}</div>
                            )}
                          </div>
                          <GeneratedTaskInlineDetail
                            task={selectedGeneratedTask}
                            childTasks={selectedGeneratedChildTasks}
                            agentNames={agentNames}
                            teams={teams}
                            taskEvents={taskEvents}
                            t={t}
                            onOpenConversation={openTaskConversation}
                          />
                        </>
                      ) : (
                        <div className={styles.taskBoardEmpty}>{t("scheduledTasksEmpty")}</div>
                      )}
                    </section>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      ) : null}
      <TaskDetailDialog
        t={t}
        title={t("taskParentDetailTitle")}
        task={parentDialogTask}
        childCount={parentDialogChildTasks.length}
        childTasks={parentDialogChildTasks}
        agentNames={agentNames}
        teams={teams}
        taskEvents={taskEvents}
        open={Boolean(parentDialogTask)}
        onClose={closeRootTaskDetail}
        onOpenConversation={openTaskConversation}
      />
      <DialogRoot open={showCreateTaskModal} onOpenChange={(open) => (!open ? onCloseCreateTaskModal?.() : null)}>
        <DialogContent className={styles.taskCreateDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>
                {activeCreateView === "scheduled" ? t("scheduledTaskCreateTitle") : t("taskCreateTitle")}
              </DialogTitle>
              <DialogDescription>
                {activeCreateView === "scheduled" ? t("scheduledTaskCreateSubtitle") : t("taskCreateSubtitle")}
              </DialogDescription>
            </div>
            <TaskDialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <div className={styles.taskCreateTabs} role="tablist" aria-label={t("tasksActionsLabel")}>
              <button
                type="button"
                role="tab"
                aria-selected={activeCreateView === "tasks"}
                data-active={activeCreateView === "tasks" ? true : undefined}
                onClick={() => setActiveCreateView("tasks")}
              >
                {t("taskCreate")}
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeCreateView === "scheduled"}
                data-active={activeCreateView === "scheduled" ? true : undefined}
                onClick={() => setActiveCreateView("scheduled")}
              >
                {t("scheduledTaskCreate")}
              </button>
            </div>
            {activeCreateView === "tasks" ? (
              <div className={classNames(styles.taskCreateForm, styles.taskCreateFormCompact)}>
                <label
                  className={classNames("field", styles.taskCreateField)}
                  data-invalid={createFieldErrors.title ? true : undefined}
                >
                  <span>{t("taskTitleLabel")}</span>
                  <input
                    value={createDraft.title}
                    maxLength={TASK_TITLE_MAX_LENGTH}
                    aria-describedby={createFieldErrors.title ? "task-create-title-error" : undefined}
                    aria-invalid={createFieldErrors.title ? true : undefined}
                    onInput={(event) => {
                      setCreateDraft((current) => ({ ...current, title: event.currentTarget.value }));
                      clearCreateFieldError("title");
                    }}
                    placeholder={t("taskTitlePlaceholder")}
                  />
                  {createFieldErrors.title ? (
                    <span id="task-create-title-error" className="form-error" role="alert">
                      {createFieldErrors.title}
                    </span>
                  ) : null}
                </label>
                <label className={classNames("field", styles.taskCreateField)}>
                  <span>{t("taskDescriptionLabel")}</span>
                  <textarea
                    value={createDraft.description}
                    aria-label={t("taskDescriptionLabel")}
                    onInput={(event) => {
                      setCreateDraft((current) => ({ ...current, description: event.currentTarget.value }));
                    }}
                    placeholder={t("taskDescriptionPlaceholder")}
                  />
                </label>
                <label
                  className={classNames("field", styles.taskCreateField)}
                  data-invalid={createFieldErrors.assignment ? true : undefined}
                >
                  <span>{t("taskAssignmentLabel")}</span>
                  <Select
                    value={createDraft.assignee}
                    onValueChange={(assignee) => {
                      setCreateDraft((current) => ({ ...current, assignee }));
                      clearCreateFieldError("assignment");
                    }}
                    triggerProps={{
                      "aria-describedby": createFieldErrors.assignment ? "task-create-assignment-error" : undefined,
                      "aria-invalid": createFieldErrors.assignment ? true : undefined,
                      "aria-label": t("taskAssignmentLabel"),
                    }}
                    options={assignmentOptions}
                    placeholder={t("taskAssignmentPlaceholder")}
                  />
                  {createFieldErrors.assignment ? (
                    <span id="task-create-assignment-error" className="form-error" role="alert">
                      {createFieldErrors.assignment}
                    </span>
                  ) : null}
                </label>
              </div>
            ) : (
              <ScheduledTaskFormFields
                draft={scheduledDraft}
                errors={scheduledFieldErrors}
                scheduledAgentOptions={scheduledAgentOptions}
                t={t}
                onChange={setScheduledDraft}
                onClearError={clearScheduledFieldError}
              />
            )}
            {activeCreateView === "tasks" && createTaskError ? (
              <div className={classNames("form-error", styles.taskCreateError)}>{createTaskError}</div>
            ) : null}
            {activeCreateView === "scheduled" && createScheduledTaskError ? (
              <div className={classNames("form-error", styles.taskCreateError)}>{createScheduledTaskError}</div>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button variant="secondaryGray" size="md" onClick={onCloseCreateTaskModal}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={activeCreateView === "scheduled" ? createScheduledTaskBusy : createTaskBusy}
              loadingLabel={activeCreateView === "scheduled" ? t("scheduledTaskCreating") : t("taskCreating")}
              disabled={activeCreateView === "scheduled" ? createScheduledTaskBusy : createTaskBusy}
              onClick={activeCreateView === "scheduled" ? submitCreateScheduledTask : submitCreateTask}
            >
              {activeCreateView === "scheduled" ? t("scheduledTaskCreateSubmit") : t("taskCreateSubmit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={Boolean(editingScheduledTask)}
        onOpenChange={(open) => (!open ? onCloseEditScheduledTaskModal?.() : null)}
      >
        <DialogContent className={styles.taskCreateDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("scheduledTaskEditTitle")}</DialogTitle>
              <DialogDescription>{t("scheduledTaskEditSubtitle")}</DialogDescription>
            </div>
            <TaskDialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <ScheduledTaskFormFields
              draft={editScheduledDraft}
              errors={editScheduledFieldErrors}
              scheduledAgentOptions={scheduledAgentOptions}
              t={t}
              onChange={setEditScheduledDraft}
              onClearError={clearEditScheduledFieldError}
            />
            {editScheduledTaskError ? (
              <div className={classNames("form-error", styles.taskCreateError)}>{editScheduledTaskError}</div>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button variant="secondaryGray" size="md" onClick={onCloseEditScheduledTaskModal}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={editScheduledTaskBusy}
              loadingLabel={t("scheduledTaskCreating")}
              disabled={editScheduledTaskBusy}
              onClick={submitEditScheduledTask}
            >
              {t("scheduledTaskSaveChanges")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={Boolean(deletingScheduledTask)}
        onOpenChange={(open) => {
          if (!open) {
            setDeletingScheduledTask(null);
          }
        }}
      >
        <DialogContent className={styles.taskCreateDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("scheduledTaskDelete")}</DialogTitle>
              <DialogDescription>
                {t("scheduledTaskDeleteConfirmMessage", { title: deletingScheduledTask?.title || "" })}
              </DialogDescription>
            </div>
            <TaskDialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="secondaryGray"
              size="md"
              disabled={Boolean(deletingScheduledTask && scheduledTaskActionID === deletingScheduledTask.id)}
              onClick={() => setDeletingScheduledTask(null)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              size="md"
              loading={Boolean(deletingScheduledTask && scheduledTaskActionID === deletingScheduledTask.id)}
              onClick={confirmDeleteScheduledTask}
            >
              {t("scheduledTaskDelete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </section>
  );
}

type ScheduledTaskFormFieldsProps = {
  draft: ScheduledTaskFormDraft;
  errors: ScheduledTaskFormFieldErrors;
  onChange: Dispatch<SetStateAction<ScheduledTaskFormDraft>>;
  onClearError: (field: keyof ScheduledTaskFormFieldErrors) => void;
  scheduledAgentOptions: ReturnType<typeof scheduledTaskAgentOptions>;
  t: TranslateFn;
};

function ScheduledTaskFormFields({
  draft,
  errors,
  scheduledAgentOptions,
  t,
  onChange,
  onClearError,
}: ScheduledTaskFormFieldsProps) {
  return (
    <div className={styles.taskCreateForm}>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.title ? true : undefined}>
        <span>{t("taskTitleLabel")}</span>
        <input
          value={draft.title}
          maxLength={TASK_TITLE_MAX_LENGTH}
          placeholder={t("taskTitlePlaceholder")}
          onInput={(event) => {
            onChange((current) => ({ ...current, title: event.currentTarget.value }));
            onClearError("title");
          }}
        />
        {errors.title ? <span className="form-error">{errors.title}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.agentID ? true : undefined}>
        <span>{t("scheduledTaskAgentLabel")}</span>
        <Select
          value={draft.agentID}
          onValueChange={(agentID) => {
            onChange((current) => ({ ...current, agentID }));
            onClearError("agentID");
          }}
          options={scheduledAgentOptions}
          placeholder={t("scheduledTaskAgentPlaceholder")}
          triggerProps={{ "aria-label": t("scheduledTaskAgentLabel") }}
        />
        {errors.agentID ? <span className="form-error">{errors.agentID}</span> : null}
      </label>
      <label
        className={classNames("field", styles.taskCreateField, styles.span2)}
        data-invalid={errors.prompt ? true : undefined}
      >
        <span>{t("scheduledTaskPromptLabel")}</span>
        <textarea
          value={draft.prompt}
          placeholder={t("scheduledTaskPromptPlaceholder")}
          onInput={(event) => {
            onChange((current) => ({ ...current, prompt: event.currentTarget.value }));
            onClearError("prompt");
          }}
        />
        {errors.prompt ? <span className="form-error">{errors.prompt}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)}>
        <span>{t("scheduledTaskRecurrenceLabel")}</span>
        <Select
          value={draft.recurrence}
          onValueChange={(recurrence) =>
            onChange((current) => ({ ...current, recurrence: recurrence as ScheduledTaskRecurrence }))
          }
          options={[
            { value: "once", label: t("scheduledTaskRecurrenceOnce") },
            { value: "daily", label: t("scheduledTaskRecurrenceDaily") },
            { value: "weekly", label: t("scheduledTaskRecurrenceWeekly") },
            { value: "monthly", label: t("scheduledTaskRecurrenceMonthly") },
          ]}
          triggerProps={{ "aria-label": t("scheduledTaskRecurrenceLabel") }}
        />
      </label>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.date ? true : undefined}>
        <span>{t("scheduledTaskDateLabel")}</span>
        <input
          type="date"
          value={draft.date}
          onInput={(event) => {
            onChange((current) => ({ ...current, date: event.currentTarget.value }));
            onClearError("date");
          }}
        />
        {errors.date ? <span className="form-error">{errors.date}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.time ? true : undefined}>
        <span>{t("scheduledTaskTimeLabel")}</span>
        <input
          type="time"
          value={draft.time}
          onInput={(event) => {
            onChange((current) => ({ ...current, time: event.currentTarget.value }));
            onClearError("time");
          }}
        />
        {errors.time ? <span className="form-error">{errors.time}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)}>
        <span>{t("scheduledTaskExpiresLabel")}</span>
        <input
          type="date"
          value={draft.expiresDate}
          onInput={(event) => onChange((current) => ({ ...current, expiresDate: event.currentTarget.value }))}
        />
      </label>
    </div>
  );
}

type TaskActionStripProps = {
  canPlanTask: boolean;
  canStartTask: boolean;
  conversationLabel?: string;
  conversationShortLabel?: string;
  onCreateScheduledTask?: () => VoidOrPromise;
  onCreateTask?: () => VoidOrPromise;
  onOpenConversation?: () => VoidOrPromise;
  onPlanTask?: () => VoidOrPromise;
  onRefresh: () => VoidOrPromise;
  onStartTask?: () => VoidOrPromise;
  onViewParentDetail?: () => VoidOrPromise;
  planTaskBusy: boolean;
  showConversation: boolean;
  showParentDetail?: boolean;
  startTaskBusy: boolean;
  t: TranslateFn;
};

function TaskActionStrip({
  t,
  showConversation,
  showParentDetail = false,
  canPlanTask,
  canStartTask,
  planTaskBusy,
  startTaskBusy,
  conversationLabel = undefined,
  conversationShortLabel = undefined,
  onOpenConversation = undefined,
  onCreateTask = undefined,
  onCreateScheduledTask = undefined,
  onViewParentDetail = undefined,
  onPlanTask = undefined,
  onStartTask = undefined,
  onRefresh,
}: TaskActionStripProps) {
  return (
    <div
      className={classNames(styles.headerRow, styles.justifyEnd, styles.tasksToolbar)}
      aria-label={t("tasksActionsLabel")}
    >
      <TaskToolbarButton label={t("tasksRefreshShort")} title={t("tasksRefresh")} onClick={onRefresh} />
      {onCreateTask ? (
        <TaskToolbarButton label={t("taskCreate")} title={t("taskCreate")} onClick={onCreateTask} />
      ) : null}
      {onCreateScheduledTask ? (
        <TaskToolbarButton
          label={t("scheduledTaskCreate")}
          title={t("scheduledTaskCreate")}
          onClick={onCreateScheduledTask}
        />
      ) : null}
      {showParentDetail ? (
        <TaskToolbarButton label={t("taskDetailsShort")} title={t("taskViewDetails")} onClick={onViewParentDetail} />
      ) : null}
      {canPlanTask ? (
        <TaskToolbarButton
          label={t("taskPlan")}
          onClick={onPlanTask}
          loading={planTaskBusy}
          loadingLabel={t("taskPlanLoading")}
          disabled={planTaskBusy || startTaskBusy}
        />
      ) : null}
      {canStartTask ? (
        <TaskToolbarButton
          label={t("taskStart")}
          variant="primary"
          onClick={onStartTask}
          loading={startTaskBusy}
          loadingLabel={t("taskStartLoading")}
          disabled={startTaskBusy || planTaskBusy}
        />
      ) : null}
      {showConversation ? (
        <TaskToolbarButton
          label={conversationShortLabel || t("taskOpenConversationShort")}
          title={conversationLabel || t("taskOpenConversation")}
          onClick={onOpenConversation}
        />
      ) : null}
    </div>
  );
}

type TaskToolbarButtonProps = {
  label: string;
  title?: string;
  variant?: ButtonVariant;
} & ComponentProps<typeof Button>;

function TaskToolbarButton({ label, title = label, variant = "secondaryGray", ...props }: TaskToolbarButtonProps) {
  return (
    <Button
      className={classNames(styles.taskToolbarButton, variant === "secondaryGray" && styles.taskToolbarButtonSecondary)}
      aria-label={title}
      title={title}
      size="sm"
      variant={variant}
      {...props}
    >
      {label}
    </Button>
  );
}

function TaskBoardStatusIcon({ status }: { status: string }) {
  const progress = taskBoardStatusProgress(status);

  return (
    <svg className={styles.taskBoardStatusIcon} viewBox="0 0 14 14" fill="none" aria-hidden="true">
      <TaskBoardProgressCircle progress={progress}>
        {progress === 1 ? (
          <path
            d="M10.951 4.24896C11.283 4.58091 11.283 5.11909 10.951 5.45104L5.95104 10.451C5.61909 10.783 5.0809 10.783 4.74896 10.451L2.74896 8.45104C2.41701 8.11909 2.41701 7.5809 2.74896 7.24896C3.0809 6.91701 3.61909 6.91701 3.95104 7.24896L5.35 8.64792L9.74896 4.24896C10.0809 3.91701 10.6191 3.91701 10.951 4.24896Z"
            fill="white"
            stroke="none"
          />
        ) : status === "failed" || status === "cancelled" || status === "canceled" ? (
          <path d="M5 5 L9 9 M9 5 L5 9" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" />
        ) : null}
      </TaskBoardProgressCircle>
    </svg>
  );
}

function TaskBoardProgressCircle({ progress, children }: { progress: number; children?: ReactNode }) {
  return (
    <>
      <circle cx={7} cy={7} r={6} fill="none" stroke="currentColor" strokeWidth={1.5} />
      {progress === 1 ? (
        <circle cx={7} cy={7} r={6} fill="currentColor" />
      ) : progress > 0 ? (
        <path d={taskBoardPiePath(7, 7, 3.5, progress)} fill="currentColor" />
      ) : null}
      {children}
    </>
  );
}

function taskBoardPiePath(cx: number, cy: number, radius: number, progress: number): string {
  const angle = 2 * Math.PI * progress;
  const endX = cx + radius * Math.sin(angle);
  const endY = cy - radius * Math.cos(angle);
  const largeArc = progress > 0.5 ? 1 : 0;
  return `M${cx},${cy} L${cx},${cy - radius} A${radius},${radius} 0 ${largeArc},1 ${endX},${endY} Z`;
}

function taskBoardStatusProgress(status: string): number {
  if (status === "completed" || status === "done") {
    return 1;
  }
  if (status === "blocked" || status === "in_review") {
    return 0.75;
  }
  if (status === "in_progress" || status === "running") {
    return 0.5;
  }
  return 0;
}

type ParentTaskBoardCardProps = {
  agentNames: ReadonlyMap<string, string>;
  children: WorkspaceTask[];
  onSelect: () => void;
  phase: TaskSidebarPhase;
  t: TranslateFn;
  task: WorkspaceTask;
};

function ParentTaskBoardCard({ task, children, agentNames, phase, t, onSelect }: ParentTaskBoardCardProps) {
  const description = task.body || task.plan_summary || task.result || task.error || t("tasksDetailPlaceholder");
  const activeWorker = taskActiveWorker(task, children, t, agentNames);
  const updatedRelative = formatTaskUpdatedRelative(task.updated_at, document.documentElement.lang);
  const updatedLabel = updatedRelative === "-" ? "" : t("taskCardUpdatedAt", { time: updatedRelative });
  const assignmentTarget = displayTaskAssignmentTargetName(task, agentNames);

  return (
    <button
      type="button"
      className={classNames(styles.taskBoardCard, styles.parentTaskBoardCard)}
      onClick={onSelect}
      title={`${task.id} ${task.title}`}
    >
      <span className={styles.taskBoardCardTopline}>
        <span className={styles.taskBoardCardId}>{task.id}</span>
        <span className={styles.taskBoardCardActions}>
          {activeWorker ? (
            <span className={styles.taskBoardCardWorkerBadge} title={`${activeWorker.name} ${activeWorker.label}`}>
              <Bot size={12} strokeWidth={1.9} aria-hidden="true" />
              <span>{activeWorker.label}</span>
            </span>
          ) : null}
          <TaskSubtaskIndicator subtasks={children} phase={phase} t={t} compact />
        </span>
      </span>
      <strong className={classNames(styles.lineClampText, styles.taskBoardCardTitle)}>{task.title}</strong>
      <span className={classNames(styles.lineClampText, styles.taskBoardCardDescription)}>{description}</span>
      <span className={styles.taskBoardCardFooter}>
        {assignmentTarget ? (
          <span className={styles.taskBoardCardTeam} title={assignmentTarget}>
            <span className={styles.taskBoardCardTeamIcon} aria-hidden="true">
              {task.assignment_type === "agent" ? (
                <Bot size={13} strokeWidth={1.8} />
              ) : (
                <Users size={13} strokeWidth={1.8} />
              )}
            </span>
            <span>{assignmentTarget}</span>
          </span>
        ) : null}
        {updatedLabel ? <span className={styles.taskBoardCardUpdated}>{updatedLabel}</span> : null}
      </span>
    </button>
  );
}

type TaskDetailDialogProps = {
  agentNames: ReadonlyMap<string, string>;
  childCount?: number;
  childTasks?: WorkspaceTask[];
  onClose?: () => void;
  onOpenConversation: (roomID: string) => VoidOrPromise;
  open: boolean;
  t: TranslateFn;
  task: WorkspaceTask | null;
  taskEvents?: WorkspaceTeamEvent[];
  teams?: WorkspaceTeam[];
  title?: string;
};

type GeneratedTaskInlineDetailProps = {
  agentNames: ReadonlyMap<string, string>;
  childTasks?: WorkspaceTask[];
  onOpenConversation: (roomID: string) => VoidOrPromise;
  t: TranslateFn;
  task: WorkspaceTask | null;
  taskEvents?: WorkspaceTeamEvent[];
  teams?: WorkspaceTeam[];
};

function GeneratedTaskInlineDetail({
  t,
  task,
  agentNames,
  childTasks = [],
  teams = [],
  taskEvents = [],
  onOpenConversation,
}: GeneratedTaskInlineDetailProps) {
  const locale = document.documentElement.lang;
  const isParentDetail = Boolean(task && !task.parent_id);
  const detailEvents = useMemo(
    () => (task ? taskEventsForDetail(task, childTasks, taskEvents) : []),
    [childTasks, task, taskEvents],
  );
  const timelineEntries = useMemo(
    () => (task ? taskTimelineEntries(task, childTasks, detailEvents, t, locale) : []),
    [childTasks, detailEvents, locale, t, task],
  );
  const timelineGroups = useMemo(
    () => (task && isParentDetail ? taskTimelineGroups(task, childTasks, detailEvents, t, locale) : []),
    [childTasks, detailEvents, isParentDetail, locale, t, task],
  );
  const metaTags = useMemo(
    () => (task ? taskMetaTags(task, childTasks.length, t, locale, agentNames) : []),
    [agentNames, childTasks.length, locale, t, task],
  );
  const detailRoomID = useMemo(
    () => (task ? taskExecutionRoomID(task, childTasks, teams) : ""),
    [childTasks, task, teams],
  );

  return (
    <section className={styles.generatedTaskDetail} aria-label={t("generatedTaskDetailTitle")}>
      <header className={styles.generatedTaskDetailHead}>
        <h3>{t("generatedTaskDetailTitle")}</h3>
        {task && (task.room_id || detailRoomID) ? (
          <Button variant="secondaryGray" size="sm" onClick={() => onOpenConversation(detailRoomID || task.room_id)}>
            {t("taskOpenConversation")}
          </Button>
        ) : null}
      </header>
      <div className={styles.generatedTaskPanel}>
        {task ? (
          <>
            <header className={styles.generatedTaskSummary}>
              <div className={styles.generatedTaskTitleRow}>
                <strong>{task.title}</strong>
                <TaskStatusPill status={task.status} t={t} showFullLabel />
              </div>
              <p>{task.id}</p>
            </header>
            <section className={classNames(styles.detailBlock, styles.generatedTaskDescription)}>
              <h4>{t("taskDescriptionLabel")}</h4>
              <p>{task.body || t("tasksDetailPlaceholder")}</p>
            </section>
            <div className={styles.generatedTaskDetailGrid}>
              <section className={classNames(styles.detailBlock, styles.generatedTaskActivity)}>
                <h4>{t("taskActivityLabel")}</h4>
                {isParentDetail ? (
                  <TaskGroupedActivityTimeline
                    groups={timelineGroups}
                    agentNames={agentNames}
                    emptyLabel={t("taskActivityEmpty")}
                    t={t}
                  />
                ) : (
                  <TaskActivityTimeline entries={timelineEntries} emptyLabel={t("taskActivityEmpty")} />
                )}
              </section>
              <aside className={styles.generatedTaskMeta} aria-label={t("taskMetadataLabel")}>
                <h4>{t("taskMetadataLabel")}</h4>
                <div className={styles.taskDetailTags}>
                  {metaTags.map((item) => (
                    <TaskMetaTag key={item.key} label={item.label} value={item.value} />
                  ))}
                </div>
              </aside>
            </div>
          </>
        ) : (
          <div className={styles.generatedTaskEmpty}>{t("generatedTaskDetailEmpty")}</div>
        )}
      </div>
    </section>
  );
}

function TaskDetailDialog({
  t,
  title = "",
  task,
  agentNames,
  childCount = undefined,
  childTasks = [],
  teams = [],
  taskEvents = [],
  open,
  onClose,
  onOpenConversation,
}: TaskDetailDialogProps) {
  const dialogTitle = task?.title || title || t("tasksDetailLabel");
  const locale = document.documentElement.lang;
  const isParentDetail = Boolean(task && !task.parent_id);
  const detailEvents = useMemo(
    () => (task ? taskEventsForDetail(task, childTasks, taskEvents) : []),
    [childTasks, task, taskEvents],
  );
  const timelineEntries = useMemo(
    () => (task ? taskTimelineEntries(task, childTasks, detailEvents, t, locale) : []),
    [childTasks, detailEvents, locale, t, task],
  );
  const timelineGroups = useMemo(
    () => (task && isParentDetail ? taskTimelineGroups(task, childTasks, detailEvents, t, locale) : []),
    [childTasks, detailEvents, isParentDetail, locale, t, task],
  );
  const metaTags = useMemo(
    () => (task ? taskMetaTags(task, childCount, t, locale, agentNames) : []),
    [agentNames, childCount, locale, t, task],
  );
  const detailRoomID = useMemo(
    () => (task ? taskExecutionRoomID(task, childTasks, teams) : ""),
    [childTasks, task, teams],
  );
  const activeWorker = useMemo(
    () => (task && isParentDetail ? taskActiveWorker(task, childTasks, t, agentNames) : null),
    [agentNames, childTasks, isParentDetail, t, task],
  );

  return (
    <DialogRoot open={open} onOpenChange={(nextOpen) => (!nextOpen ? onClose?.() : null)}>
      <DialogContent className={styles.taskDetailDialog}>
        <DialogHeader className={styles.taskDetailDialogHeader}>
          <div className={styles.taskDetailDialogHeading}>
            <div className={styles.taskDetailDialogTitleRow}>
              <DialogTitle className={styles.taskDetailDialogTitle} title={dialogTitle}>
                {dialogTitle}
              </DialogTitle>
              {task ? <TaskStatusPill status={task.status} t={t} showFullLabel /> : null}
            </div>
            <DialogDescription className={styles.taskDetailDialogSubtitle}>
              {task ? task.id : t("tasksSelectHint")}
            </DialogDescription>
          </div>
          <div className={styles.taskDetailDialogTools}>
            {activeWorker ? <TaskActiveWorkerBadge worker={activeWorker} /> : null}
            <TaskDialogCloseButton label={t("close")} />
          </div>
        </DialogHeader>
        {task ? (
          <>
            <DialogBody className={styles.taskDetailDialogBody}>
              <div className={styles.taskDetailLayout}>
                <main className={styles.taskDetailMain}>
                  <section className={classNames(styles.detailBlock, styles.taskDetailDescriptionBlock)}>
                    <h3>{t("taskDescriptionLabel")}</h3>
                    <p>{task.body || t("tasksDetailPlaceholder")}</p>
                  </section>
                  <section className={classNames(styles.detailBlock, styles.taskDetailActivityBlock)}>
                    <h3>{t("taskActivityLabel")}</h3>
                    {isParentDetail ? (
                      <TaskGroupedActivityTimeline
                        groups={timelineGroups}
                        agentNames={agentNames}
                        emptyLabel={t("taskActivityEmpty")}
                        t={t}
                      />
                    ) : (
                      <TaskActivityTimeline entries={timelineEntries} emptyLabel={t("taskActivityEmpty")} />
                    )}
                  </section>
                </main>
                <aside className={styles.taskDetailAside} aria-label={t("taskMetadataLabel")}>
                  {isParentDetail ? <TaskDependencyGraph tasks={childTasks} t={t} /> : null}
                  <h3>{t("taskMetadataLabel")}</h3>
                  <div className={styles.taskDetailTags}>
                    {metaTags.map((item) => (
                      <TaskMetaTag key={item.key} label={item.label} value={item.value} />
                    ))}
                  </div>
                </aside>
              </div>
            </DialogBody>
            <DialogFooter className={styles.taskDialogActions}>
              <Button variant="secondaryGray" size="md" onClick={onClose}>
                {t("close")}
              </Button>
              <Button variant="primary" size="md" onClick={() => onOpenConversation(detailRoomID || task.room_id)}>
                {t("taskOpenConversation")}
              </Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </DialogRoot>
  );
}

function TaskDialogCloseButton({ label }: { label: string }) {
  return (
    <DialogClose asChild>
      <button type="button" className={styles.taskDialogCloseBtn} aria-label={label} title={label}>
        <X size={18} strokeWidth={1.75} aria-hidden="true" />
      </button>
    </DialogClose>
  );
}

type TaskActiveWorker = {
  label: string;
  name: string;
  tone: "working";
};

function TaskActiveWorkerBadge({ worker }: { worker: TaskActiveWorker }) {
  return (
    <div
      className={classNames(styles.taskActiveWorker, moduleSuffixStyle("taskActiveWorker", worker.tone))}
      title={`${worker.name} ${worker.label}`}
    >
      <span className={styles.taskActiveAvatar} aria-hidden="true">
        <Bot size={14} strokeWidth={1.9} />
      </span>
      <span className={styles.taskActiveWorkerName}>{worker.name}</span>
      <span>{worker.label}</span>
    </div>
  );
}

type TaskTimelineEntry = {
  id: string;
  title: string;
  subject: string;
  meta: string;
  body: string;
  tone?: "success" | "warning" | "danger";
  order: number;
};

type TaskTimelineGroup = {
  entries: TaskTimelineEntry[];
  kind: "parent" | "child";
  task: WorkspaceTask;
};

type TaskMetaTagItem = {
  key: string;
  label: string;
  value: ReactNode;
};

function TaskGroupedActivityTimeline({
  groups,
  agentNames,
  emptyLabel,
  t,
}: {
  agentNames: ReadonlyMap<string, string>;
  emptyLabel: string;
  groups: TaskTimelineGroup[];
  t: TranslateFn;
}) {
  const [expandedTaskIDs, setExpandedTaskIDs] = useState<Set<string>>(() => new Set());
  const hasEntries = groups.some((group) => group.entries.length > 0);
  const parentGroup = groups.find((group) => group.kind === "parent") ?? null;
  const childGroups = groups
    .filter((group) => group.kind === "child")
    .sort(
      (left, right) =>
        timelineGroupOrder(left) - timelineGroupOrder(right) || left.task.id.localeCompare(right.task.id),
    );
  const parentEntries = parentGroup?.entries ?? [];
  const leadingParentEntries = parentEntries.length > 1 ? parentEntries.slice(0, 1) : parentEntries;
  const trailingParentEntries = parentEntries.length > 1 ? parentEntries.slice(1) : [];
  const defaultExpandedTaskID = defaultExpandedChildTaskID(childGroups);
  const childGroupSignature = childGroups
    .map((group) => `${group.task.id}:${group.task.status}:${timelineGroupOrder(group)}:${group.entries.length}`)
    .join("|");

  useEffect(() => {
    setExpandedTaskIDs(defaultExpandedTaskID ? new Set([defaultExpandedTaskID]) : new Set());
  }, [childGroupSignature, defaultExpandedTaskID]);

  if (!hasEntries) {
    return <div className={styles.taskActivityEmpty}>{emptyLabel}</div>;
  }

  function toggleTask(taskID: string) {
    setExpandedTaskIDs((current) => {
      const next = new Set(current);
      if (next.has(taskID)) {
        next.delete(taskID);
      } else {
        next.add(taskID);
      }
      return next;
    });
  }

  return (
    <div className={styles.taskGroupedActivityList}>
      {parentGroup ? (
        <header className={classNames(styles.taskActivityGroupHead, styles.taskActivityRootHead)}>
          <div className={styles.taskActivityGroupTitle}>
            <span className={styles.taskActivityGroupKind}>{t("taskTimelineMainTask")}</span>
            <strong>{parentGroup.task.id}</strong>
            <span>{parentGroup.task.title}</span>
          </div>
          <div className={styles.taskActivityGroupActions}>
            <span>{t("taskTimelineEventsCount", { count: parentGroup.entries.length + childGroups.length })}</span>
          </div>
        </header>
      ) : null}
      <ol className={classNames(styles.taskActivityList, styles.taskCombinedActivityList)}>
        {leadingParentEntries.map((entry) => (
          <TaskActivityTimelineItem key={entry.id} entry={entry} />
        ))}
        {childGroups.length ? (
          <li className={classNames(styles.taskActivityItem, styles.taskActivityChildStack)}>
            <span className={styles.taskActivityMarker} aria-hidden="true" />
            <article className={styles.taskActivityContent}>
              <header className={styles.taskActivityHead}>
                <div className={styles.taskActivityTitleRow}>
                  <strong>{t("taskTimelineChildTask")}</strong>
                  <span className={styles.taskActivitySubject}>
                    {t("taskTimelineEventsCount", { count: childGroups.length })}
                  </span>
                </div>
              </header>
              <div className={styles.taskChildActivityAccordion}>
                {childGroups.map((group) => {
                  const expanded = expandedTaskIDs.has(group.task.id);
                  const entryCount = group.entries.length;
                  const latestEntry = group.entries[entryCount - 1];
                  const assignee = displayTaskWorkerName(group.task, agentNames) || t("taskAssigneeUnassigned");
                  return (
                    <section key={`child-${group.task.id}`} className={styles.taskChildActivityItem}>
                      <button
                        type="button"
                        aria-expanded={expanded}
                        onClick={() => toggleTask(group.task.id)}
                        className={styles.taskChildActivityTrigger}
                      >
                        <span className={styles.taskChildActivityArrow} aria-hidden="true">
                          <ChevronDown size={16} strokeWidth={1.9} />
                        </span>
                        <span className={classNames(styles.childActivityMetaRow, styles.taskChildActivityMain)}>
                          <span className={styles.taskActivityGroupKind}>{t("taskTimelineChildTask")}</span>
                          <strong>{group.task.id}</strong>
                          <span>{group.task.title}</span>
                        </span>
                        <span className={classNames(styles.childActivityMetaRow, styles.taskChildActivityTags)}>
                          <span>{assignee}</span>
                          <span>{t("taskTimelineEventsCount", { count: entryCount })}</span>
                          <strong>{expanded ? t("taskTimelineCollapse") : t("taskTimelineExpand")}</strong>
                        </span>
                      </button>
                      {!expanded && latestEntry ? (
                        <div className={styles.taskChildActivitySummary}>
                          <span>{latestEntry.title}</span>
                          <span>{latestEntry.meta}</span>
                        </div>
                      ) : null}
                      {expanded ? (
                        <div className={styles.taskChildActivityPanel}>
                          <TaskActivityTimeline entries={group.entries} emptyLabel={emptyLabel} />
                        </div>
                      ) : null}
                    </section>
                  );
                })}
              </div>
            </article>
          </li>
        ) : null}
        {trailingParentEntries.map((entry) => (
          <TaskActivityTimelineItem key={entry.id} entry={entry} />
        ))}
      </ol>
    </div>
  );
}

function TaskActivityTimeline({ entries, emptyLabel }: { entries: TaskTimelineEntry[]; emptyLabel: string }) {
  if (!entries.length) {
    return <div className={styles.taskActivityEmpty}>{emptyLabel}</div>;
  }

  return (
    <ol className={styles.taskActivityList}>
      {entries.map((entry) => (
        <TaskActivityTimelineItem key={entry.id} entry={entry} />
      ))}
    </ol>
  );
}

function TaskActivityTimelineItem({ entry }: { entry: TaskTimelineEntry }) {
  return (
    <li className={classNames(styles.taskActivityItem, moduleSuffixStyle("taskActivityItem", entry.tone))}>
      <span className={styles.taskActivityMarker} aria-hidden="true" />
      <article className={styles.taskActivityContent}>
        <header className={styles.taskActivityHead}>
          <div className={styles.taskActivityTitleRow}>
            <strong>{entry.title}</strong>
            {entry.subject ? <span className={styles.taskActivitySubject}>{entry.subject}</span> : null}
          </div>
          <span>{entry.meta}</span>
        </header>
        {entry.body ? <p>{entry.body}</p> : null}
      </article>
    </li>
  );
}

function TaskMetaTag({ label, value }: Pick<TaskMetaTagItem, "label" | "value">) {
  return (
    <div className={styles.taskDetailTag}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function TaskDependencyGraph({ tasks, t }: { tasks: readonly WorkspaceTask[]; t: TranslateFn }) {
  const dependencyLevels = taskDependencyLevels(tasks);

  return (
    <section className={styles.taskDependencyGraph} aria-label={t("taskDependencyGraphLabel")}>
      <h3>{t("taskDependencyGraphLabel")}</h3>
      {dependencyLevels.length ? (
        <div className={styles.taskDependencyChain}>
          {dependencyLevels.map((level, levelIndex) => (
            <div key={`level-${levelIndex}`} className={styles.taskDependencyStage}>
              {levelIndex > 0 ? <span className={styles.taskDependencyArrow} aria-hidden="true" /> : null}
              <div className={styles.taskDependencyRow}>
                {level.map((task) => (
                  <article key={task.id} className={styles.taskDependencyCard} title={`${task.id} ${task.title}`}>
                    <strong>{task.id}</strong>
                    <span>{task.title}</span>
                  </article>
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className={styles.taskDependencyEmpty}>{t("taskBoardNoChildren")}</div>
      )}
    </section>
  );
}

function timelineGroupOrder(group: TaskTimelineGroup): number {
  const firstEntry = group.entries[0];
  return firstEntry?.order ?? Number.MAX_SAFE_INTEGER;
}

function defaultExpandedChildTaskID(groups: readonly TaskTimelineGroup[]): string {
  const activeGroups = groups.filter((group) => isActiveTaskStatus(group.task.status));
  if (!activeGroups.length) {
    return "";
  }
  return (
    activeGroups
      .slice()
      .sort(
        (left, right) =>
          latestTimelineOrder(right) - latestTimelineOrder(left) ||
          right.task.updated_at.localeCompare(left.task.updated_at),
      )[0]?.task.id ?? ""
  );
}

function latestTimelineOrder(group: TaskTimelineGroup): number {
  return group.entries.reduce((latest, entry) => Math.max(latest, entry.order), 0);
}

function isActiveTaskStatus(status: string): boolean {
  return ["pending", "assigned", "in_progress", "running", "blocked"].includes(status);
}

function taskActiveWorker(
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

function displayTaskClaimedAgentName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  return task.claimed_by_agent_name || displayAgentByID(task.claimed_by, agentNames);
}

function displayTaskWorkerName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  return displayTaskClaimedAgentName(task, agentNames) || displayTaskAssignedAgentName(task, agentNames);
}

function displayTaskAssignmentTargetName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
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

function taskDependencyLevels(tasks: readonly WorkspaceTask[]): WorkspaceTask[][] {
  const tasksByID = new Map(tasks.map((task) => [task.id, task]));
  const depthCache = new Map<string, number>();

  function depthForTask(task: WorkspaceTask, visiting = new Set<string>()): number {
    const cached = depthCache.get(task.id);
    if (cached !== undefined) {
      return cached;
    }
    if (visiting.has(task.id)) {
      return 0;
    }
    const nextVisiting = new Set(visiting);
    nextVisiting.add(task.id);
    const dependencyDepths = task.depends_on
      .map((id) => tasksByID.get(id))
      .filter((dependency): dependency is WorkspaceTask => Boolean(dependency))
      .map((dependency) => depthForTask(dependency, nextVisiting) + 1);
    const depth = dependencyDepths.length ? Math.max(...dependencyDepths) : 0;
    depthCache.set(task.id, depth);
    return depth;
  }

  const levels = new Map<number, WorkspaceTask[]>();
  for (const task of tasks) {
    const depth = depthForTask(task);
    const level = levels.get(depth) ?? [];
    level.push(task);
    levels.set(depth, level);
  }

  return Array.from(levels.entries())
    .sort(([left], [right]) => left - right)
    .map(([, level]) =>
      level.slice().sort((left, right) => left.id.localeCompare(right.id, undefined, { numeric: true })),
    );
}

function taskMetaTags(
  task: WorkspaceTask,
  childCount: number | undefined,
  t: TranslateFn,
  locale: string,
  agentNames: ReadonlyMap<string, string>,
): TaskMetaTagItem[] {
  const tags: TaskMetaTagItem[] = [];
  const addTag = (key: string, label: string, value: ReactNode) => {
    if (value === "" || value === null || value === undefined) {
      return;
    }
    tags.push({ key, label, value });
  };

  addTag("kind", t("taskKindLabel"), task.parent_id ? t("taskKindChild") : t("taskKindParent"));
  addTag("status", t("taskStatusLabel"), taskStatusLabel(task.status, t));

  if (childCount !== undefined) {
    addTag("children", t("taskChildrenLabel"), String(childCount));
  }

  const claimedBy = displayTaskClaimedAgentName(task, agentNames);
  if (task.parent_id || task.assignment_type === "agent") {
    addTag("claimed_by", t("taskClaimedByLabel"), claimedBy);
  }
  addTag("parent", t("taskParentLabel"), task.parent_id);
  const assignmentTarget = displayTaskAssignmentTargetName(task, agentNames);
  if (!claimedBy || assignmentTarget !== claimedBy) {
    addTag("assignment", t("taskAssignmentLabel"), assignmentTarget);
  }
  addTag("execution_channel", t("taskExecutionChannelLabel"), task.execution_channel);
  addTag("room", t("taskRoomLabel"), displayTaskRoomTitle(task));
  addTag("priority", t("taskPriorityLabel"), String(task.priority || 0));
  const updatedAt = formatTaskUpdatedAt(task.updated_at, locale);
  addTag("updated_at", t("taskUpdatedAtLabel"), updatedAt === "-" ? "" : updatedAt);
  addTag(
    "dispatched_at",
    t("taskDispatchedAtLabel"),
    task.dispatched_at ? formatTaskUpdatedAt(task.dispatched_at, locale) : "",
  );
  addTag("depends_on", t("taskDependsOnLabel"), task.depends_on.length ? task.depends_on.join(", ") : "");

  return tags;
}

function boardColumnsForParentTasks(tasks: readonly WorkspaceTask[]) {
  const defaultStatuses: readonly string[] = TASK_BOARD_STATUSES;
  const extraStatuses = Array.from(
    new Set(tasks.map((task) => task.status).filter((status) => !defaultStatuses.includes(status))),
  ).sort();
  return [...TASK_BOARD_STATUSES, ...extraStatuses].map((status) => ({
    status,
    tasks: tasks.filter((task) => task.status === status),
  }));
}

function taskEventsForDetail(
  task: WorkspaceTask,
  childTasks: readonly WorkspaceTask[],
  taskEvents: readonly WorkspaceTeamEvent[],
): WorkspaceTeamEvent[] {
  const relatedTaskIDs = new Set([task.id, ...childTasks.map((item) => item.id)]);
  return taskEvents
    .filter((event) => event.task_id && relatedTaskIDs.has(event.task_id))
    .slice()
    .sort((left, right) => left.seq - right.seq || left.created_at.localeCompare(right.created_at));
}

function taskTimelineEntries(
  task: WorkspaceTask,
  childTasks: readonly WorkspaceTask[],
  events: readonly WorkspaceTeamEvent[],
  t: TranslateFn,
  locale: string,
): TaskTimelineEntry[] {
  const tasksByID = new Map([task, ...childTasks].map((item) => [item.id, item]));
  const taskEventTypes = new Set(events.filter((event) => event.task_id === task.id).map((event) => event.type));
  const eventEntries = events
    .map((event) => taskTimelineEntryForEvent(event, tasksByID, t, locale))
    .filter((entry): entry is TaskTimelineEntry => Boolean(entry));
  return [...eventEntries, ...syntheticTimelineEntries(task, taskEventTypes, t, locale)].sort(
    (left, right) => left.order - right.order,
  );
}

function taskTimelineGroups(
  task: WorkspaceTask,
  childTasks: readonly WorkspaceTask[],
  events: readonly WorkspaceTeamEvent[],
  t: TranslateFn,
  locale: string,
): TaskTimelineGroup[] {
  const tasksByID = new Map([task, ...childTasks].map((item) => [item.id, item]));
  return [task, ...childTasks].map((item) => {
    const taskEventsForGroup = events.filter((event) => event.task_id === item.id);
    const taskEventTypes = new Set(taskEventsForGroup.map((event) => event.type));
    const eventEntries = taskEventsForGroup
      .map((event) => taskTimelineEntryForEvent(event, tasksByID, t, locale))
      .filter((entry): entry is TaskTimelineEntry => Boolean(entry));
    return {
      task: item,
      kind: item.id === task.id ? "parent" : "child",
      entries: [...eventEntries, ...syntheticTimelineEntries(item, taskEventTypes, t, locale)].sort(
        (left, right) => left.order - right.order,
      ),
    };
  });
}

function taskTimelineEntryForEvent(
  event: WorkspaceTeamEvent,
  tasksByID: ReadonlyMap<string, WorkspaceTask>,
  t: TranslateFn,
  locale: string,
): TaskTimelineEntry | null {
  const title = taskEventTitle(event.type, t);
  if (!title) {
    return null;
  }
  const subjectTask = tasksByID.get(event.task_id);
  const subject = event.task_id ? `${event.task_id}${subjectTask?.title ? ` · ${subjectTask.title}` : ""}` : "";
  return {
    id: `event-${event.seq}-${event.type}-${event.task_id}`,
    title,
    subject,
    meta: taskEventMeta(event, locale),
    body: taskEventBody(event, t),
    tone: taskEventTone(event.type),
    order: event.seq || Number.MAX_SAFE_INTEGER,
  };
}

function syntheticTimelineEntries(
  task: WorkspaceTask,
  existingEventTypes: ReadonlySet<string>,
  t: TranslateFn,
  locale: string,
): TaskTimelineEntry[] {
  const entries: TaskTimelineEntry[] = [];
  const syntheticOrder = () => Number.MAX_SAFE_INTEGER - 100 + entries.length;
  if (displayTaskAssignedAgent(task) && !existingEventTypes.has("task.assigned")) {
    const assignee = displayTaskAssignedAgent(task);
    entries.push({
      id: `synthetic-assigned-${task.id}`,
      title: t("taskTimelineAssigned"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.updated_at, locale),
      body: `${t("taskActivityTargetLabel")}: ${assignee}`,
      order: syntheticOrder(),
    });
  }
  if (displayTaskClaimedAgent(task) && !existingEventTypes.has("task.claimed")) {
    const claimedBy = displayTaskClaimedAgent(task);
    entries.push({
      id: `synthetic-claimed-${task.id}`,
      title: t("taskTimelineClaimed"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.updated_at, locale),
      body: `${t("taskActivityTargetLabel")}: ${claimedBy}`,
      order: syntheticOrder(),
    });
  }
  if (task.plan_summary && !existingEventTypes.has("task.planned")) {
    entries.push({
      id: `synthetic-plan-${task.id}`,
      title: t("taskTimelinePlanned"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.updated_at, locale),
      body: task.plan_summary,
      order: syntheticOrder(),
    });
  }
  if (task.dispatched_at && !existingEventTypes.has("task.dispatched")) {
    const assignee = displayTaskAssignedAgent(task);
    entries.push({
      id: `synthetic-dispatched-${task.id}`,
      title: t("taskTimelineDispatched"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.dispatched_at, locale),
      body: assignee ? `${t("taskActivityTargetLabel")}: ${assignee}` : "",
      order: syntheticOrder(),
    });
  }
  if (task.result && !existingEventTypes.has("task.completed")) {
    entries.push({
      id: `synthetic-result-${task.id}`,
      title: t("taskTimelineCompleted"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.updated_at, locale),
      body: task.result,
      tone: "success",
      order: syntheticOrder(),
    });
  }
  if (task.error && !existingEventTypes.has("task.failed") && !existingEventTypes.has("task.blocked")) {
    entries.push({
      id: `synthetic-error-${task.id}`,
      title: task.status === "failed" ? t("taskTimelineFailed") : t("taskTimelineBlocked"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.updated_at, locale),
      body: task.error,
      tone: task.status === "failed" ? "danger" : "warning",
      order: syntheticOrder(),
    });
  }
  return entries;
}

function taskEventTitle(type: string, t: TranslateFn): string {
  switch (type) {
    case "task.created":
      return t("taskTimelineCreated");
    case "task.planned":
      return t("taskTimelinePlanned");
    case "task.execution_room":
      return t("taskTimelineExecutionRoom");
    case "task.started":
      return t("taskTimelineStarted");
    case "task.dispatched":
      return t("taskTimelineDispatched");
    case "task.assigned":
      return t("taskTimelineAssigned");
    case "task.claimed":
      return t("taskTimelineClaimed");
    case "task.blocked":
      return t("taskTimelineBlocked");
    case "task.completed":
      return t("taskTimelineCompleted");
    case "task.failed":
      return t("taskTimelineFailed");
    case "task.cancelled":
      return t("taskTimelineCancelled");
    case "presence.updated":
    case "presence.changed":
      return t("taskTimelinePresence");
    case "approval.requested":
    case "approval.resolved":
      return t("taskTimelineApproval");
    default:
      return t("taskTimelineUpdated");
  }
}

function taskEventMeta(event: WorkspaceTeamEvent, locale: string): string {
  return [formatTaskUpdatedAt(event.created_at, locale), event.actor_agent_name].filter(Boolean).join(" · ");
}

function taskEventBody(event: WorkspaceTeamEvent, t: TranslateFn): string {
  const lines: string[] = [];
  if (event.summary) {
    lines.push(event.summary);
  }
  const target = event.target_agent_name;
  if (target) {
    lines.push(`${t("taskActivityTargetLabel")}: ${target}`);
  }
  return lines.join("\n");
}

function taskEventTone(type: string): TaskTimelineEntry["tone"] {
  if (type === "task.completed") {
    return "success";
  }
  if (type === "task.blocked") {
    return "warning";
  }
  if (type === "task.failed" || type === "task.cancelled") {
    return "danger";
  }
  return undefined;
}
