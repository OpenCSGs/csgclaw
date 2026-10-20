import { useMemo, type ReactNode } from "react";
import { Bot, Users } from "lucide-react";
import { Tooltip } from "@/components/ui";
import { TaskSubtaskIndicator } from "@/components/business/TaskSubtaskIndicator";
import type { TranslateFn } from "@/models/conversations";
import { displayTaskAssignmentTargetName, taskActiveWorker } from "@/models/taskBoard";
import {
  boardColumnsForTasks,
  formatTaskUpdatedRelative,
  resolveTaskSidebarPhase,
  rootTasks,
  taskChildren,
  taskStatusLabel,
  type TaskSidebarPhase,
  type WorkspaceTask,
} from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TaskBoard.module.css";

type TaskBoardProps = {
  tasks: readonly WorkspaceTask[];
  agentNames: ReadonlyMap<string, string>;
  onSelectTask: (task: WorkspaceTask, anchor: HTMLButtonElement) => void;
  renderTaskStatus?: (task: WorkspaceTask) => ReactNode;
  planningTaskID?: string;
  startingTaskID?: string;
  layout?: "columns" | "responsive";
  locale?: string;
  t: TranslateFn;
};

export function TaskBoard({
  tasks,
  agentNames,
  onSelectTask,
  renderTaskStatus,
  planningTaskID,
  startingTaskID,
  layout = "columns",
  locale,
  t,
}: TaskBoardProps) {
  const columns = useMemo(() => boardColumnsForTasks(rootTasks(tasks)), [tasks]);
  return (
    <div
      className={classNames(styles.tasksKanbanScroll, layout === "responsive" && styles.responsive)}
      role="region"
      aria-label={t("mainTaskBoardTitle")}
    >
      <div className={styles.tasksKanban}>
        {columns.map((column) => (
          <section
            key={column.status}
            className={styles.taskBoardColumn}
            data-status={column.status}
            aria-label={taskStatusLabel(column.status, t)}
          >
            <header className={styles.taskBoardColumnHead}>
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
                      agentNames={agentNames}
                      phase={phase}
                      status={renderTaskStatus?.(task)}
                      locale={locale}
                      t={t}
                      onSelect={(anchor) => onSelectTask(task, anchor)}
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
  onSelect: (anchor: HTMLButtonElement) => void;
  locale?: string;
  phase: TaskSidebarPhase;
  t: TranslateFn;
  task: WorkspaceTask;
  status?: ReactNode;
};

function ParentTaskBoardCard({
  task,
  children,
  agentNames,
  phase,
  t,
  onSelect,
  locale,
  status,
}: ParentTaskBoardCardProps) {
  const description = task.body || task.plan_summary || task.result || task.error || t("tasksDetailPlaceholder");
  const activeWorker = taskActiveWorker(task, children, t, agentNames);
  const updatedRelative = formatTaskUpdatedRelative(task.updated_at, locale || document.documentElement.lang);
  const updatedLabel = updatedRelative === "-" ? "" : t("taskCardUpdatedAt", { time: updatedRelative });
  const assignmentTarget = displayTaskAssignmentTargetName(task, agentNames);

  return (
    <Tooltip content={`${task.id} ${task.title}`}>
      <button
        type="button"
        className={classNames(styles.taskBoardCard, styles.parentTaskBoardCard)}
        onClick={(event) => onSelect(event.currentTarget)}
      >
        <span className={styles.taskBoardCardTopline}>
          <span className={styles.taskBoardCardId}>{task.id}</span>
          <span className={styles.taskBoardCardActions}>
            {activeWorker ? (
              <Tooltip content={`${activeWorker.name} ${activeWorker.label}`}>
                <span className={styles.taskBoardCardWorkerBadge}>
                  <Bot size={12} strokeWidth={1.9} aria-hidden="true" />
                  <span>{activeWorker.label}</span>
                </span>
              </Tooltip>
            ) : null}
            <TaskSubtaskIndicator subtasks={children} phase={phase} t={t} compact />
          </span>
        </span>
        <strong className={classNames(styles.lineClampText, styles.taskBoardCardTitle)}>{task.title}</strong>
        {status}
        <span className={classNames(styles.lineClampText, styles.taskBoardCardDescription)}>{description}</span>
        <span className={styles.taskBoardCardFooter}>
          {assignmentTarget ? (
            <Tooltip content={assignmentTarget}>
              <span className={styles.taskBoardCardTeam}>
                <span className={styles.taskBoardCardTeamIcon} aria-hidden="true">
                  {task.assignment_type === "agent" ? (
                    <Bot size={13} strokeWidth={1.8} />
                  ) : (
                    <Users size={13} strokeWidth={1.8} />
                  )}
                </span>
                <span>{assignmentTarget}</span>
              </span>
            </Tooltip>
          ) : null}
          {updatedLabel ? <span className={styles.taskBoardCardUpdated}>{updatedLabel}</span> : null}
        </span>
      </button>
    </Tooltip>
  );
}
