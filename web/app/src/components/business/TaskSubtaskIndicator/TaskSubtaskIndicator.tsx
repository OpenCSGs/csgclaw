import { ListChecks } from "lucide-react";
import { normalizeTaskStatus, type TaskSidebarPhase } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import "./TaskSubtaskIndicator.css";

type TranslateFn = (key: string, params?: Record<string, string | number>) => string;

type TaskSubtaskIndicatorProps = {
  subtasks: readonly { status: string }[];
  t: TranslateFn;
  phase?: TaskSidebarPhase;
  compact?: boolean;
  className?: string;
};

export function TaskSubtaskIndicator({
  subtasks,
  t,
  phase = "idle",
  compact = false,
  className,
}: TaskSubtaskIndicatorProps) {
  if (phase === "planning" || phase === "dispatching") {
    const label = phase === "planning" ? t("taskSidebarPlanning") : t("taskSidebarDispatching");
    return (
      <span
        className={classNames(
          "task-subtask-indicator",
          compact && "task-subtask-indicator-compact",
          "task-subtask-indicator-loading",
          phase === "planning" ? "task-subtask-indicator-planning" : "task-subtask-indicator-dispatching",
          className,
        )}
        title={label}
        aria-label={label}
      >
        <span className="task-subtask-indicator-action">{label}</span>
        <span className="task-subtask-indicator-dots" aria-hidden="true">
          <span />
          <span />
          <span />
        </span>
      </span>
    );
  }

  const total = subtasks.length;
  if (total === 0) {
    return null;
  }

  const completed = subtasks.filter((task) => normalizeTaskStatus(task.status) === "completed").length;
  const allCompleted = completed === total;
  const inProgress = completed > 0 && !allCompleted;
  const label = t("taskChildrenProgressAria", { completed, total });

  return (
    <span
      className={classNames(
        "task-subtask-indicator",
        compact && "task-subtask-indicator-compact",
        inProgress && "task-subtask-indicator-progress",
        allCompleted && "task-subtask-indicator-complete",
        className,
      )}
      title={label}
      aria-label={label}
    >
      <ListChecks className="task-subtask-indicator-icon" size={12} strokeWidth={2.2} aria-hidden="true" />
      <span className="task-subtask-indicator-value">
        {completed}/{total}
      </span>
    </span>
  );
}
