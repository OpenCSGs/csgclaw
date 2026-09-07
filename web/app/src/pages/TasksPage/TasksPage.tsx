import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { Navigate } from "react-router-dom";
import { roomTaskPath } from "@/models/roomTasks";
import { TasksView } from "./components";

export function TasksPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  const selected = controller.taskViewProps?.selectedTask;
  if (selected?.assignment_type === "room" && selected.room_id) {
    return <Navigate replace to={roomTaskPath(selected.room_id, selected.id)} />;
  }

  return <TasksView {...controller.taskViewProps} />;
}
