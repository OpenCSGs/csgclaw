import { Tooltip } from "@/components/ui";
import { normalizeTaskStatus, taskStatusLabel, taskStatusShortLabel } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TaskStatusPill.module.css";

type TranslateFn = (key: string) => string;

type TaskStatusPillProps = {
  status: unknown;
  t: TranslateFn;
  compact?: boolean;
  showFullLabel?: boolean;
  className?: string;
};

function statusClassName(status: string): string {
  switch (status) {
    case "assigned":
      return styles.assigned;
    case "in_progress":
    case "running":
      return styles.inProgress;
    case "in_review":
      return styles.inReview;
    case "completed":
    case "done":
      return styles.completed;
    case "blocked":
      return styles.blocked;
    case "failed":
      return styles.failed;
    case "cancelled":
      return styles.cancelled;
    case "canceled":
      return styles.canceled;
    case "pending":
    default:
      return styles.pending;
  }
}

export function TaskStatusPill({ status, t, compact = false, showFullLabel = false, className }: TaskStatusPillProps) {
  const normalized = normalizeTaskStatus(status);
  const shortLabel = taskStatusShortLabel(normalized, t);
  const fullLabel = taskStatusLabel(normalized, t);

  return (
    <Tooltip content={fullLabel}>
      <span
        className={classNames(styles.pill, statusClassName(normalized), compact && styles.compact, className)}
        aria-label={fullLabel}
      >
        {showFullLabel ? fullLabel : shortLabel}
      </span>
    </Tooltip>
  );
}
