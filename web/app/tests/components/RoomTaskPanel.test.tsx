import { fireEvent, render, screen } from "@testing-library/react";
import { RoomTaskPanel } from "@/components/business/ConversationWorkspace/RoomTaskPanel";
import { normalizeTaskList } from "@/models/tasks";
import { createTranslator } from "@/shared/i18n";

const t = createTranslator("en");

describe("RoomTaskPanel", () => {
  it("keeps room-scoped status cards, task selection and refresh after removing the standalone board", () => {
    const tasks = normalizeTaskList([
      {
        id: "task-1",
        title: "Room task",
        assignment_type: "room",
        assignment_id: "room-1",
        status: "completed",
        goal_outcome: "issues",
      },
      { id: "task-2", title: "Other room task", assignment_type: "room", assignment_id: "room-2", status: "pending" },
    ]);
    const onSelectTask = vi.fn();
    const onRefresh = vi.fn();
    render(
      <RoomTaskPanel
        roomID="room-1"
        tasks={tasks}
        loading={false}
        error=""
        t={t}
        onSelectTask={onSelectTask}
        onRefresh={onRefresh}
      />,
    );
    expect(screen.getByText(t("roomTaskStates.issues"))).toBeInTheDocument();
    expect(screen.queryByText("Other room task")).not.toBeInTheDocument();
    const card = screen.getByText("Room task").closest("button")!;
    fireEvent.click(card);
    expect(onSelectTask).toHaveBeenCalledWith("task-1", card);
    fireEvent.click(screen.getByRole("button", { name: t("tasksRefresh") }));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});
