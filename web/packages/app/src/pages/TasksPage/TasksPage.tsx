import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { Navigate } from "react-router-dom";
import { roomTaskPath } from "@/models/roomTasks";
import { useScheduledTaskConversation } from "./useScheduledTaskConversation";
import { TasksView } from "./components";

export function TasksPage() {
  const controller = useWorkspaceControllerContext();
  const scheduledEntry = useScheduledTaskConversation();

  if (!controller.ready) {
    return null;
  }

  const selected = controller.taskViewProps?.selectedTask;
  if (selected?.assignment_type === "room" && selected.room_id) {
    return <Navigate replace to={roomTaskPath(selected.room_id, selected.id)} />;
  }

  return (
    <TasksView
      {...controller.taskViewProps}
      onSendTextToManager={scheduledEntry.send}
      scheduledTextBusy={scheduledEntry.busy}
      scheduledTextError={scheduledEntry.error}
    />
  );
}
