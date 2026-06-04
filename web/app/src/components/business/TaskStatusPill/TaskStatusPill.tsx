import { normalizeTaskStatus, taskStatusLabel, taskStatusShortLabel } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import "./TaskStatusPill.css";

type TranslateFn = (key: string) => string;

type TaskStatusPillProps = {
  status: unknown;
  t: TranslateFn;
  compact?: boolean;
  showFullLabel?: boolean;
  className?: string;
};

export function TaskStatusPill({ status, t, compact = false, showFullLabel = false, className }: TaskStatusPillProps) {
  const normalized = normalizeTaskStatus(status);
  const shortLabel = taskStatusShortLabel(normalized, t);
  const fullLabel = taskStatusLabel(normalized, t);

  return (
    <span
      className={classNames(
        "task-status-pill",
        `task-status-${normalized}`,
        compact && "task-status-pill-compact",
        className,
      )}
      title={fullLabel}
      aria-label={fullLabel}
    >
      {showFullLabel ? fullLabel : shortLabel}
    </span>
  );
}
