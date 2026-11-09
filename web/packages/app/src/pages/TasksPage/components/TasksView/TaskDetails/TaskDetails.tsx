import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Bot, ChevronDown } from "lucide-react";
import {
  Button,
  Tooltip,
  DialogBody,
  DialogContent,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import { TaskStatusPill } from "@/components/business";
import type { TranslateFn } from "@/models/conversations";
import {
  displayTaskAssignedAgent,
  displayTaskClaimedAgent,
  displayTaskRoomTitle,
  formatTaskUpdatedAt,
  taskExecutionRoomID,
  taskStatusLabel,
} from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTeam, WorkspaceTeamEvent } from "@/models/tasks";
import {
  displayTaskClaimedAgentName,
  displayTaskWorkerName,
  displayTaskAssignmentTargetName,
  taskActiveWorker,
  type TaskActiveWorker,
} from "@/models/taskBoard";
import { classNames } from "@/shared/lib/classNames";
import taskDetailsStyles from "./TaskDetails.module.css";

function RoomTaskSummary({ task, t }: { task: WorkspaceTask; t: TranslateFn }) {
  if (task.assignment_type !== "room" || task.parent_id || !["completed", "failed", "cancelled"].includes(task.status))
    return null;
  return (
    <section className={taskDetailsStyles.detailBlock}>
      <h3>{t("roomTaskSummary")}</h3>
      <p>{task.goal_outcome ? t("roomTaskOutcome." + task.goal_outcome) : t("roomTaskAwaitingSummary")}</p>
      {task.report ? <p className="whitespace-pre-wrap">{task.report}</p> : null}
      {task.report_status === "pending" ? <p>{t("roomTaskReportPending")}</p> : null}
    </section>
  );
}

type TaskDetailDialogProps = {
  agentNames: ReadonlyMap<string, string>;
  childCount?: number;
  childTasks?: WorkspaceTask[];
  onClose?: () => void;
  onOpenConversation: (roomID: string) => void | Promise<void>;
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
  onOpenConversation: (roomID: string) => void | Promise<void>;
  t: TranslateFn;
  task: WorkspaceTask | null;
  taskEvents?: WorkspaceTeamEvent[];
  teams?: WorkspaceTeam[];
};

export function GeneratedTaskInlineDetail({
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
    <section className={taskDetailsStyles.generatedTaskDetail} aria-label={t("generatedTaskDetailTitle")}>
      <header className={taskDetailsStyles.generatedTaskDetailHead}>
        <h3>{t("generatedTaskDetailTitle")}</h3>
        {task && (task.room_id || detailRoomID) ? (
          <Button variant="secondaryGray" size="sm" onClick={() => onOpenConversation(detailRoomID || task.room_id)}>
            {t("taskOpenConversation")}
          </Button>
        ) : null}
      </header>
      <div className={taskDetailsStyles.generatedTaskPanel}>
        {task ? (
          <>
            <header className={taskDetailsStyles.generatedTaskSummary}>
              <div className={taskDetailsStyles.generatedTaskTitleRow}>
                <strong>{task.title}</strong>
                <TaskStatusPill status={task.status} t={t} showFullLabel />
              </div>
              <p>{task.id}</p>
            </header>
            {task.assignment_type === "agent" && (task.result || task.error) ? (
              <section
                className={classNames(taskDetailsStyles.detailBlock, taskDetailsStyles.generatedTaskDescription)}
              >
                <h4>{t(task.result ? "taskResultLabel" : "taskErrorLabel")}</h4>
                <p>{task.result || task.error}</p>
              </section>
            ) : null}
            <section className={classNames(taskDetailsStyles.detailBlock, taskDetailsStyles.generatedTaskDescription)}>
              <h4>{t("taskDescriptionLabel")}</h4>
              <p>{task.body || t("tasksDetailPlaceholder")}</p>
            </section>
            <RoomTaskSummary task={task} t={t} />
            <div className={taskDetailsStyles.generatedTaskDetailGrid}>
              <section className={classNames(taskDetailsStyles.detailBlock, taskDetailsStyles.generatedTaskActivity)}>
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
              <aside className={taskDetailsStyles.generatedTaskMeta} aria-label={t("taskMetadataLabel")}>
                <h4>{t("taskMetadataLabel")}</h4>
                <div className={taskDetailsStyles.taskDetailTags}>
                  {metaTags.map((item) => (
                    <TaskMetaTag key={item.key} label={item.label} value={item.value} />
                  ))}
                </div>
              </aside>
            </div>
          </>
        ) : (
          <div className={taskDetailsStyles.generatedTaskEmpty}>{t("generatedTaskDetailEmpty")}</div>
        )}
      </div>
    </section>
  );
}

