import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import { TasksView } from "@/pages/TasksPage/components";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTask, WorkspaceTeam, WorkspaceTeamEvent } from "@/models/tasks";

const labels: Record<string, string> = {
  mainTaskBoardTitle: "Task board",
  mainTaskBoardSubtitle: "Browse tasks",
  subTaskBoardTitle: "Sub-task board",
  taskAssigneeLabel: "Assignee",
  taskAssigneeUnassigned: "Unassigned",
  taskBoardColumnEmpty: "No tasks",
  taskBoardNoChildren: "No child tasks",
  taskCardUpdatedAt: "Updated {time}",
  taskChildrenCount: "{count} child tasks",
  taskChildrenLabel: "Child tasks",
  taskDependencyGraphLabel: "Dependency flow",
  taskDependsOnLabel: "Depends on",
  taskDescriptionLabel: "Description",
  taskExecutionChannelLabel: "Channel type",
  taskMetadataLabel: "Task info",
  taskNoDependencies: "No dependencies",
  taskOpenExecutionRoom: "Open execution room",
  taskOpenExecutionRoomShort: "Room",
  taskParentDetailTitle: "Task detail",
  taskRoomLabel: "Room",
  taskCreateSubmit: "Create",
  taskCreateSubtitle: "Create a task",
  taskCreateTitle: "New task",
  taskTimelineChildTask: "Child task",
  taskTimelineCollapse: "Collapse",
  taskTimelineCollapsedSummary: "{count} events collapsed",
  taskTimelineCreated: "Task created",
  taskTimelineCompleted: "Task completed",
  taskTimelineDispatched: "Task dispatched",
  taskTimelineAssigned: "Task assigned",
  taskTimelineClaimed: "Task claimed",
  taskTimelineEventsCount: "{count} events",
  taskTimelineExpand: "Expand",
  taskTimelineMainTask: "Task",
  taskTimelinePlanned: "Task planned",
  taskActivityEmpty: "No activity",
  taskActivityLabel: "Activity",
  taskActivityTargetLabel: "Target",
  taskActiveWorkerPlanning: "planning",
  taskActiveWorkerWorking: "working",
  taskActiveWorkerDone: "done",
  tasksActionsLabel: "Task actions",
  tasksDetailPlaceholder: "No description",
  tasksDetailLabel: "Task detail",
  tasksRefresh: "Refresh tasks",
  tasksRefreshShort: "Refresh",
  taskTitleLabel: "Title",
  taskTitlePlaceholder: "Title placeholder",
  taskStatus: "Status",
  taskTeamLabel: "Team",
  cancel: "Cancel",
  close: "Close",
};

const t: TranslateFn = (key, params = {}) => {
  if (key === "taskChildrenProgressAria") {
    return `${params.completed}/${params.total} child tasks completed`;
  }
  if (key === "taskChildrenCount") {
    return `${params.count} child tasks`;
  }
  if (key === "taskTimelineEventsCount") {
    return `${params.count} events`;
  }
  if (key === "taskTimelineCollapsedSummary") {
    return `${params.count} events collapsed`;
  }
  if (key === "taskCardUpdatedAt") {
    return `Updated ${params.time}`;
  }
  if (key.startsWith("taskStatus.")) {
    return key.replace("taskStatus.", "");
  }
  return labels[key] ?? key;
};

