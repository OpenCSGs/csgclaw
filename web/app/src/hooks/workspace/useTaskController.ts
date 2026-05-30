import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchGlobalTasks } from "@/api/tasks";
import { WorkspacePaneTypes } from "@/models/routing";
import { errorMessage } from "@/api/client";
import type { TranslateFn } from "@/models/conversations";

type UseTaskControllerArgs = {
  activePane: { type?: string; id?: string };
  t: TranslateFn;
  onSelectConversation: (id: string) => void;
  onSelectTask: (taskID?: string) => void;
};

export function useTaskController({ activePane, t, onSelectConversation, onSelectTask }: UseTaskControllerArgs) {
  const tasksQuery = useQuery({
    queryKey: ["workspace", "tasks"],
    queryFn: fetchGlobalTasks,
  });

  const tasks = useMemo(() => tasksQuery.data ?? [], [tasksQuery.data]);
  const selectedTaskID = activePane.type === WorkspacePaneTypes.task ? String(activePane.id || "") : "";
  const selectedTask = useMemo(() => tasks.find((item) => item.id === selectedTaskID) ?? null, [selectedTaskID, tasks]);

  return {
    tasks,
    taskViewProps: {
      t,
      tasks,
      selectedTask,
      selectedTaskID,
      loading: tasksQuery.isLoading,
      error: tasksQuery.isError ? errorMessage(tasksQuery.error, t("tasksLoadFailed")) : "",
      onRefresh: () => tasksQuery.refetch(),
      onSelectTask,
      onOpenConversation: onSelectConversation,
    },
  };
}
