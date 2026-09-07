import { useRef, useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import { normalizeTaskList } from "@/models/tasks";
import { RoomTaskDialog, RoomTaskReference } from "./RoomTaskDialog";

const t: TranslateFn = (key) => key;
const tasks = normalizeTaskList([
  {
    id: "old",
    title: "Portal",
    assignment_type: "room",
    assignment_id: "r",
    assigned_to: "manager",
    status: "in_progress",
  },
  {
    id: "dev",
    parent_id: "old",
    title: "Build",
    assignment_type: "room",
    assignment_id: "r",
    assigned_to: "developer",
    status: "pending_review",
    result: "Build artifact",
  },
  {
    id: "qa",
    parent_id: "old",
    title: "Test",
    assignment_type: "room",
    assignment_id: "r",
    assigned_to: "tester",
    status: "pending",
    depends_on: ["dev"],
    body: "Check the portal",
  },
  { id: "new", title: "Second goal", assignment_type: "room", assignment_id: "r", status: "queued" },
]);
function Fixture({ initial = "", error = "" }: { initial?: string; error?: string }) {
  const [selected, select] = useState(initial);
  const anchor = useRef<HTMLElement | null>(null);
  return (
    <>
      <input aria-label="Chat draft" defaultValue="Keep my draft" />
      <RoomTaskReference
        task={tasks.find((task) => task.id === "old")}
        tasks={tasks}
        t={t}
        onOpen={(element) => {
          anchor.current = element;
          select("old");
        }}
      />
      <RoomTaskDialog
        roomID="r"
        roomTitle="Development"
        taskID={selected}
        tasks={tasks}
        t={t}
        loading={false}
        error={error}
        onClose={() => select("")}
        onRestoreFocus={() => anchor.current?.focus()}
      />
    </>
  );
}
describe("RoomTaskDialog", () => {
  it("opens from a small reference, keeps the draft, closes with Escape and restores focus", async () => {
    const user = userEvent.setup();
    render(<Fixture />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const reference = screen.getByRole("button", { name: /roomTaskView.*Portal.*0\/2/ });
    await user.click(reference);
    const dialog = screen.getByRole("dialog", { name: "Portal" });
    expect(within(dialog).getByText("#old")).toBeVisible();
    expect(within(dialog).getByText("#dev")).toBeVisible();
    expect(within(dialog).getByText("#qa")).toBeVisible();
    expect(within(dialog).getByText("roomTaskStates.pending_review")).toBeVisible();
    expect(within(dialog).getByText("roomTaskStates.waiting_dependency")).toBeVisible();
    expect(within(dialog).queryByText("Build artifact")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "roomTasksAll" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Chat draft" })).toHaveValue("Keep my draft");
    expect(reference).toHaveFocus();
  });
  it("resolves child links to the parent checklist without nested details or back navigation", () => {
    render(<Fixture initial="dev" />);
    const dialog = screen.getByRole("dialog", { name: "Portal" });
    expect(within(dialog).getByText("Build")).toBeVisible();
    expect(within(dialog).getByText("Test")).toBeVisible();
    expect(within(dialog).queryByText("Build artifact")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "roomTasksAll" })).not.toBeInTheDocument();
  });
  it("uses a compact close button without a redundant refresh action", () => {
    render(<Fixture initial="old" />);
    const dialog = screen.getByRole("dialog", { name: "Portal" });
    const close = within(dialog).getByRole("button", { name: "close" });
    expect(close).toHaveClass("btn-sm", "csg-icon-button");
    expect(within(dialog).queryByRole("button", { name: "tasksRefreshShort" })).not.toBeInTheDocument();
  });
  it("shows room parents and their child states directly in the room overview", () => {
    render(<Fixture initial="list" />);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Portal")).toBeVisible();
    expect(within(dialog).getByText("Second goal")).toBeVisible();
    expect(within(dialog).getByText("Build")).toBeVisible();
    expect(within(dialog).getByText("Test")).toBeVisible();
  });
  it("shows an unavailable reference without opening an unrelated task", () => {
    render(<Fixture initial="missing" error="Offline" />);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("roomTaskNotFound")).toBeVisible();
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Offline");
    expect(within(dialog).queryByText("Portal")).not.toBeInTheDocument();
  });
});

it("shows a partial outcome in the header while leaving reports out of the checklist", () => {
  const completed = tasks.map((task) =>
    task.id === "old"
      ? { ...task, status: "completed", goal_outcome: "issues", report: "[测试报告](https://example.com/report)" }
      : task,
  );
  render(
    <RoomTaskDialog
      roomID="r"
      roomTitle="Development"
      taskID="old"
      tasks={completed}
      t={createTranslator("zh")}
      loading={false}
      error=""
      onClose={() => {}}
      onRestoreFocus={() => {}}
    />,
  );
  expect(screen.getByText("已结束（有遗留）")).toBeVisible();
  expect(screen.queryByRole("link", { name: "测试报告" })).not.toBeInTheDocument();
  expect(screen.queryByText("Manager 汇总")).not.toBeInTheDocument();
  expect(screen.queryByText("目标已达成")).not.toBeInTheDocument();
});

it("uses a compact task number without long titles or progress in the message action area", async () => {
  const onOpen = vi.fn();
  render(<RoomTaskReference compact task={tasks[0]} tasks={tasks} t={t} onOpen={onOpen} />);
  const button = screen.getByRole("button", { name: /roomTaskView/ });
  expect(button.textContent).toBe(`#${tasks[0].id}`);
  expect(button.textContent).not.toContain("0/2");
  await userEvent.click(button);
  expect(onOpen).toHaveBeenCalledWith(button);
});
