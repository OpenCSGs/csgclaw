import { useMemo } from "react";
import { TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui";
import { TaskBoard } from "@/components/business/TaskBoard";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn, UsersById } from "@/models/conversations";
import { roomTaskGroups, roomTaskState } from "@/models/roomTasks";
import { agentNameLookup } from "@/models/taskBoard";
import type { WorkspaceTask } from "@/models/tasks";
import styles from "./RoomTaskPanel.module.css";

const EMPTY_AGENTS: readonly AgentLike[] = [];

type RoomTaskPanelProps = {
  roomID: string;
  tasks: readonly WorkspaceTask[];
  loading: boolean;
  error: string;
  refreshing?: boolean;
  agents?: readonly AgentLike[];
  usersById?: UsersById;
  locale?: string;
  t: TranslateFn;
  onRefresh: () => void;
  onSelectTask: (taskID: string, anchor: HTMLElement) => void;
};

export function RoomTaskPanel({
  roomID,
  tasks,
  loading,
  error,
  refreshing = false,
  agents = EMPTY_AGENTS,
  usersById,
  locale,
  t,
  onRefresh,
  onSelectTask,
}: RoomTaskPanelProps) {
  const scopedTasks = useMemo(
    () => roomTaskGroups(tasks, roomID).flatMap(({ task, children }) => [task, ...children]),
    [tasks, roomID],
  );
  const agentNames = useMemo(() => agentNameLookup(agents, usersById), [agents, usersById]);
  return (
    <div className={styles.panel} aria-busy={loading || refreshing}>
      <section className={styles.board} aria-label={t("roomTasksOpen")}>
        <header className={styles.header}>
          <h2>{t("mainTaskBoardTitle")}</h2>
          <Button
            variant="secondaryGray"
            size="sm"
            disabled={loading || refreshing}
            onClick={onRefresh}
            aria-label={t("tasksRefresh")}
          >
            {t("tasksRefreshShort")}
          </Button>
        </header>
        {loading ? (
          <p className={styles.message} role="status">
            {t("roomPlanLoading")}
          </p>
        ) : null}
        {error ? (
          <p className={styles.message} role="alert">
            {t("roomPlanLoadFailed")} {error}
          </p>
        ) : null}
        {!loading ? (
          <TaskBoard
            tasks={scopedTasks}
            agentNames={agentNames}
            layout="responsive"
            locale={locale}
            t={t}
            renderTaskStatus={(task) =>
              roomTaskState(task, scopedTasks) === "issues" ? (
                <span className={styles.issuesStatus}>
                  <TriangleAlert size={12} aria-hidden="true" />
                  <span>{t("roomTaskStates.issues")}</span>
                </span>
              ) : null
            }
            onSelectTask={(task, anchor) => onSelectTask(task.id, anchor)}
          />
        ) : null}
      </section>
    </div>
  );
}
