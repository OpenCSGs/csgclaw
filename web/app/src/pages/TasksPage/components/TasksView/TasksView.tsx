import { useEffect, useMemo, useState } from "react";
import type { ComponentProps, ReactNode } from "react";
import { X } from "lucide-react";
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
import { TaskStatusPill } from "@/components/business";
import type { CreateWorkspaceTaskPayload } from "@/api/tasks";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import {
  boardColumnsForTask,
  displayTaskRoom,
  displayTaskTeam,
  displayTeam,
  formatTaskUpdatedAt,
  rootTaskForTask,
  rootTasks,
  taskChildren,
  taskExecutionRoomID,
  taskStatusLabel,
  taskUsesExecutionRoom,
} from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTeam, WorkspaceTeamEvent } from "@/models/tasks";
import { MANAGER_AGENT_ID } from "@/shared/constants/agents";
import "./TasksView.css";

const TASK_TITLE_MAX_LENGTH = 80;

type TaskCreateDraft = {
  team_id: string;
  title: string;
  description: string;
};

const emptyCreateDraft: TaskCreateDraft = {
  team_id: "",
  title: "",
  description: "",
};

type VoidOrPromise = void | Promise<void>;

export type TasksViewProps = {
  agents?: AgentLike[];
  createTaskBusy?: boolean;
  createTaskError?: string;
  error?: string;
  loading?: boolean;
  onCloseCreateTaskModal?: () => void;
  onCloseParentTaskDetail?: () => void;
  onCreateTask?: (payload: CreateWorkspaceTaskPayload) => VoidOrPromise;
  onOpenConversation?: (roomID: string) => VoidOrPromise;
  onPlanTask?: (taskID: string) => VoidOrPromise;
  onRefresh?: () => VoidOrPromise;
  onStartTask?: (taskID: string) => VoidOrPromise;
  onViewParentDetail?: (taskID: string) => VoidOrPromise;
  parentDetailTaskID?: string;
  planTaskBusy?: boolean;
  selectedTask?: WorkspaceTask | null;
  showCreateTaskModal?: boolean;
  startTaskBusy?: boolean;
  taskActionError?: string;
  taskEvents?: WorkspaceTeamEvent[];
  tasks?: WorkspaceTask[];
  t?: TranslateFn;
  teams?: WorkspaceTeam[];
};

