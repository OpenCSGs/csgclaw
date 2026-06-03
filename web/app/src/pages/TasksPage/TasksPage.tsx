import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { TasksView } from "./components";

export function TasksPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <TasksView {...controller.taskViewProps} />;
}
