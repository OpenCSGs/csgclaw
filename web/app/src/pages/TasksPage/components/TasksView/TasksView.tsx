import { useEffect, useMemo, useState } from "react";
import type { ComponentProps, ReactNode } from "react";
import { Bot, ChevronDown, Users, X } from "lucide-react";
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
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import {
  TASK_BOARD_STATUSES,
  displayTaskRoom,
  displayTaskTeam,
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

function moduleSuffixStyle(prefix: string, suffix: string | undefined): string {
  if (!suffix) {
    return "";
  }
  const normalized = suffix.replace(/-+([a-zA-Z0-9_])/g, (_, char: string) => char.toUpperCase());
  const key = `${prefix}${normalized.charAt(0).toUpperCase()}${normalized.slice(1)}`;
  return styles[key] ?? "";
}

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
  onCloseTaskDetails?: () => VoidOrPromise;
  onCreateTask?: (payload: CreateWorkspaceTaskPayload) => VoidOrPromise;
  onOpenConversation?: (roomID: string) => VoidOrPromise;
  onPlanTask?: (taskID: string) => VoidOrPromise;
  onRefresh?: () => VoidOrPromise;
  onStartTask?: (taskID: string) => VoidOrPromise;
  onViewParentDetail?: (taskID: string) => VoidOrPromise;
  parentDetailTaskID?: string;
  planTaskBusy?: boolean;
  planningTaskID?: string;
  selectedTask?: WorkspaceTask | null;
  showCreateTaskModal?: boolean;
  startTaskBusy?: boolean;
  startingTaskID?: string;
  taskActionError?: string;
  taskEvents?: WorkspaceTeamEvent[];
  tasks?: WorkspaceTask[];
  t?: TranslateFn;
  teams?: WorkspaceTeam[];
};