export function TasksView({
  t = (key) => key,
  agents = [],
  tasks = [],
  taskEvents = [],
  teams = [],
  selectedTask,
  loading = false,
  error = "",
  taskActionError = "",
  planTaskBusy = false,
  startTaskBusy = false,
  createTaskBusy = false,
  createTaskError = "",
  showCreateTaskModal = false,
  parentDetailTaskID = "",
  onCloseCreateTaskModal,
  onCloseParentTaskDetail,
  onCreateTask,
  onPlanTask,
  onStartTask,
  onRefresh = () => {},
  onOpenConversation = () => {},
  onViewParentDetail,
}: TasksViewProps) {
  const parentTasks = useMemo(() => rootTasks(tasks), [tasks]);
  const routedRootTask = useMemo(() => rootTaskForTask(tasks, selectedTask), [selectedTask, tasks]);
  const activeRootTask = routedRootTask ?? parentTasks[0] ?? null;
  const childTasks = useMemo(
    () => (activeRootTask ? taskChildren(tasks, activeRootTask.id) : []),
    [activeRootTask, tasks],
  );
  const columns = useMemo(
    () => (activeRootTask ? boardColumnsForTask(tasks, activeRootTask.id) : []),
    [activeRootTask, tasks],
  );
  const managerIDs = useMemo(() => {
    const idSet = new Set<string>([MANAGER_AGENT_ID]);
    for (const agent of agents) {
      if (agent.role === "manager" && agent.id) {
        idSet.add(agent.id);
      }
    }
    return idSet;
  }, [agents]);
  const isUnstarted = (status: string) => status === "" || status === "pending";
  const isPlannableTask = (task: WorkspaceTask) =>
    isUnstarted(task.status) || (task.status === "assigned" && managerIDs.has(task.assigned_to));
  const canPlanRootTask = Boolean(activeRootTask && isPlannableTask(activeRootTask) && childTasks.length === 0);
  const canStartRootTask = Boolean(activeRootTask && isUnstarted(activeRootTask.status) && childTasks.length > 0);
  const activeRootExecutionRoomID = useMemo(
    () => (activeRootTask ? taskExecutionRoomID(activeRootTask, childTasks, teams) : ""),
    [activeRootTask, childTasks, teams],
  );
  const activeRootUsesExecutionRoom = useMemo(
    () => (activeRootTask ? taskUsesExecutionRoom(activeRootTask, teams, childTasks) : false),
    [activeRootTask, childTasks, teams],
  );
  const [createDraft, setCreateDraft] = useState<TaskCreateDraft>(emptyCreateDraft);
  const [childDetailTaskID, setChildDetailTaskID] = useState("");
  const childDetailTask = useMemo(
    () => (childDetailTaskID ? (tasks.find((item) => item.id === childDetailTaskID) ?? null) : null),
    [childDetailTaskID, tasks],
  );
  const parentDetailTask = useMemo(() => {
    if (!parentDetailTaskID) {
      return null;
    }
    const task = tasks.find((item) => item.id === parentDetailTaskID) ?? null;
    return rootTaskForTask(tasks, task) ?? task;
  }, [parentDetailTaskID, tasks]);
  const parentDetailChildTasks = useMemo(
    () => (parentDetailTask ? taskChildren(tasks, parentDetailTask.id) : []),
    [parentDetailTask, tasks],
  );

  useEffect(() => {
    if (!showCreateTaskModal) {
      return;
    }
    setCreateDraft({
      ...emptyCreateDraft,
      team_id: teams[0]?.id || "",
    });
  }, [showCreateTaskModal, teams]);

  useEffect(() => {
    setChildDetailTaskID("");
  }, [activeRootTask?.id]);

  useEffect(() => {
    if (!selectedTask?.parent_id) {
      return;
    }
    setChildDetailTaskID(selectedTask.id);
  }, [selectedTask]);

  async function submitCreateTask() {
    const title = createDraft.title.trim();
    const description = createDraft.description.trim();
    if (!createDraft.team_id || !title || !description) {
      return;
    }
    await onCreateTask?.({
      team_id: createDraft.team_id,
      title,
      body: description,
    });
  }

  return (
    <section className="entity-pane tasks-pane">
      {!parentTasks.length ? (
        <header className="tasks-page-header">
          <div className="tasks-board-heading">
            <h1>{t("subTaskBoardTitle")}</h1>
          </div>
          <TaskActionStrip
            t={t}
            showConversation={false}
            canPlanTask={false}
            canStartTask={false}
            planTaskBusy={false}
            startTaskBusy={false}
            onOpenConversation={undefined}
            onRefresh={onRefresh}
          />
        </header>
      ) : null}
      {error ? <div className="form-error">{error}</div> : null}
      {taskActionError ? <div className="form-error tasks-action-error">{taskActionError}</div> : null}
      {!loading && !error && parentTasks.length === 0 ? (
        <div className="empty-state shell-empty-state">
          <strong>{t("tasksEmpty")}</strong>
          <span>{t("tasksEmptyHint")}</span>
        </div>
      ) : null}
      {loading && !tasks.length ? <div className="workspace-empty">{t("tasksLoading")}</div> : null}
      {!loading && !error && parentTasks.length ? (
        <div className="tasks-board-workbench">
          <div className="tasks-board-panel">
            <div className="tasks-board-head">
              <div className="tasks-board-heading">
                <h1>{t("subTaskBoardTitle")}</h1>
              </div>
              <TaskActionStrip
                t={t}
                showConversation={Boolean(activeRootTask)}
                showParentDetail={Boolean(activeRootTask)}
                canPlanTask={canPlanRootTask}
                canStartTask={canStartRootTask}
                planTaskBusy={planTaskBusy}
                startTaskBusy={startTaskBusy}
                conversationLabel={
                  activeRootTask
                    ? activeRootUsesExecutionRoom
                      ? t("taskOpenExecutionRoom")
                      : t("taskOpenConversation")
                    : t("taskOpenConversation")
                }
                conversationShortLabel={
                  activeRootTask && activeRootUsesExecutionRoom
                    ? t("taskOpenExecutionRoomShort")
                    : t("taskOpenConversationShort")
                }
                onOpenConversation={() =>
                  activeRootExecutionRoomID ? onOpenConversation(activeRootExecutionRoomID) : undefined
                }
                onViewParentDetail={() => activeRootTask && onViewParentDetail?.(activeRootTask.id)}
                onPlanTask={() => activeRootTask && onPlanTask?.(activeRootTask.id)}
                onStartTask={() => activeRootTask && onStartTask?.(activeRootTask.id)}
                onRefresh={onRefresh}
              />
            </div>
            {activeRootTask ? (
              <p className="tasks-room-hint" role="note">
                {activeRootUsesExecutionRoom
                  ? t("taskRoomHintActive", { taskId: activeRootTask.id })
                  : t("taskRoomHintPending", { taskId: activeRootTask.id })}
              </p>
            ) : null}
            <div className="tasks-kanban-scroll" role="region" aria-label={t("subTaskBoardTitle")}>
              <div className="tasks-kanban">
                {columns.map((column) => (
                  <section key={column.status} className={`task-board-column task-board-column-${column.status}`}>
                    <header className="task-board-column-head">
                      <span>{taskStatusLabel(column.status, t)}</span>
                      <strong>{column.tasks.length}</strong>
                    </header>
                    <div className="task-board-column-body">
                      {column.tasks.length ? (
                        column.tasks.map((task) => (
                          <TaskBoardCard
                            key={task.id}
                            task={task}
                            t={t}
                            onSelect={() => setChildDetailTaskID(task.id)}
                          />
                        ))
                      ) : (
                        <div className="task-board-empty">{t("taskBoardColumnEmpty")}</div>
                      )}
                    </div>
                  </section>
                ))}
              </div>
            </div>
          </div>
        </div>
      ) : null}
      <TaskDetailDialog
        t={t}
        task={childDetailTask}
        teams={teams}
        taskEvents={taskEvents}
        open={Boolean(childDetailTask?.parent_id)}
        onClose={() => setChildDetailTaskID("")}
        onOpenConversation={onOpenConversation}
      />
      <TaskDetailDialog
        t={t}
        title={t("taskParentDetailTitle")}
        task={parentDetailTask}
        childCount={parentDetailChildTasks.length}
        childTasks={parentDetailChildTasks}
        teams={teams}
        taskEvents={taskEvents}
        open={Boolean(parentDetailTask)}
        onClose={onCloseParentTaskDetail}
        onOpenConversation={onOpenConversation}
      />
      <DialogRoot open={showCreateTaskModal} onOpenChange={(open) => (!open ? onCloseCreateTaskModal?.() : null)}>
        <DialogContent className="task-create-dialog">
          <DialogHeader>
            <div>
              <DialogTitle>{t("taskCreateTitle")}</DialogTitle>
              <DialogDescription>{t("taskCreateSubtitle")}</DialogDescription>
            </div>
            <TaskDialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <div className="task-create-form task-create-form-compact">
              <label className="field">
                <span>{t("taskTitleLabel")}</span>
                <input
                  value={createDraft.title}
                  maxLength={TASK_TITLE_MAX_LENGTH}
                  onInput={(event) => setCreateDraft((current) => ({ ...current, title: event.currentTarget.value }))}
                  placeholder={t("taskTitlePlaceholder")}
                />
              </label>
              <label className="field">
                <span>{t("taskDescriptionLabel")}</span>
                <textarea
                  value={createDraft.description}
                  onInput={(event) =>
                    setCreateDraft((current) => ({ ...current, description: event.currentTarget.value }))
                  }
                  placeholder={t("taskDescriptionPlaceholder")}
                />
              </label>
              <label className="field">
                <span>{t("taskTeamLabel")}</span>
                <Select
                  value={createDraft.team_id}
                  onValueChange={(teamID) => setCreateDraft((current) => ({ ...current, team_id: teamID }))}
                  triggerProps={{ "aria-label": t("taskTeamLabel") }}
                  options={teams.map((team) => ({ value: team.id, label: displayTeam(team) }))}
                />
              </label>
            </div>
            {createTaskError ? <div className="form-error task-create-error">{createTaskError}</div> : null}
          </DialogBody>
          <DialogFooter>
            <Button variant="secondaryGray" size="md" onClick={onCloseCreateTaskModal}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={createTaskBusy}
              loadingLabel={t("taskCreating")}
              disabled={
                createTaskBusy || !createDraft.team_id || !createDraft.title.trim() || !createDraft.description.trim()
              }
              onClick={submitCreateTask}
            >
              {t("taskCreateSubmit")}
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
  onViewParentDetail = undefined,
  onPlanTask = undefined,
  onStartTask = undefined,
  onRefresh,
}: TaskActionStripProps) {
  return (
    <div className="tasks-toolbar" aria-label={t("tasksActionsLabel")}>
      <TaskToolbarButton label={t("tasksRefreshShort")} title={t("tasksRefresh")} onClick={onRefresh} />
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
    <Button className="task-toolbar-button" aria-label={title} title={title} size="sm" variant={variant} {...props}>
      {label}
    </Button>
  );
}

