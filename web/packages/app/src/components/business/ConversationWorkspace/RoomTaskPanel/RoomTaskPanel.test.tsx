import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { normalizeTaskList } from "@/models/tasks";
import { RoomTaskPanel } from "./RoomTaskPanel";

const t = createTranslator("en");
const tasks = normalizeTaskList([
  { id: "task-1", title: "Release", assignment_type: "room", assignment_id: "r", status: "in_progress" },
  {
    id: "task-2",
    title: "Verify release",
    parent_id: "task-1",
    assignment_type: "room",
    assignment_id: "r",
    status: "completed",
  },
  { id: "task-3", title: "Next release", assignment_type: "room", assignment_id: "r", status: "queued" },
  { id: "task-4", title: "Other room", assignment_type: "room", assignment_id: "other", status: "in_progress" },
]);

describe("RoomTaskPanel", () => {
  it.each(["en", "zh"] as const)("在 %s 看板区分完成任务的遗留问题并保留详情入口", async (locale) => {
    const user = userEvent.setup();
    const translate = createTranslator(locale);
    const onSelectTask = vi.fn();
    const completedTasks = normalizeTaskList(
      ["succeeded", "issues"].flatMap((outcome) => [
        {
          id: `task-${outcome}`,
          title: `Release ${outcome}`,
          assignment_type: "room",
          assignment_id: "r",
          status: "completed",
          goal_outcome: outcome,
        },
        {
          id: `child-${outcome}`,
          title: "Verify release",
          parent_id: `task-${outcome}`,
          assignment_type: "room",
          assignment_id: "r",
          status: "completed",
        },
      ]),
    );
    render(
      <RoomTaskPanel
        roomID="r"
        tasks={completedTasks}
        loading={false}
        error=""
        onRefresh={vi.fn()}
        onSelectTask={onSelectTask}
        locale={locale}
        t={translate}
      />,
    );
    const column = screen.getByRole("region", { name: translate("taskStatus.completed") });
    const issuesCard = within(column).getByRole("button", { name: /Release issues/ });
    const succeededCard = within(column).getByRole("button", { name: /Release succeeded/ });
    expect(within(issuesCard).getByText(translate("roomTaskStates.issues"))).toBeVisible();
    expect(within(succeededCard).queryByText(translate("roomTaskStates.issues"))).not.toBeInTheDocument();
    expect(within(issuesCard).getByText("1/1")).toBeInTheDocument();
    expect(within(succeededCard).getByText("1/1")).toBeInTheDocument();
    issuesCard.focus();
    await user.keyboard("{Enter}");
    expect(onSelectTask).toHaveBeenCalledWith("task-issues", issuesCard);
  });

  it("展示当前房间的父任务并将卡片与焦点来源传给详情入口", async () => {
    const user = userEvent.setup();
    const onSelectTask = vi.fn();
    const { rerender } = render(
      <RoomTaskPanel
        roomID="r"
        tasks={tasks}
        loading={false}
        error=""
        onRefresh={vi.fn()}
        onSelectTask={onSelectTask}
        t={t}
      />,
    );
    expect(screen.queryByText("Other room")).not.toBeInTheDocument();
    expect(screen.queryByText("Verify release")).not.toBeInTheDocument();
    const column = screen.getByRole("region", { name: t("taskStatus.in_progress") });
    const card = within(column).getByRole("button", { name: /Release/ });
    expect(within(card).getByText("1/1")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: t("taskStatus.queued") })).toHaveTextContent("Next release");
    card.focus();
    await user.keyboard("{Enter}");
    expect(onSelectTask).toHaveBeenCalledWith("task-1", card);

    rerender(
      <RoomTaskPanel
        roomID="other"
        tasks={tasks}
        loading={false}
        error=""
        onRefresh={vi.fn()}
        onSelectTask={onSelectTask}
        t={t}
      />,
    );
    expect(screen.queryByText("Release")).not.toBeInTheDocument();
    expect(screen.getByText("Other room")).toBeInTheDocument();
  });

  it("读取期间禁用刷新并在失败后保留缓存任务及重试入口", async () => {
    const user = userEvent.setup();
    const onRefresh = vi.fn();
    const props = { roomID: "r", tasks, onRefresh, onSelectTask: vi.fn(), t };
    const { rerender } = render(<RoomTaskPanel {...props} loading error="" />);
    expect(screen.getByRole("status")).toHaveTextContent(t("roomPlanLoading"));
    expect(screen.getByRole("button", { name: t("tasksRefresh") })).toBeDisabled();
    rerender(<RoomTaskPanel {...props} loading={false} error="Connection failed" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Connection failed");
    expect(screen.getByText("Release")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: t("tasksRefresh") }));
    expect(onRefresh).toHaveBeenCalledOnce();
    rerender(<RoomTaskPanel {...props} loading={false} error="" refreshing />);
    expect(screen.getByRole("button", { name: t("tasksRefresh") })).toBeDisabled();
  });
});