export function TasksView({
  t = (key) => key,
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
  showCreateTaskModal = false,
  parentDetailTaskID = "",
  planningTaskID = "",
  startingTaskID = "",
  onCloseCreateTaskModal,
  onCloseParentTaskDetail,
  onCloseTaskDetails,
  onCreateTask,
  onRefresh = () => {},
  onOpenConversation = () => {},
}: TasksViewProps) {
  const parentTasks = useMemo(() => rootTasks(tasks), [tasks]);
  const [parentDialogTaskID, setParentDialogTaskID] = useState("");
  const dialogStateRootTask = useMemo(
    () => (parentDialogTaskID ? (parentTasks.find((item) => item.id === parentDialogTaskID) ?? null) : null),
    [parentDialogTaskID, parentTasks],
  );
  const parentDetailTask = useMemo(() => {
    if (!parentDetailTaskID) {
      return null;
    }
    const task = tasks.find((item) => item.id === parentDetailTaskID) ?? null;
    return rootTaskForTask(tasks, task) ?? task;
  }, [parentDetailTaskID, tasks]);
  const parentDialogTask = parentDetailTask ?? dialogStateRootTask;
  const parentDialogChildTasks = useMemo(
    () => (parentDialogTask ? taskChildren(tasks, parentDialogTask.id) : []),
    [parentDialogTask, tasks],
  );
  const parentColumns = useMemo(() => boardColumnsForParentTasks(parentTasks), [parentTasks]);
  const [createDraft, setCreateDraft] = useState<TaskCreateDraft>(emptyCreateDraft);

  useEffect(() => {
    if (!showCreateTaskModal) {
      return;
    }
    setCreateDraft({
      ...emptyCreateDraft,
      team_id: teams[0]?.id || "",
    });
  }, [showCreateTaskModal, teams]);

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
      execution_channel: "csgclaw",
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

  return (
    <section className={classNames("entity-pane", "tasks-pane", styles.tasksPane)}>
      {error ? <div className="form-error">{error}</div> : null}
      {taskActionError ? (
        <div className={classNames("form-error", styles.tasksActionError)}>{taskActionError}</div>
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
                onRefresh={onRefresh}
              />
            </div>
            <div className={styles.tasksKanbanScroll} role="region" aria-label={t("mainTaskBoardTitle")}>
              <div className={styles.tasksKanban}>
                {parentColumns.map((column) => (
                  <section
                    key={column.status}
                    className={classNames(styles.taskBoardColumn, moduleSuffixStyle("taskBoardColumn", column.status))}
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
                          const children = taskChildren(tasks, task.id);
                          const phase = resolveTaskSidebarPhase(task, children, { planningTaskID, startingTaskID });
                          return (
                            <ParentTaskBoardCard
                              key={task.id}
                              task={task}
                              children={children}
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
          </div>
        </div>
      ) : null}
      <TaskDetailDialog
        t={t}
        title={t("taskParentDetailTitle")}
        task={parentDialogTask}
        childCount={parentDialogChildTasks.length}
        childTasks={parentDialogChildTasks}
        teams={teams}
        taskEvents={taskEvents}
        open={Boolean(parentDialogTask)}
        onClose={closeRootTaskDetail}
        onOpenConversation={onOpenConversation}
      />
      <DialogRoot open={showCreateTaskModal} onOpenChange={(open) => (!open ? onCloseCreateTaskModal?.() : null)}>
        <DialogContent className={styles.taskCreateDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("taskCreateTitle")}</DialogTitle>
              <DialogDescription>{t("taskCreateSubtitle")}</DialogDescription>
            </div>
            <TaskDialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <div className={classNames(styles.taskCreateForm, styles.taskCreateFormCompact)}>
              <label className={classNames("field", styles.taskCreateField)}>
                <span>{t("taskTitleLabel")}</span>
                <input
                  value={createDraft.title}
                  maxLength={TASK_TITLE_MAX_LENGTH}
                  onInput={(event) => setCreateDraft((current) => ({ ...current, title: event.currentTarget.value }))}
                  placeholder={t("taskTitlePlaceholder")}
                />
              </label>
              <label className={classNames("field", styles.taskCreateField)}>
                <span>{t("taskDescriptionLabel")}</span>
                <textarea
                  value={createDraft.description}
                  onInput={(event) =>
                    setCreateDraft((current) => ({ ...current, description: event.currentTarget.value }))
                  }
                  placeholder={t("taskDescriptionPlaceholder")}
                />
              </label>
              <label className={classNames("field", styles.taskCreateField)}>
                <span>{t("taskTeamLabel")}</span>
                <Select
                  value={createDraft.team_id}
                  onValueChange={(teamID) => setCreateDraft((current) => ({ ...current, team_id: teamID }))}
                  triggerProps={{ "aria-label": t("taskTeamLabel") }}
                  options={teams.map((team) => ({ value: team.id, label: displayTeam(team) }))}
                />
              </label>
            </div>
            {createTaskError ? (
              <div className={classNames("form-error", styles.taskCreateError)}>{createTaskError}</div>
            ) : null}
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
    <div
      className={classNames(styles.headerRow, styles.justifyEnd, styles.tasksToolbar)}
      aria-label={t("tasksActionsLabel")}
    >
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
  children: WorkspaceTask[];
  onSelect: () => void;
  phase: TaskSidebarPhase;
  t: TranslateFn;
  task: WorkspaceTask;
};

function ParentTaskBoardCard({ task, children, phase, t, onSelect }: ParentTaskBoardCardProps) {
  const description = task.body || task.plan_summary || task.result || task.error || t("tasksDetailPlaceholder");
  const activeWorker = taskActiveWorker(task, children, t);
  const updatedRelative = formatTaskUpdatedRelative(task.updated_at, document.documentElement.lang);
  const updatedLabel = updatedRelative === "-" ? "" : t("taskCardUpdatedAt", { time: updatedRelative });

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
        <span className={styles.taskBoardCardTeam} title={displayTaskTeam(task)}>
          <span className={styles.taskBoardCardTeamIcon} aria-hidden="true">
            <Users size={13} strokeWidth={1.8} />
          </span>
          <span>{displayTaskTeam(task)}</span>
        </span>
        {updatedLabel ? <span className={styles.taskBoardCardUpdated}>{updatedLabel}</span> : null}
      </span>
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
    () => (task ? taskMetaTags(task, childCount, t, locale) : []),
    [childCount, locale, t, task],
  );
  const detailRoomID = useMemo(
    () => (task ? taskExecutionRoomID(task, childTasks, teams) : ""),
    [childTasks, task, teams],
  );
  const activeWorker = useMemo(
    () => (task && isParentDetail ? taskActiveWorker(task, childTasks, t) : null),
    [childTasks, isParentDetail, t, task],
  );

  return (
    <DialogRoot open={open} onOpenChange={(nextOpen) => (!nextOpen ? onClose?.() : null)}>
      <DialogContent className={styles.taskDetailDialog}>
        <DialogHeader className={styles.taskDetailDialogHeader}>
          <div className={styles.taskDetailDialogHeading}>
            <div className={styles.taskDetailDialogTitleRow}>
              <DialogTitle className={styles.taskDetailDialogTitle}>{dialogTitle}</DialogTitle>
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
                      <TaskGroupedActivityTimeline groups={timelineGroups} emptyLabel={t("taskActivityEmpty")} t={t} />
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
  emptyLabel,
  t,
}: {
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
                  const assignee =
                    group.task.assigned_to_agent_name || group.task.assigned_to || t("taskAssigneeUnassigned");
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
): TaskActiveWorker | null {
  const activeChild = childTasks
    .filter((child) => !isTerminalTaskStatus(child.status))
    .filter((child) => taskWorkerName(child))
    .sort(
      (left, right) =>
        activeWorkerStatusRank(left.status) - activeWorkerStatusRank(right.status) ||
        right.updated_at.localeCompare(left.updated_at),
    )[0];
  if (activeChild) {
    const workerName = taskWorkerName(activeChild);
    if (!workerName) {
      return null;
    }
    return {
      name: workerName,
      label: t("taskActiveWorkerWorking"),
      tone: "working",
    };
  }
  const parentWorkerName = taskWorkerName(task);
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

function taskWorkerName(task: WorkspaceTask): string {
  const name = task.claimed_by_agent_name || task.claimed_by || task.assigned_to_agent_name || task.assigned_to || "";
  return isDisplayableWorkerName(name) ? name : "";
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

  addTag("assignee", t("taskAssigneeLabel"), task.assigned_to_agent_name || task.assigned_to);
  addTag("claimed_by", t("taskClaimedByLabel"), task.claimed_by_agent_name || task.claimed_by);
  addTag("parent", t("taskParentLabel"), task.parent_id);
  addTag("team", t("taskTeamLabel"), displayTaskTeam(task));
  addTag("execution_channel", t("taskExecutionChannelLabel"), task.execution_channel);
  addTag("room", t("taskRoomLabel"), displayTaskRoom(task));
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
  if ((task.assigned_to_agent_name || task.assigned_to) && !existingEventTypes.has("task.assigned")) {
    const assignee = task.assigned_to_agent_name || task.assigned_to;
    entries.push({
      id: `synthetic-assigned-${task.id}`,
      title: t("taskTimelineAssigned"),
      subject: task.id,
      meta: formatTaskUpdatedAt(task.updated_at, locale),
      body: `${t("taskActivityTargetLabel")}: ${assignee}`,
      order: syntheticOrder(),
    });
  }
  if ((task.claimed_by_agent_name || task.claimed_by) && !existingEventTypes.has("task.claimed")) {
    const claimedBy = task.claimed_by_agent_name || task.claimed_by;
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
    const assignee = task.assigned_to_agent_name || task.assigned_to;
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
  return [formatTaskUpdatedAt(event.created_at, locale), event.actor_agent_name || event.actor_id]
    .filter(Boolean)
    .join(" · ");
}

function taskEventBody(event: WorkspaceTeamEvent, t: TranslateFn): string {
  const lines: string[] = [];
  if (event.summary) {
    lines.push(event.summary);
  }
  const target = event.target_agent_name || event.target_id;
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