type TaskBoardCardProps = {
  onSelect: () => void;
  t: TranslateFn;
  task: WorkspaceTask;
};

function TaskBoardCard({ task, t, onSelect }: TaskBoardCardProps) {
  const description = task.body || task.plan_summary || task.result || task.error || t("tasksDetailPlaceholder");

  return (
    <button type="button" className="task-board-card" onClick={onSelect} title={`${task.id} ${task.title}`}>
      <span className="task-board-card-id">{task.id}</span>
      <strong className="task-board-card-title">{task.title}</strong>
      <span className="task-board-card-description">{description}</span>
    </button>
  );
}

type TaskDetailDialogProps = {
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

function TaskDetailDialog({
  t,
  title = "",
  task,
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
  const detailEvents = useMemo(
    () => (task ? taskEventsForDetail(task, childTasks, taskEvents) : []),
    [childTasks, task, taskEvents],
  );
  const timelineEntries = useMemo(
    () => (task ? taskTimelineEntries(task, childTasks, detailEvents, t, locale) : []),
    [childTasks, detailEvents, locale, t, task],
  );
  const metaTags = useMemo(
    () => (task ? taskMetaTags(task, childCount, t, locale) : []),
    [childCount, locale, t, task],
  );
  const detailRoomID = useMemo(
    () => (task ? taskExecutionRoomID(task, childTasks, teams) : ""),
    [childTasks, task, teams],
  );

  return (
    <DialogRoot open={open} onOpenChange={(nextOpen) => (!nextOpen ? onClose?.() : null)}>
      <DialogContent className="task-detail-dialog">
        <DialogHeader className="task-detail-dialog-header">
          <div className="task-detail-dialog-heading">
            <div className="task-detail-dialog-title-row">
              <DialogTitle className="task-detail-dialog-title">{dialogTitle}</DialogTitle>
              {task ? <TaskStatusPill status={task.status} t={t} showFullLabel /> : null}
            </div>
            <DialogDescription className="task-detail-dialog-subtitle">
              {task ? task.id : t("tasksSelectHint")}
            </DialogDescription>
          </div>
          <TaskDialogCloseButton label={t("close")} />
        </DialogHeader>
        {task ? (
          <>
            <DialogBody className="task-detail-dialog-body">
              <div className="task-detail-layout">
                <main className="task-detail-main">
                  <section className="task-detail-description-block">
                    <h3>{t("taskDescriptionLabel")}</h3>
                    <p>{task.body || t("tasksDetailPlaceholder")}</p>
                  </section>
                  <section className="task-detail-activity-block">
                    <h3>{t("taskActivityLabel")}</h3>
                    <TaskActivityTimeline entries={timelineEntries} emptyLabel={t("taskActivityEmpty")} />
                  </section>
                </main>
                <aside className="task-detail-aside" aria-label={t("taskMetadataLabel")}>
                  <h3>{t("taskMetadataLabel")}</h3>
                  <div className="task-detail-tags">
                    {metaTags.map((item) => (
                      <TaskMetaTag key={item.key} label={item.label} value={item.value} />
                    ))}
                  </div>
                </aside>
              </div>
            </DialogBody>
            <DialogFooter className="task-dialog-actions">
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
      <button type="button" className="task-dialog-close-btn" aria-label={label} title={label}>
        <X size={18} strokeWidth={1.75} aria-hidden="true" />
      </button>
    </DialogClose>
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

type TaskMetaTagItem = {
  key: string;
  label: string;
  value: ReactNode;
};

function TaskActivityTimeline({ entries, emptyLabel }: { entries: TaskTimelineEntry[]; emptyLabel: string }) {
  if (!entries.length) {
    return <div className="task-activity-empty">{emptyLabel}</div>;
  }

  return (
    <ol className="task-activity-list">
      {entries.map((entry) => (
        <li key={entry.id} className={`task-activity-item ${entry.tone ? `task-activity-item-${entry.tone}` : ""}`}>
          <span className="task-activity-marker" aria-hidden="true" />
          <article className="task-activity-content">
            <header className="task-activity-head">
              <div className="task-activity-title-row">
                <strong>{entry.title}</strong>
                {entry.subject ? <span className="task-activity-subject">{entry.subject}</span> : null}
              </div>
              <span>{entry.meta}</span>
            </header>
            {entry.body ? <p>{entry.body}</p> : null}
          </article>
        </li>
      ))}
    </ol>
  );
}

function TaskMetaTag({ label, value }: TaskMetaTagItem) {
  return (
    <div className="task-detail-tag">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function taskMetaTags(
  task: WorkspaceTask,
  childCount: number | undefined,
  t: TranslateFn,
  locale: string,
): TaskMetaTagItem[] {
  const tags: TaskMetaTagItem[] = [
    {
      key: "kind",
      label: t("taskKindLabel"),
      value: task.parent_id ? t("taskKindChild") : t("taskKindParent"),
    },
    {
      key: "status",
      label: t("taskStatusLabel"),
      value: taskStatusLabel(task.status, t),
    },
    {
      key: "assignee",
      label: t("taskAssigneeLabel"),
      value: task.assigned_to || "-",
    },
    {
      key: "claimed_by",
      label: t("taskClaimedByLabel"),
      value: task.claimed_by || "-",
    },
    {
      key: "parent",
      label: t("taskParentLabel"),
      value: task.parent_id || "-",
    },
    {
      key: "team",
      label: t("taskTeamLabel"),
      value: displayTaskTeam(task),
    },
    {
      key: "room",
      label: t("taskRoomLabel"),
      value: displayTaskRoom(task),
    },
    {
      key: "priority",
      label: t("taskPriorityLabel"),
      value: String(task.priority || 0),
    },
    {
      key: "updated_at",
      label: t("taskUpdatedAtLabel"),
      value: formatTaskUpdatedAt(task.updated_at, locale),
    },
    {
      key: "dispatched_at",
      label: t("taskDispatchedAtLabel"),
      value: task.dispatched_at ? formatTaskUpdatedAt(task.dispatched_at, locale) : "-",
    },
    {
      key: "depends_on",
      label: t("taskDependsOnLabel"),
      value: task.depends_on.length ? task.depends_on.join(", ") : "-",
    },
  ];

  if (childCount !== undefined) {
    tags.splice(2, 0, {
      key: "children",
      label: t("taskChildrenLabel"),
      value: t("taskChildrenCount", { count: childCount }),
    });
  }

  return tags;
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
    entries.push({
      id: `synthetic-dispatched-${task.id}`,
      title: t("taskTimelineDispatched"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.dispatched_at, locale),
      body: task.assigned_to ? `${t("taskActivityTargetLabel")}: ${task.assigned_to}` : "",
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
  return [formatTaskUpdatedAt(event.created_at, locale), event.actor_id].filter(Boolean).join(" · ");
}

function taskEventBody(event: WorkspaceTeamEvent, t: TranslateFn): string {
  const lines: string[] = [];
  if (event.summary) {
    lines.push(event.summary);
  }
  if (event.target_id) {
    lines.push(`${t("taskActivityTargetLabel")}: ${event.target_id}`);
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