export function TaskDetailDialog({
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
      <DialogContent className={taskDetailsStyles.taskDetailDialog}>
        <DialogHeader className={taskDetailsStyles.taskDetailDialogHeader}>
          <div className={taskDetailsStyles.taskDetailDialogHeading}>
            <div className={taskDetailsStyles.taskDetailDialogTitleRow}>
              <DialogTitle className={taskDetailsStyles.taskDetailDialogTitle} title={dialogTitle}>
                {dialogTitle}
              </DialogTitle>
              {task ? <TaskStatusPill status={task.status} t={t} showFullLabel /> : null}
            </div>
            <DialogDescription className={taskDetailsStyles.taskDetailDialogSubtitle}>
              {task ? task.id : t("tasksSelectHint")}
            </DialogDescription>
          </div>
          <div className={taskDetailsStyles.taskDetailDialogTools}>
            {activeWorker ? <TaskActiveWorkerBadge worker={activeWorker} /> : null}
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
          </div>
        </DialogHeader>
        {task ? (
          <>
            <DialogBody className={taskDetailsStyles.taskDetailDialogBody}>
              <div className={taskDetailsStyles.taskDetailLayout}>
                <main className={taskDetailsStyles.taskDetailMain}>
                  <section
                    className={classNames(taskDetailsStyles.detailBlock, taskDetailsStyles.taskDetailDescriptionBlock)}
                  >
                    <h3>{t("taskDescriptionLabel")}</h3>
                    <p>{task.body || t("tasksDetailPlaceholder")}</p>
                  </section>
                  <RoomTaskSummary task={task} t={t} />
                  <section
                    className={classNames(taskDetailsStyles.detailBlock, taskDetailsStyles.taskDetailActivityBlock)}
                  >
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
                <aside className={taskDetailsStyles.taskDetailAside} aria-label={t("taskMetadataLabel")}>
                  {isParentDetail ? <TaskDependencyGraph tasks={childTasks} t={t} /> : null}
                  <h3>{t("taskMetadataLabel")}</h3>
                  <div className={taskDetailsStyles.taskDetailTags}>
                    {metaTags.map((item) => (
                      <TaskMetaTag key={item.key} label={item.label} value={item.value} />
                    ))}
                  </div>
                </aside>
              </div>
            </DialogBody>
            <DialogFooter>
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

function TaskActiveWorkerBadge({ worker }: { worker: TaskActiveWorker }) {
  return (
    <Tooltip content={`${worker.name} ${worker.label}`}>
      <div
        className={classNames(
          taskDetailsStyles.taskActiveWorker,
          worker.tone === "working"
            ? taskDetailsStyles.taskActiveWorkerWorking
            : worker.tone === "planning"
              ? taskDetailsStyles.taskActiveWorkerPlanning
              : undefined,
        )}
      >
        <span className={taskDetailsStyles.taskActiveAvatar} aria-hidden="true">
          <Bot size={14} strokeWidth={1.9} />
        </span>
        <span className={taskDetailsStyles.taskActiveWorkerName}>{worker.name}</span>
        <span>{worker.label}</span>
      </div>
    </Tooltip>
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
    return <div className={taskDetailsStyles.taskActivityEmpty}>{emptyLabel}</div>;
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
    <div className={taskDetailsStyles.taskGroupedActivityList}>
      {parentGroup ? (
        <header className={classNames(taskDetailsStyles.taskActivityGroupHead, taskDetailsStyles.taskActivityRootHead)}>
          <div className={taskDetailsStyles.taskActivityGroupTitle}>
            <span className={taskDetailsStyles.taskActivityGroupKind}>{t("taskTimelineMainTask")}</span>
            <strong>{parentGroup.task.id}</strong>
            <span>{parentGroup.task.title}</span>
          </div>
          <div className={taskDetailsStyles.taskActivityGroupActions}>
            <span>{t("taskTimelineEventsCount", { count: parentGroup.entries.length + childGroups.length })}</span>
          </div>
        </header>
      ) : null}
      <ol className={classNames(taskDetailsStyles.taskActivityList, taskDetailsStyles.taskCombinedActivityList)}>
        {leadingParentEntries.map((entry) => (
          <TaskActivityTimelineItem key={entry.id} entry={entry} />
        ))}
        {childGroups.length ? (
          <li className={classNames(taskDetailsStyles.taskActivityItem, taskDetailsStyles.taskActivityChildStack)}>
            <span className={taskDetailsStyles.taskActivityMarker} aria-hidden="true" />
            <article className={taskDetailsStyles.taskActivityContent}>
              <header className={taskDetailsStyles.taskActivityHead}>
                <div className={taskDetailsStyles.taskActivityTitleRow}>
                  <strong>{t("taskTimelineChildTask")}</strong>
                  <span className={taskDetailsStyles.taskActivitySubject}>
                    {t("taskTimelineEventsCount", { count: childGroups.length })}
                  </span>
                </div>
              </header>
              <div className={taskDetailsStyles.taskChildActivityAccordion}>
                {childGroups.map((group) => {
                  const expanded = expandedTaskIDs.has(group.task.id);
                  const entryCount = group.entries.length;
                  const latestEntry = group.entries[entryCount - 1];
                  const assignee = displayTaskWorkerName(group.task, agentNames) || t("taskAssigneeUnassigned");
                  return (
                    <section key={`child-${group.task.id}`} className={taskDetailsStyles.taskChildActivityItem}>
                      <button
                        type="button"
                        aria-expanded={expanded}
                        onClick={() => toggleTask(group.task.id)}
                        className={taskDetailsStyles.taskChildActivityTrigger}
                      >
                        <span className={taskDetailsStyles.taskChildActivityArrow} aria-hidden="true">
                          <ChevronDown size={16} strokeWidth={1.9} />
                        </span>
                        <span
                          className={classNames(
                            taskDetailsStyles.childActivityMetaRow,
                            taskDetailsStyles.taskChildActivityMain,
                          )}
                        >
                          <span className={taskDetailsStyles.taskActivityGroupKind}>{t("taskTimelineChildTask")}</span>
                          <strong>{group.task.id}</strong>
                          <span>{group.task.title}</span>
                        </span>
                        <span
                          className={classNames(
                            taskDetailsStyles.childActivityMetaRow,
                            taskDetailsStyles.taskChildActivityTags,
                          )}
                        >
                          <span>{assignee}</span>
                          <span>{t("taskTimelineEventsCount", { count: entryCount })}</span>
                          <strong>{expanded ? t("taskTimelineCollapse") : t("taskTimelineExpand")}</strong>
                        </span>
                      </button>
                      {!expanded && latestEntry ? (
                        <div className={taskDetailsStyles.taskChildActivitySummary}>
                          <span>{latestEntry.title}</span>
                          <span>{latestEntry.meta}</span>
                        </div>
                      ) : null}
                      {expanded ? (
                        <div className={taskDetailsStyles.taskChildActivityPanel}>
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
    return <div className={taskDetailsStyles.taskActivityEmpty}>{emptyLabel}</div>;
  }

  return (
    <ol className={taskDetailsStyles.taskActivityList}>
      {entries.map((entry) => (
        <TaskActivityTimelineItem key={entry.id} entry={entry} />
      ))}
    </ol>
  );
}

function TaskActivityTimelineItem({ entry }: { entry: TaskTimelineEntry }) {
  return (
    <li className={classNames(taskDetailsStyles.taskActivityItem, entry.tone && activityToneClass[entry.tone])}>
      <span className={taskDetailsStyles.taskActivityMarker} aria-hidden="true" />
      <article className={taskDetailsStyles.taskActivityContent}>
        <header className={taskDetailsStyles.taskActivityHead}>
          <div className={taskDetailsStyles.taskActivityTitleRow}>
            <strong>{entry.title}</strong>
            {entry.subject ? <span className={taskDetailsStyles.taskActivitySubject}>{entry.subject}</span> : null}
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
    <div className={taskDetailsStyles.taskDetailTag}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function TaskDependencyGraph({ tasks, t }: { tasks: readonly WorkspaceTask[]; t: TranslateFn }) {
  const dependencyLevels = taskDependencyLevels(tasks);

  return (
    <section className={taskDetailsStyles.taskDependencyGraph} aria-label={t("taskDependencyGraphLabel")}>
      <h3>{t("taskDependencyGraphLabel")}</h3>
      {dependencyLevels.length ? (
        <div className={taskDetailsStyles.taskDependencyChain}>
          {dependencyLevels.map((level, levelIndex) => (
            <div key={`level-${levelIndex}`} className={taskDetailsStyles.taskDependencyStage}>
              {levelIndex > 0 ? <span className={taskDetailsStyles.taskDependencyArrow} aria-hidden="true" /> : null}
              <div className={taskDetailsStyles.taskDependencyRow}>
                {level.map((task) => (
                  <Tooltip key={task.id} content={`${task.id} ${task.title}`}>
                    <article className={taskDetailsStyles.taskDependencyCard}>
                      <strong>{task.id}</strong>
                      <span>{task.title}</span>
                    </article>
                  </Tooltip>
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className={taskDetailsStyles.taskDependencyEmpty}>{t("taskBoardNoChildren")}</div>
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

const activityToneClass = {
  success: taskDetailsStyles.taskActivityItemSuccess,
  warning: taskDetailsStyles.taskActivityItemWarning,
  danger: taskDetailsStyles.taskActivityItemDanger,
};
