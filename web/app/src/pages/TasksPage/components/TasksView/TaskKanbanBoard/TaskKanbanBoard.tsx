import { useEffect, useMemo, useState } from "react";
import { Select, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { formatTaskUpdatedRelative, taskStatusLabel } from "@/models/tasks";
import type { WorkspaceTask, WorkspaceTaskColumn } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "../TasksView.module.css";

type TaskKanbanBoardProps = {
  activeParentTask: WorkspaceTask | null;
  agentNames: ReadonlyMap<string, string>;
  columns: WorkspaceTaskColumn[];
  parentTasks: WorkspaceTask[];
  t: TranslateFn;
  tasks: WorkspaceTask[];
  onOpenTask: (task: WorkspaceTask) => void;
  onSelectParentTask: (taskID: string) => void;
};

export function TaskKanbanBoard({
  activeParentTask,
  agentNames,
  columns,
  parentTasks,
  t,
  tasks,
  onOpenTask,
  onSelectParentTask,
}: TaskKanbanBoardProps) {
  const [query, setQuery] = useState("");
  const [assigneeID, setAssigneeID] = useState("");
  const tasksByID = useMemo(() => new Map(tasks.map((task) => [task.id, task])), [tasks]);
  const parentOptions = useMemo(
    () =>
      parentTasks.map((task) => ({
        value: task.id,
        label: task.title,
        description: task.id,
        textValue: `${task.title} ${task.id}`,
      })),
    [parentTasks],
  );
  const assigneeOptions = useMemo(() => {
    const options = new Map<string, string>();
    for (const column of columns) {
      for (const task of column.tasks) {
        const workerID = taskWorkerID(task);
        const workerName = taskWorkerName(task, agentNames);
        if (workerID && workerName && !options.has(workerID)) {
          options.set(workerID, workerName);
        }
      }
    }
    return [
      { value: "", label: t("taskBoardAllAssignees") },
      ...Array.from(options.entries())
        .sort(([, left], [, right]) => left.localeCompare(right))
        .map(([value, label]) => ({ value, label })),
    ];
  }, [agentNames, columns, t]);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const visibleColumns = useMemo(
    () =>
      columns.map((column) => ({
        ...column,
        tasks: column.tasks.filter((task) => {
          if (assigneeID && taskWorkerID(task) !== assigneeID) {
            return false;
          }
          if (!normalizedQuery) {
            return true;
          }
          return taskSearchText(task).includes(normalizedQuery);
        }),
      })),
    [assigneeID, columns, normalizedQuery],
  );

  useEffect(() => {
    setQuery("");
    setAssigneeID("");
  }, [activeParentTask?.id]);

  return (
    <div className={styles.taskBoardView}>
      <div className={styles.taskBoardControls}>
        <Select
          value={activeParentTask?.id || ""}
          options={parentOptions}
          disabled={!parentOptions.length}
          searchable={parentOptions.length > 8}
          size="sm"
          placeholder={t("taskBoardSelectPlaceholder")}
          searchPlaceholder={t("taskBoardSelectSearchPlaceholder")}
          triggerClassName={styles.taskBoardParentSelect}
          triggerProps={{ "aria-label": t("taskBoardSelectLabel") }}
          onValueChange={onSelectParentTask}
        />
        <input
          className={styles.taskBoardSearchInput}
          type="search"
          value={query}
          aria-label={t("taskBoardSearchLabel")}
          placeholder={t("taskBoardSearchPlaceholder")}
          onChange={(event) => setQuery(event.currentTarget.value)}
        />
        <Select
          value={assigneeID}
          options={assigneeOptions}
          size="sm"
          triggerClassName={styles.taskBoardAssigneeSelect}
          triggerProps={{ "aria-label": t("taskBoardAssigneeFilterLabel") }}
          onValueChange={setAssigneeID}
        />
      </div>
      <div className={styles.taskBoardHint}>{t("taskBoardInteractionHint")}</div>
      <div className={styles.tasksKanbanScroll} role="region" aria-label={t("subTaskBoardTitle")}>
        <div className={styles.tasksKanban}>
          {visibleColumns.map((column) => (
            <section
              key={column.status}
              className={classNames(styles.taskBoardColumn, statusStyle("taskBoardColumn", column.status))}
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
                  column.tasks.map((task) => (
                    <ChildTaskBoardCard
                      key={task.id}
                      task={task}
                      agentNames={agentNames}
                      tasksByID={tasksByID}
                      t={t}
                      onSelect={() => onOpenTask(task)}
                    />
                  ))
                ) : (
                  <div className={styles.taskBoardEmpty}>{t("taskBoardColumnEmpty")}</div>
                )}
              </div>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}

function ChildTaskBoardCard({
  task,
  agentNames,
  tasksByID,
  t,
  onSelect,
}: {
  agentNames: ReadonlyMap<string, string>;
  onSelect: () => void;
  t: TranslateFn;
  task: WorkspaceTask;
  tasksByID: ReadonlyMap<string, WorkspaceTask>;
}) {
  const workerName = taskWorkerName(task, agentNames) || t("taskAssigneeUnassigned");
  const unresolvedDependencies = task.depends_on
    .map((dependencyID) => tasksByID.get(dependencyID))
    .filter((dependency): dependency is WorkspaceTask => Boolean(dependency && dependency.status !== "completed"));
  const updatedRelative = formatTaskUpdatedRelative(task.updated_at, document.documentElement.lang);
  const updatedLabel = updatedRelative === "-" ? "" : t("taskCardUpdatedAt", { time: updatedRelative });
  const dependencyLabel = unresolvedDependencies.length
    ? t("taskBoardWaitingDependencies", { count: unresolvedDependencies.length })
    : task.depends_on.length
      ? t("taskBoardDependenciesReady")
      : "";

  return (
    <Tooltip content={`${task.id} ${task.title}`}>
      <button type="button" className={styles.taskBoardCard} onClick={onSelect}>
        <span className={styles.taskBoardCardTopline}>
          {task.priority > 0 ? (
            <span className={styles.taskBoardCardPriority} data-priority={taskPriorityTone(task.priority)}>
              P{task.priority}
            </span>
          ) : null}
          <strong className={classNames(styles.lineClampText, styles.taskBoardCardTitle)}>{task.title}</strong>
        </span>
        <span className={styles.taskBoardCardMeta}>
          <span className={styles.taskBoardCardWorker}>
            <span className={styles.taskBoardCardAvatar} data-tone={workerAvatarTone(taskWorkerID(task))}>
              {workerName.slice(0, 1).toLocaleUpperCase()}
            </span>
            <span className={styles.taskBoardCardWorkerName}>{workerName}</span>
          </span>
        </span>
        {dependencyLabel || updatedLabel ? (
          <span className={styles.taskBoardCardTags}>
            {unresolvedDependencies.length ? (
              <Tooltip
                content={unresolvedDependencies.map((dependency) => `${dependency.id} ${dependency.title}`).join(", ")}
              >
                <span className={styles.taskBoardCardTag} data-tone="warning">
                  {dependencyLabel}
                </span>
              </Tooltip>
            ) : dependencyLabel ? (
              <span className={styles.taskBoardCardTag}>{dependencyLabel}</span>
            ) : null}
            {updatedLabel ? (
              <span className={styles.taskBoardCardTag} data-tone="time">
                {updatedLabel}
              </span>
            ) : null}
          </span>
        ) : null}
      </button>
    </Tooltip>
  );
}

function taskWorkerID(task: WorkspaceTask): string {
  return task.claimed_by || task.assigned_to || (task.assignment_type === "agent" ? task.assignment_id : "");
}

function taskWorkerName(task: WorkspaceTask, agentNames: ReadonlyMap<string, string>): string {
  const workerID = taskWorkerID(task);
  return task.claimed_by_agent_name || task.assigned_to_agent_name || agentNames.get(workerID) || "";
}

function taskSearchText(task: WorkspaceTask): string {
  return [task.id, task.title].join(" ").toLocaleLowerCase();
}

function taskPriorityTone(priority: number): "urgent" | "high" | "normal" {
  if (priority >= 8) {
    return "urgent";
  }
  if (priority >= 5) {
    return "high";
  }
  return "normal";
}

function workerAvatarTone(workerID: string): string {
  let hash = 0;
  for (const char of workerID) {
    hash = (hash * 31 + char.charCodeAt(0)) % 5;
  }
  return String(hash);
}

function statusStyle(prefix: string, status: string): string {
  const normalized = status.replace(/-+([a-zA-Z0-9_])/g, (_, char: string) => char.toUpperCase());
  const key = `${prefix}${normalized.charAt(0).toUpperCase()}${normalized.slice(1)}`;
  return styles[key] ?? "";
}

function TaskBoardStatusIcon({ status }: { status: string }) {
  const isReview = status === "blocked" || status === "in_review";
  const isRunning = status === "in_progress" || status === "running";
  const isBlocked = status === "failed" || status === "cancelled" || status === "canceled";
  const isFilled = isReview || isRunning || isBlocked;

  return (
    <svg className={styles.taskBoardStatusIcon} viewBox="0 0 14 14" fill="none" aria-hidden="true">
      <circle cx={7} cy={7} r={6} fill={isFilled ? "currentColor" : "none"} stroke="currentColor" strokeWidth={1.5} />
      {isReview ? (
        <path d="M4.2 7.1 6.1 9 9.9 5.2" fill="none" stroke="white" strokeWidth={1.4} strokeLinecap="round" />
      ) : isRunning ? (
        <path d="M5.7 4.8 9.5 7 5.7 9.2Z" fill="white" stroke="none" />
      ) : isBlocked ? (
        <>
          <path d="M7 4.1v3.8" stroke="white" strokeWidth={1.4} strokeLinecap="round" />
          <circle cx={7} cy={10} r={0.8} fill="white" />
        </>
      ) : null}
    </svg>
  );
}