function task(overrides: Partial<WorkspaceTask>): WorkspaceTask {
  return {
    id: "task-1",
    team_id: "team-1",
    team_title: "te-team",
    execution_channel: "csgclaw",
    room_id: "room-1",
    room_title: "Room 1",
    parent_id: "",
    title: "Build blog",
    body: "Create the blog site",
    status: "pending",
    created_by: "manager",
    created_by_agent_name: "manager",
    assigned_to: "",
    assigned_to_agent_name: "",
    claimed_by: "",
    claimed_by_agent_name: "",
    priority: 0,
    depends_on: [],
    plan_summary: "",
    dispatched_at: "",
    result: "",
    error: "",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}

function team(overrides: Partial<WorkspaceTeam> = {}): WorkspaceTeam {
  return {
    id: "team-1",
    title: "dev-team",
    lead_agent_id: "",
    member_agent_ids: [],
    status: "active",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}

describe("TasksView", () => {
  it("shows parent tasks first and opens a parent-task dialog with grouped activity", async () => {
    const tasks = [
      task({ id: "task-1", title: "Build blog", status: "pending" }),
      task({
        id: "task-2",
        parent_id: "task-1",
        title: "Implement frontend",
        status: "completed",
        assigned_to_agent_name: "alice",
      }),
      task({
        id: "task-3",
        parent_id: "task-1",
        title: "Verify quality",
        status: "in_progress",
        assigned_to_agent_name: "bob",
        claimed_by_agent_name: "bob",
        depends_on: ["task-2"],
      }),
    ];
    const taskEvents: WorkspaceTeamEvent[] = [
      {
        seq: 1,
        team_id: "team-1",
        channel: "csgclaw",
        room_id: "room-1",
        type: "task.planned",
        actor_id: "manager",
        actor_agent_name: "manager",
        task_id: "task-1",
        target_id: "",
        target_agent_name: "",
        summary: "Plan parent",
        created_at: "2026-06-01T00:00:00Z",
      },
      {
        seq: 2,
        team_id: "team-1",
        channel: "csgclaw",
        room_id: "room-1",
        type: "task.dispatched",
        actor_id: "manager",
        actor_agent_name: "manager",
        task_id: "task-3",
        target_id: "bob",
        target_agent_name: "bob",
        summary: "Dispatch quality check",
        created_at: "2026-06-01T00:01:00Z",
      },
      {
        seq: 4,
        team_id: "team-1",
        channel: "csgclaw",
        room_id: "room-1",
        type: "task.completed",
        actor_id: "manager",
        actor_agent_name: "manager",
        task_id: "task-1",
        target_id: "",
        target_agent_name: "",
        summary: "Parent done",
        created_at: "2026-06-01T00:03:00Z",
      },
    ];

    render(<TasksView tasks={tasks} taskEvents={taskEvents} selectedTask={tasks[0]} t={t} />);

    expect(screen.getByRole("heading", { name: "Task board" })).toBeInTheDocument();
    const parentCard = screen.getByText("Build blog").closest("button");
    expect(parentCard).toHaveTextContent("1/2");
    expect(parentCard).toHaveTextContent("te-team");
    expect(parentCard).toHaveTextContent(/Updated/);
    expect(parentCard).not.toHaveTextContent(/\d{4}\.\d{2}\.\d{2}/);
    expect(screen.queryByRole("button", { name: "View details" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open execution room" })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Build blog" })).not.toBeInTheDocument();

    fireEvent.click(parentCard!);

    expect(screen.getByRole("dialog", { name: "Build blog" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Build blog" })).toBeInTheDocument();
    expect(screen.getByText("Dependency flow")).toBeInTheDocument();
    expect(screen.getAllByText("working").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Task").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Child task").length).toBeGreaterThan(0);
    expect(screen.getAllByText("task-3").length).toBeGreaterThan(0);
    expect(screen.getAllByText("task-2").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Implement frontend").length).toBeGreaterThan(0);
    const planNode = screen.getByText("Task planned");
    const childTrigger = screen.getByRole("button", { name: /Verify quality[\s\S]*Collapse/ });
    const completeNode = screen.getByText("Task completed");
    expect(planNode.compareDocumentPosition(childTrigger) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(childTrigger.compareDocumentPosition(completeNode) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    expect(await screen.findByText(/Dispatch quality check/)).toBeInTheDocument();
    expect(screen.getAllByText("Task assigned").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Task claimed").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Target: bob").length).toBeGreaterThan(0);

    fireEvent.click(childTrigger);
    expect(screen.queryByText("Target: bob")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Verify quality[\s\S]*Expand/ }));
    expect(screen.getByText(/Dispatch quality check/)).toBeInTheDocument();
  });

  it("creates parent tasks with the CSGClaw channel without exposing channel selection", async () => {
    const onCreateTask = vi.fn();

    render(<TasksView showCreateTaskModal teams={[team()]} onCreateTask={onCreateTask} t={t} />);

    expect(screen.getByRole("dialog", { name: "New task" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Channel type")).not.toBeInTheDocument();

    fireEvent.input(screen.getByLabelText("Title"), { target: { value: "Ship review" } });
    fireEvent.input(screen.getByLabelText("Description"), { target: { value: "Review the release" } });

    const submit = screen.getByRole("button", { name: "Create" });
    await waitFor(() => expect(submit).not.toBeDisabled());
    fireEvent.click(submit);

    await waitFor(() =>
      expect(onCreateTask).toHaveBeenCalledWith({
        team_id: "team-1",
        title: "Ship review",
        body: "Review the release",
        execution_channel: "csgclaw",
      }),
    );
  });

  it("keeps the task board visible when there are no tasks", () => {
    render(<TasksView tasks={[]} t={t} />);

    expect(screen.getByRole("heading", { name: "Task board" })).toBeInTheDocument();
    expect(screen.getAllByText("No tasks").length).toBeGreaterThan(0);
    expect(screen.queryByText("tasksEmpty")).not.toBeInTheDocument();
  });

  it("does not show planner as the current worker after a parent task is completed", () => {
    const tasks = [
      task({
        id: "task-1",
        title: "Build blog",
        status: "completed",
        assigned_to_agent_name: "planner",
      }),
      task({
        id: "task-2",
        parent_id: "task-1",
        title: "Implement frontend",
        status: "completed",
        assigned_to_agent_name: "alice",
      }),
    ];

    render(<TasksView tasks={tasks} selectedTask={tasks[0]} t={t} />);

    fireEvent.click(screen.getByText("Build blog").closest("button")!);

    expect(screen.getByRole("dialog", { name: "Build blog" })).toBeInTheDocument();
    expect(screen.queryByTitle("planner done")).not.toBeInTheDocument();
  });
});
