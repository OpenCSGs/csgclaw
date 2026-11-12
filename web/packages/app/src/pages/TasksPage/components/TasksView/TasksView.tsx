import { useEffect, useMemo, useState } from "react";
import type { ComponentProps } from "react";
import { CalendarClock, Pencil, Play, Trash2 } from "lucide-react";
import {
  InlineError,
  Button,
  Tooltip,
  type ButtonVariant,
  DialogBody,
  DialogContent,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { ScheduledTaskStart } from "../ScheduledTaskStart";
import type { CreateWorkspaceTaskPayload } from "@/api/tasks";
import type { CreateScheduledTaskPayload, UpdateScheduledTaskPayload } from "@/api/scheduledTasks";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import {
  scheduledTaskRecurrenceLabel,
  scheduledTaskExecutors,
  type WorkspaceScheduledTask,
  type WorkspaceScheduledTaskRun,
} from "@/models/scheduledTasks";
import { displayTeam, formatTaskUpdatedAt, rootTaskForTask, taskChildren } from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTeam, WorkspaceTeamEvent } from "@/models/tasks";
import { agentNameLookup, displayAgentByID } from "@/models/taskBoard";
import { classNames } from "@/shared/lib/classNames";
import { ScheduledTaskCreateDialog } from "./ScheduledTaskCreateDialog";
import { ScheduledTaskFormFields } from "./ScheduledTaskFormFields";
import { TaskCreateDialog } from "./TaskCreateDialog";
import {
  TASK_TITLE_MAX_LENGTH,
  type ScheduledTaskFormDraft,
  type ScheduledTaskFormFieldErrors,
  type TaskCreateDraft,
  type TaskCreateFieldErrors,
} from "./taskDialogTypes";
import styles from "./TasksView.module.css";
import taskFormStyles from "./TaskForm.module.css";
import scheduledTasksStyles from "./ScheduledTasks.module.css";
import taskDetailsStyles from "./TaskDetails/TaskDetails.module.css";
import { GeneratedTaskInlineDetail, TaskDetailDialog } from "./TaskDetails";

const EMPTY_AGENTS: AgentLike[] = [];

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
  return scheduledTaskExecutors(agents).map((agent) => ({
    value: String(agent.id || ""),
    textValue: displayAgent(agent),
    label: (
      <span className={taskFormStyles.executorOption}>
        <span className={taskFormStyles.executorAvatar} aria-hidden="true">
          <AgentAvatarContent avatar={agent.avatar} fallback={displayAgent(agent).slice(0, 1)} />
        </span>
        {displayAgent(agent)}
      </span>
    ),
    description: String(agent.id || ""),
  }));
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
  onSendTextToManager?: (text: string) => Promise<boolean>;
  scheduledTextBusy?: boolean;
  scheduledTextError?: string;
  agents?: AgentLike[];
  createTaskBusy?: boolean;
  createTaskError?: string;
  error?: string;
  loading?: boolean;
  onCloseCreateScheduledTaskModal?: () => void;
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
  showCreateScheduledTaskModal?: boolean;
  editingScheduledTaskID?: string;
  startTaskBusy?: boolean;
  startingTaskID?: string;
  taskActionError?: string;
  taskEvents?: WorkspaceTeamEvent[];
  tasks?: WorkspaceTask[];
  activeView?: TaskBoardView;
  t?: TranslateFn;
  teams?: WorkspaceTeam[];
  scheduledTasks?: WorkspaceScheduledTask[];
  scheduledTaskRuns?: WorkspaceScheduledTaskRun[];
  allScheduledTaskRuns?: WorkspaceScheduledTaskRun[];
  createScheduledTaskBusy?: boolean;
  createScheduledTaskError?: string;
  editScheduledTaskBusy?: boolean;
  editScheduledTaskError?: string;
  scheduledTaskActionID?: string;
  scheduledTaskActionError?: string;
};

