import { ListTodo, CheckCircle2, Circle, Eye } from "lucide-react";
import {
  Button,
  DialogRoot,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  Tooltip,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTask } from "@/models/tasks";
import { roomTaskGroups, roomTaskParent, roomTaskState } from "@/models/roomTasks";
import styles from "./RoomTaskDialog.module.css";

type Shared = { tasks: readonly WorkspaceTask[]; t: TranslateFn };
function taskNumber(id: string): string {
  return `#${id.replace(/^task-/, "")}`;
}
function Status({ task, tasks, t }: Shared & { task: WorkspaceTask }) {
  const state = roomTaskState(task, tasks);
  return (
    <span className={styles.status} data-state={state}>
      {t("roomTaskStates." + state)}
    </span>
  );
}
export function RoomTaskReference({
  task,
  tasks,
  t,
  onOpen,
  compact = false,
  taskID = task?.id ?? "",
}: Shared & { compact?: boolean; taskID?: string; task?: WorkspaceTask; onOpen: (anchor: HTMLElement) => void }) {
  const children = tasks.filter((child) => child.parent_id === task?.id);
  if (compact) {
    const label = `${t("roomTaskView")} · ${task?.title || taskID}`;
    return (
      <Tooltip content={label} contentProps={{ side: "top" }}>
        <button
          type="button"
          className={styles.compactReference}
          aria-label={label}
          onClick={(event) => onOpen(event.currentTarget)}
        >
          <Eye size={16} aria-hidden="true" />
          <span>{taskID ? taskNumber(taskID) : t("roomTaskView")}</span>
        </button>
      </Tooltip>
    );
  }
  return (
    <Button
      variant="tertiaryGray"
      size="sm"
      className={styles.reference}
      onClick={(event) => onOpen(event.currentTarget)}
    >
      <ListTodo size={16} aria-hidden="true" />
      <span>{t("roomTaskView")}</span>
      {task ? (
        <>
          <span className={styles.title}>{task.title}</span>
          {children.length ? (
            <span>
              {children.filter((child) => child.status === "completed").length}/{children.length}
            </span>
          ) : null}
          <Status task={task} tasks={tasks} t={t} />
        </>
      ) : null}
    </Button>
  );
}
type Props = Shared & {
  roomID: string;
  roomTitle: string;
  taskID: string;
  loading: boolean;
  error: string;
  onClose: () => void;
  onRestoreFocus: () => void;
};
export function RoomTaskDialog({
  roomID,
  roomTitle,
  taskID,
  tasks,
  loading,
  error,
  onClose,
  onRestoreFocus,
  t,
}: Props) {
  const groups = roomTaskGroups(tasks, roomID);
  const scoped = groups.flatMap((group) => [group.task, ...group.children]);
  const parent = roomTaskParent(scoped, taskID);
  const visibleGroups = taskID === "list" ? groups : groups.filter((group) => group.task.id === parent?.id);
  return (
    <DialogRoot
      open={Boolean(taskID)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className={styles.dialog}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          onRestoreFocus();
        }}
      >
        <div className={styles.header}>
          <div className={styles.heading}>
            <div className={styles.headingRow}>
              <DialogTitle>{parent?.title ?? t("roomTasksOpen")}</DialogTitle>
              {parent ? <Status task={parent} tasks={scoped} t={t} /> : null}
            </div>
            <DialogDescription className={styles.description}>
              {parent ? (
                <span className={styles.taskNumber} title={parent.id}>
                  {taskNumber(parent.id)}
                </span>
              ) : null}
              <span>{roomTitle}</span>
            </DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
        </div>
        <div className={styles.body}>
          {loading ? <p role="status">{t("roomPlanLoading")}</p> : null}
          {error ? (
            <p role="alert">
              {t("roomPlanLoadFailed")} {error}
            </p>
          ) : null}
          {!loading && !visibleGroups.length ? (
            <p>{t(taskID === "list" ? "roomPlanEmpty" : "roomTaskNotFound")}</p>
          ) : null}
          {visibleGroups.map((group) => (
            <section key={group.task.id} className={styles.group} aria-label={group.task.title}>
              {taskID === "list" ? (
                <div className={styles.headingRow}>
                  <span className={styles.taskNumber} title={group.task.id}>
                    {taskNumber(group.task.id)}
                  </span>
                  <h3>{group.task.title}</h3>
                  <Status task={group.task} tasks={scoped} t={t} />
                </div>
              ) : null}
              {group.children.length ? (
                <ol className={styles.children} aria-label={t("roomTaskChildren")}>
                  {group.children.map((child) => (
                    <li key={child.id} className={styles.row}>
                      {child.status === "completed" ? (
                        <CheckCircle2 className={styles.completedIcon} size={18} aria-hidden="true" />
                      ) : (
                        <Circle className={styles.pendingIcon} size={16} aria-hidden="true" />
                      )}
                      <span className={styles.taskNumber} title={child.id}>
                        {taskNumber(child.id)}
                      </span>
                      <strong>{child.title}</strong>
                      <span className={styles.assignee}>
                        {child.assigned_to_agent_name || child.assigned_to || t("taskAssigneeUnassigned")}
                      </span>
                      <Status task={child} tasks={scoped} t={t} />
                    </li>
                  ))}
                </ol>
              ) : (
                <p>{t("roomPlanWaiting")}</p>
              )}
            </section>
          ))}
        </div>
      </DialogContent>
    </DialogRoot>
  );
}