export function TasksView({
  onSendTextToManager,
  scheduledTextBusy = false,
  scheduledTextError = "",
  t = (key) => key,
  agents = EMPTY_AGENTS,
  tasks = [],
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
  showCreateScheduledTaskModal = false,
  activeView = "scheduled",
  editingScheduledTaskID = "",
  scheduledTasks = [],
  scheduledTaskRuns = [],
  allScheduledTaskRuns = scheduledTaskRuns,
  selectedScheduledTaskID = "",
  scheduledTaskActionID = "",
  scheduledTaskActionError = "",
  selectedTask = null,
  parentDetailTaskID = "",
  rooms,
  onCloseCreateScheduledTaskModal,
  onCloseCreateTaskModal,
  onCloseEditScheduledTaskModal,
  onCloseParentTaskDetail,
  onCloseTaskDetails,
  onCreateTask,
  onCreateScheduledTask,
  onDeleteScheduledTask,
  onEditScheduledTask,
  onOpenCreateScheduledTaskModal,
  onRefresh = () => {},
  onRunScheduledTask = () => {},
  onSelectScheduledTask = () => {},
  onSelectTaskBoardView = () => {},
  onToggleScheduledTask = () => {},
  onOpenEditScheduledTaskModal,
  onOpenConversation = () => {},
}: TasksViewProps) {
  const [showScheduledStart, setShowScheduledStart] = useState(false);
  const [scheduledTextFocus, setScheduledTextFocus] = useState(0);
  useEffect(() => {
    if (selectedScheduledTaskID) setShowScheduledStart(false);
  }, [selectedScheduledTaskID]);
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
  const activeScheduledTaskIDs = useMemo(() => {
    const activeIDs = new Set<string>();
    const tasksByID = new Map(tasks.map((task) => [task.id, task]));
    for (const run of allScheduledTaskRuns) {
      if (!run.task_id || run.status === "failed") {
        continue;
      }
      const task = tasksByID.get(run.task_id);
      if (!task || !isTerminalWorkspaceTaskStatus(task.status)) {
        activeIDs.add(run.scheduled_task_id);
      }
    }
    return activeIDs;
  }, [allScheduledTaskRuns, tasks]);
  const hasActiveGeneratedTask = Boolean(selectedScheduledTask && activeScheduledTaskIDs.has(selectedScheduledTask.id));
  const parentDetailTask = useMemo(() => {
    const task = parentDetailTaskID ? (tasks.find((item) => item.id === parentDetailTaskID) ?? null) : selectedTask;
    return rootTaskForTask(tasks, task) ?? task;
  }, [parentDetailTaskID, selectedTask, tasks]);
  const parentDialogTask = parentDetailTask;
  const parentDialogChildTasks = useMemo(
    () => (parentDialogTask ? taskChildren(tasks, parentDialogTask.id) : []),
    [parentDialogTask, tasks],
  );
  const [createDraft, setCreateDraft] = useState<TaskCreateDraft>(emptyCreateDraft);
  const [createFieldErrors, setCreateFieldErrors] = useState<TaskCreateFieldErrors>({});
  const [scheduledDraft, setScheduledDraft] = useState<ScheduledTaskFormDraft>(emptyScheduledTaskDraft);
  const [scheduledFieldErrors, setScheduledFieldErrors] = useState<ScheduledTaskFormFieldErrors>({});
  const [editScheduledDraft, setEditScheduledDraft] = useState<ScheduledTaskFormDraft>(emptyScheduledTaskDraft);
  const [editScheduledFieldErrors, setEditScheduledFieldErrors] = useState<ScheduledTaskFormFieldErrors>({});
  const [deletingScheduledTask, setDeletingScheduledTask] = useState<WorkspaceScheduledTask | null>(null);
  const latestRunTaskID = scheduledTaskRuns[0]?.task_id;
  const latestRunTask = latestRunTaskID ? tasks.find((item) => item.id === latestRunTaskID) : undefined;
  const latestRunOutcome = latestRunTask?.result || latestRunTask?.error;
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
    setConversationOpenError("");
  }, [activeView, selectedScheduledTask?.id]);

  useEffect(() => {
    if (!showCreateTaskModal) {
      return;
    }
    setCreateDraft(emptyCreateDraft);
    setCreateFieldErrors({});
  }, [showCreateTaskModal]);

  useEffect(() => {
    if (!showCreateScheduledTaskModal) {
      return;
    }
    const now = new Date();
    setScheduledDraft({
      ...emptyScheduledTaskDraft,
      date: todayInputValue(now),
      time: timeInputValue(now),
    });
    setScheduledFieldErrors({});
  }, [showCreateScheduledTaskModal]);

  useEffect(() => {
    if (!editingScheduledTask) {
      return;
    }
    setEditScheduledDraft(scheduledTaskDraftFromTask(editingScheduledTask));
    setEditScheduledFieldErrors({});
  }, [editingScheduledTask]);

  useEffect(() => {
    setSelectedGeneratedTaskID("");
  }, [selectedScheduledTask?.id]);

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

  function closeRootTaskDetail() {
    setConversationOpenError("");
    onCloseParentTaskDetail?.();
    void onCloseTaskDetails?.();
  }

  function selectScheduledTask(taskID: string) {
    setConversationOpenError("");
    setShowScheduledStart(false);
    onSelectScheduledTask(taskID);
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
      setConversationOpenError(t("taskConversationUnavailable"));
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
    <section className={classNames("tasks-pane", styles.tasksPane)}>
      {error ? <InlineError>{error}</InlineError> : null}
      {taskActionError ? <InlineError className={styles.tasksActionError}>{taskActionError}</InlineError> : null}
      {scheduledTaskActionError ? (
        <InlineError className={styles.tasksActionError}>{scheduledTaskActionError}</InlineError>
      ) : null}
      {conversationOpenError ? (
        <InlineError className={styles.tasksActionError}>{conversationOpenError}</InlineError>
      ) : null}
      {!error && activeView === "scheduled" ? (
        <div className={styles.tasksBoardWorkbench} aria-busy={loading}>
          <div className={styles.tasksBoardPanel}>
            <div className={classNames(styles.headerRow, styles.justifyEnd, styles.tasksBoardHead)}>
              <div className={styles.tasksBoardHeading}>
                <h1>{t("scheduledTasksTab")}</h1>
              </div>
              <TaskActionStrip
                t={t}
                showConversation={false}
                showParentDetail={false}
                canPlanTask={false}
                canStartTask={false}
                planTaskBusy={planTaskBusy}
                startTaskBusy={startTaskBusy}
                onCreateScheduledTask={onOpenCreateScheduledTaskModal}
                onCreateScheduledTaskText={() => {
                  setShowScheduledStart(true);
                  setScheduledTextFocus((value) => value + 1);
                }}
                onRefresh={onRefresh}
              />
            </div>
            <div className={styles.taskContentLayout}>
              <div className={styles.taskContentMain}>
                <div className={scheduledTasksStyles.scheduledTaskLayout}>
                  <section className={scheduledTasksStyles.scheduledTaskList} aria-label={t("scheduledTasksTab")}>
                    {scheduledTasks.length ? (
                      scheduledTasks.map((item) => {
                        const status = !item.enabled
                          ? "disabled"
                          : activeScheduledTaskIDs.has(item.id)
                            ? "running"
                            : "enabled";
                        const active = !showScheduledStart && selectedScheduledTask?.id === item.id;
                        return (
                          <button
                            key={item.id}
                            type="button"
                            className={scheduledTasksStyles.scheduledTaskRow}
                            data-active={active || undefined}
                            aria-pressed={active}
                            onClick={() => selectScheduledTask(item.id)}
                          >
                            <span className={scheduledTasksStyles.scheduledTaskRowHeading}>
                              <CalendarClock size={15} aria-hidden="true" />
                              <strong>{item.title}</strong>
                              <span className={scheduledTasksStyles.scheduledTaskStatus} data-status={status}>
                                {t(
                                  status === "disabled"
                                    ? "scheduledTaskStatusDisabled"
                                    : status === "running"
                                      ? "scheduledTaskStatusRunning"
                                      : "scheduledTaskStatusEnabled",
                                )}
                              </span>
                            </span>
                            <small>
                              {scheduledTaskRecurrenceLabel(item.recurrence, t)} ·{" "}
                              {formatTaskUpdatedAt(item.next_run_at)}
                            </small>
                          </button>
                        );
                      })
                    ) : (
                      <div className={styles.taskBoardEmpty}>{t("scheduledTasksEmpty")}</div>
                    )}
                  </section>
                  <section
                    className={scheduledTasksStyles.scheduledTaskDetail}
                    data-start={showScheduledStart || !selectedScheduledTask || undefined}
                    aria-label={t("scheduledTaskDetailTitle")}
                  >
                    {selectedScheduledTask && !showScheduledStart ? (
                      <>
                        <div className={scheduledTasksStyles.scheduledTaskDetailHead}>
                          <div>
                            <h2>{selectedScheduledTask.title}</h2>
                            <p>{scheduledTaskRecurrenceLabel(selectedScheduledTask.recurrence, t)}</p>
                          </div>
                          <div className={scheduledTasksStyles.scheduledTaskActions}>
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
                                disabled={
                                  scheduledTaskActionID === selectedScheduledTask.id ||
                                  Boolean(selectedScheduledTask.agent_state)
                                }
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
                              disabled={
                                scheduledTaskActionID === selectedScheduledTask.id ||
                                hasActiveGeneratedTask ||
                                Boolean(selectedScheduledTask.agent_state)
                              }
                              onClick={() => onRunScheduledTask(selectedScheduledTask.id)}
                            >
                              <Play size={14} aria-hidden="true" />
                              {hasActiveGeneratedTask ? t("scheduledTaskActiveTask") : t("scheduledTaskRunNow")}
                            </Button>
                          </div>
                        </div>
                        <dl className={scheduledTasksStyles.scheduledTaskMeta}>
                          <div>
                            <dt>{t("scheduledTaskAgentLabel")}</dt>
                            <Tooltip content={selectedScheduledTask.agent_id}>
                              <dd>
                                {displayAgentByID(
                                  selectedScheduledTask.agent_id,
                                  agentNames,
                                  selectedScheduledTask.agent_name,
                                )}
                                {selectedScheduledTask.agent_state && (
                                  <span>
                                    {" "}
                                    ·{" "}
                                    {t(
                                      selectedScheduledTask.agent_state === "archived"
                                        ? "agentStateArchived"
                                        : "agentStateDeleted",
                                    )}
                                  </span>
                                )}
                              </dd>
                            </Tooltip>
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
                        <div className={scheduledTasksStyles.scheduledTaskPrompt}>{selectedScheduledTask.prompt}</div>
                        {!selectedGeneratedTaskID && latestRunTask?.assignment_type === "agent" && latestRunOutcome ? (
                          <section
                            className={classNames(
                              scheduledTasksStyles.scheduledTaskRuns,
                              taskDetailsStyles.generatedTaskDescription,
                            )}
                          >
                            <h3>{t("scheduledTaskLatestResult")}</h3>
                            <p>{latestRunOutcome}</p>
                          </section>
                        ) : null}
                        <div className={scheduledTasksStyles.scheduledTaskRuns}>
                          <h3>{t("scheduledTaskRunsTitle")}</h3>
                          {scheduledTaskRuns.length ? (
                            <div className={scheduledTasksStyles.scheduledTaskRunList}>
                              {scheduledTaskRuns.map((run) => (
                                <div key={run.id} className={scheduledTasksStyles.scheduledTaskRunRow}>
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
                                    <Tooltip content={run.error}>
                                      <small>{scheduledTaskRunErrorLabel(run.error, t)}</small>
                                    </Tooltip>
                                  ) : null}
                                </div>
                              ))}
                            </div>
                          ) : (
                            <div className={styles.taskBoardEmpty}>{t("scheduledTaskRunsEmpty")}</div>
                          )}
                        </div>
                        {selectedGeneratedTaskID ? (
                          <GeneratedTaskInlineDetail
                            task={selectedGeneratedTask}
                            childTasks={selectedGeneratedChildTasks}
                            agentNames={agentNames}
                            teams={teams}
                            taskEvents={taskEvents}
                            t={t}
                            onOpenConversation={openTaskConversation}
                          />
                        ) : null}
                      </>
                    ) : (
                      <ScheduledTaskStart
                        agents={agents}
                        t={t}
                        busy={scheduledTextBusy}
                        error={scheduledTextError}
                        focusToken={scheduledTextFocus}
                        onSend={onSendTextToManager}
                      />
                    )}
                  </section>
                </div>
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
      <TaskCreateDialog
        assignmentOptions={assignmentOptions}
        busy={createTaskBusy}
        draft={createDraft}
        error={createTaskError}
        errors={createFieldErrors}
        open={showCreateTaskModal}
        t={t}
        onChange={setCreateDraft}
        onClearError={clearCreateFieldError}
        onClose={onCloseCreateTaskModal}
        onSubmit={submitCreateTask}
      />
      <ScheduledTaskCreateDialog
        busy={createScheduledTaskBusy}
        draft={scheduledDraft}
        error={createScheduledTaskError}
        errors={scheduledFieldErrors}
        open={showCreateScheduledTaskModal}
        scheduledAgentOptions={scheduledAgentOptions}
        t={t}
        onChange={setScheduledDraft}
        onClearError={clearScheduledFieldError}
        onClose={onCloseCreateScheduledTaskModal}
        onSubmit={submitCreateScheduledTask}
      />
      <DialogRoot
        open={Boolean(editingScheduledTask)}
        onOpenChange={(open) => (!open ? onCloseEditScheduledTaskModal?.() : null)}
      >
        <DialogContent className={taskFormStyles.taskCreateDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("scheduledTaskEditTitle")}</DialogTitle>
              <DialogDescription>{t("scheduledTaskEditSubtitle")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
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
              <InlineError className={taskFormStyles.taskCreateError}>{editScheduledTaskError}</InlineError>
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
        <DialogContent className={taskFormStyles.taskCreateDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("scheduledTaskDelete")}</DialogTitle>
              <DialogDescription>
                {t("scheduledTaskDeleteConfirmMessage", { title: deletingScheduledTask?.title || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
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

type TaskActionStripProps = {
  canPlanTask: boolean;
  canStartTask: boolean;
  conversationLabel?: string;
  conversationShortLabel?: string;
  onCreateScheduledTask?: () => VoidOrPromise;
  onCreateScheduledTaskText?: () => VoidOrPromise;
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
  onCreateScheduledTaskText = undefined,
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
      <TaskToolbarButton
        label={t("tasksRefreshShort")}
        title={t("tasksRefresh")}
        tooltipContentProps={{ side: "bottom", sideOffset: 6 }}
        onClick={onRefresh}
      />
      {onCreateTask ? (
        <TaskToolbarButton
          label={t("taskCreate")}
          title={t("taskCreate")}
          variant="primary"
          tooltipContentProps={{ side: "bottom", sideOffset: 6 }}
          onClick={onCreateTask}
        />
      ) : null}
      {onCreateScheduledTaskText ? (
        <TaskToolbarButton label={t("scheduledTaskTextCreate")} onClick={onCreateScheduledTaskText} />
      ) : null}
      {onCreateScheduledTask ? (
        <TaskToolbarButton
          label={t("scheduledTaskCreate")}
          title={t("scheduledTaskCreate")}
          variant="primary"
          tooltipContentProps={{ side: "bottom", sideOffset: 6 }}
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
  tooltipContentProps?: ComponentProps<typeof Tooltip>["contentProps"];
  title?: string;
  variant?: ButtonVariant;
} & ComponentProps<typeof Button>;

function TaskToolbarButton({
  label,
  tooltipContentProps = { side: "bottom", sideOffset: 6 },
  title = label,
  variant = "secondaryGray",
  ...props
}: TaskToolbarButtonProps) {
  return (
    <Tooltip content={title} contentProps={tooltipContentProps}>
      <span>
        <Button
          className={classNames(
            styles.taskToolbarButton,
            variant === "secondaryGray" && styles.taskToolbarButtonSecondary,
          )}
          aria-label={title}
          size="sm"
          variant={variant}
          {...props}
        >
          {label}
        </Button>
      </span>
    </Tooltip>
  );
}
