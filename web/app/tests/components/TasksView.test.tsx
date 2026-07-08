import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { vi } from "vitest";
import { TasksView } from "@/pages/TasksPage/components";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceScheduledTask, WorkspaceScheduledTaskRun } from "@/models/scheduledTasks";
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
  taskClaimedByLabel: "Claimed by",
  taskDependencyGraphLabel: "Dependency flow",
  taskDependsOnLabel: "Depends on",
  taskDescriptionLabel: "Description",
  taskDescriptionPlaceholder: "Optional: add background, target outcome, scope, and acceptance notes",
  taskExecutionChannelLabel: "Channel type",
  taskMetadataLabel: "Task info",
  taskNoDependencies: "No dependencies",
  taskOpenExecutionRoom: "Open execution room",
  taskOpenExecutionRoomShort: "Room",
  taskOpenConversation: "View history",
  taskConversationAgentDeleted: "Cannot open history: the corresponding agent was deleted.",
  taskParentDetailTitle: "Task detail",
  taskRoomLabel: "Room",
  taskCreate: "New task",
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
  taskTitlePlaceholder: "Required: add a task title",
  taskTitleRequired: "Title is required.",
  taskStatus: "Status",
  taskAssignmentLabel: "Assign to",
  taskAssignmentTeamGroup: "Teams",
  taskAssignmentAgentGroup: "Agents",
  taskAssignmentPlaceholder: "Choose an assignee",
  taskAssignmentRequired: "Choose an assignee.",
  taskTeamLabel: "Team",
  scheduledTasksTab: "Scheduled",
  scheduledTasksEmpty: "No scheduled tasks yet.",
  scheduledTaskCreate: "New scheduled task",
  scheduledTaskCreateSubmit: "Save",
  scheduledTaskCreateTitle: "New scheduled task",
  scheduledTaskCreateSubtitle: "Create scheduled task",
  scheduledTaskEdit: "Edit",
  scheduledTaskDelete: "Delete",
  scheduledTaskDeleteConfirmMessage: 'Delete scheduled task "{title}"? This action cannot be undone.',
  scheduledTaskEditTitle: "Edit scheduled task",
  scheduledTaskEditSubtitle: "Edit scheduled task",
  scheduledTaskSaveChanges: "Save changes",
  scheduledTaskAgentLabel: "Assign to",
  scheduledTaskAgentPlaceholder: "Choose an agent",
  scheduledTaskAgentRequired: "Choose an agent.",
  scheduledTaskPromptLabel: "Prompt",
  scheduledTaskPromptPlaceholder: "Prompt sent to the agent",
  scheduledTaskPromptRequired: "Prompt is required.",
  scheduledTaskRecurrenceLabel: "Schedule",
  scheduledTaskRecurrenceOnce: "Does not repeat",
  scheduledTaskRecurrenceDaily: "Daily",
  scheduledTaskRecurrenceWeekly: "Weekly",
  scheduledTaskRecurrenceMonthly: "Monthly",
  scheduledTaskDateLabel: "Date",
  scheduledTaskDateRequired: "Choose a date.",
  scheduledTaskTimeLabel: "Time",
  scheduledTaskTimeRequired: "Choose a time.",
  scheduledTaskExpiresLabel: "End date (optional)",
  scheduledTaskNextRunLabel: "Next run",
  scheduledTaskLastRunLabel: "Last run",
  scheduledTaskRunsTitle: "Run history",
  scheduledTaskRunsEmpty: "No runs yet.",
  scheduledTaskRunNow: "Run now",
  scheduledTaskEnable: "Enable",
  scheduledTaskDisable: "Disable",
  scheduledTaskCompleted: "Completed",
  scheduledTaskActiveTask: "Task running",
  scheduledTaskRunTriggeredStatus: "Triggered",
  scheduledTaskRunFailedStatus: "Failed",
  generatedTaskDetailTitle: "Generated task detail",
  generatedTaskDetailEmpty: "Select a run record to inspect the generated task.",
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
  if (key === "scheduledTaskDeleteConfirmMessage") {
    return `Delete scheduled task "${params.title}"? This action cannot be undone.`;
  }
  if (key.startsWith("taskStatus.")) {
    return key.replace("taskStatus.", "");
  }
  return labels[key] ?? key;
};

function task(overrides: Partial<WorkspaceTask>): WorkspaceTask {
  return {
    id: "task-1",
    assignment_type: "team",
    assignment_id: "team-1",
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

function agent(overrides: Partial<AgentLike> = {}): AgentLike {
  return {
    id: "agent-1",
    name: "Worker One",
    role: "worker",
    ...overrides,
  };
}

function scheduledTask(overrides: Partial<WorkspaceScheduledTask> = {}): WorkspaceScheduledTask {
  return {
    id: "scheduled-task-1",
    title: "Morning report",
    agent_id: "agent-1",
    prompt: "Summarize overnight progress",
    recurrence: "daily",
    enabled: true,
    next_run_at: "2026-07-07T09:30:00",
    last_run_at: "",
    expires_at: "2026-07-31T23:59:00",
    created_at: "2026-07-01T00:00:00",
    updated_at: "2026-07-01T00:00:00",
    ...overrides,
  };
}

function scheduledTaskRun(overrides: Partial<WorkspaceScheduledTaskRun> = {}): WorkspaceScheduledTaskRun {
  return {
    id: "scheduled-run-1",
    scheduled_task_id: "scheduled-task-1",
    triggered_at: "2026-07-07T10:00:00",
    status: "triggered",
    task_id: "task-scheduled-run-1",
    error: "",
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

  it("hides unresolved technical ids from parent task metadata", () => {
    const parent = task({
      id: "task-1",
      assignment_id: "picoclaw",
      team_id: "picoclaw",
      team_title: "",
      room_id: "picoclaw",
      room_title: "",
      status: "completed",
      assigned_to: "picoclaw",
      assigned_to_agent_name: "",
      claimed_by: "picoclaw",
      claimed_by_agent_name: "",
    });

    render(<TasksView tasks={[parent]} t={t} />);

    fireEvent.click(screen.getByText("Build blog").closest("button")!);

    const metadata = screen.getByRole("complementary", { name: "Task info" });
    expect(within(metadata).queryByText("Assignee")).not.toBeInTheDocument();
    expect(within(metadata).queryByText("Claimed by")).not.toBeInTheDocument();
    expect(within(metadata).queryByText("Assign to")).not.toBeInTheDocument();
    expect(within(metadata).queryByText("Room")).not.toBeInTheDocument();
    expect(within(metadata).queryByText("picoclaw")).not.toBeInTheDocument();
  });

  it("creates parent tasks with the CSGClaw channel without exposing channel selection", async () => {
    const onCreateTask = vi.fn();

    render(<TasksView showCreateTaskModal teams={[team()]} onCreateTask={onCreateTask} t={t} />);

    expect(screen.getByRole("dialog", { name: "New task" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Channel type")).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Required/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Optional/)).toBeInTheDocument();

    fireEvent.input(screen.getByLabelText("Title"), { target: { value: "Ship review" } });
    fireEvent.input(screen.getByLabelText("Description"), { target: { value: "Review the release" } });
    fireEvent.click(screen.getByRole("combobox", { name: "Assign to" }));
    fireEvent.click(await screen.findByRole("option", { name: /dev-team/ }));

    const submit = screen.getByRole("button", { name: "Create" });
    await waitFor(() => expect(submit).not.toBeDisabled());
    fireEvent.click(submit);

    await waitFor(() =>
      expect(onCreateTask).toHaveBeenCalledWith({
        assignment_id: "team-1",
        assignment_type: "team",
        team_id: "team-1",
        title: "Ship review",
        body: "Review the release",
        execution_channel: "csgclaw",
      }),
    );
  });

  it("requires title and assignment without generating a title from description", async () => {
    const onCreateTask = vi.fn();

    render(<TasksView showCreateTaskModal teams={[team()]} onCreateTask={onCreateTask} t={t} />);

    const submit = screen.getByRole("button", { name: "Create" });
    expect(submit).not.toBeDisabled();

    fireEvent.click(submit);

    expect(await screen.findByText("Title is required.")).toBeInTheDocument();
    expect(screen.getByText("Choose an assignee.")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Required: add a task title")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("combobox", { name: "Assign to" })).toHaveAttribute("aria-invalid", "true");
    expect(onCreateTask).not.toHaveBeenCalled();

    fireEvent.input(screen.getByPlaceholderText("Required: add a task title"), {
      target: { value: "Review Beta 1 release readiness" },
    });
    fireEvent.input(
      screen.getByPlaceholderText("Optional: add background, target outcome, scope, and acceptance notes"),
      {
        target: { value: "Review Beta 1 release readiness.\nCheck acceptance criteria." },
      },
    );
    fireEvent.click(screen.getByRole("combobox", { name: "Assign to" }));
    fireEvent.click(await screen.findByRole("option", { name: /dev-team/ }));
    expect(screen.queryByText("Title is required.")).not.toBeInTheDocument();
    expect(screen.queryByText("Choose an assignee.")).not.toBeInTheDocument();

    fireEvent.click(submit);

    await waitFor(() =>
      expect(onCreateTask).toHaveBeenCalledWith({
        assignment_id: "team-1",
        assignment_type: "team",
        team_id: "team-1",
        title: "Review Beta 1 release readiness",
        body: "Review Beta 1 release readiness.\nCheck acceptance criteria.",
        execution_channel: "csgclaw",
      }),
    );
  });

  it("creates a task with an explicit title and optional description omitted", async () => {
    const onCreateTask = vi.fn();

    render(<TasksView showCreateTaskModal teams={[team()]} onCreateTask={onCreateTask} t={t} />);

    fireEvent.input(screen.getByLabelText("Title"), { target: { value: "Ship review" } });
    fireEvent.click(screen.getByRole("combobox", { name: "Assign to" }));
    fireEvent.click(await screen.findByRole("option", { name: /dev-team/ }));

    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(onCreateTask).toHaveBeenCalledWith({
        assignment_id: "team-1",
        assignment_type: "team",
        team_id: "team-1",
        title: "Ship review",
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

  it("shows task and scheduled-task creation as tabs in the create dialog", () => {
    render(<TasksView showCreateTaskModal agents={[agent()]} teams={[team()]} t={t} />);

    expect(screen.getByRole("dialog", { name: "New task" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "New task" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "New scheduled task" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "New scheduled task" }));

    expect(screen.getByRole("dialog", { name: "New scheduled task" })).toBeInTheDocument();
    expect(screen.getByLabelText("Prompt")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Schedule" })).toBeInTheDocument();
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

  it("edits an existing scheduled task from the detail pane", async () => {
    const onOpenEditScheduledTaskModal = vi.fn();
    const onEditScheduledTask = vi.fn();
    const agents = [agent(), agent({ id: "agent-2", name: "Worker Two" })];
    const item = scheduledTask();

    const view = (
      <TasksView
        agents={agents}
        scheduledTasks={[item]}
        selectedScheduledTaskID={item.id}
        activeView="scheduled"
        onOpenEditScheduledTaskModal={onOpenEditScheduledTaskModal}
        onEditScheduledTask={onEditScheduledTask}
        t={t}
      />
    );

    const { rerender } = render(view);

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    expect(onOpenEditScheduledTaskModal).toHaveBeenCalledWith(item.id);

    rerender(<TasksView {...view.props} editingScheduledTaskID={item.id} />);

    expect(screen.getByRole("dialog", { name: "Edit scheduled task" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("Morning report")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Summarize overnight progress")).toBeInTheDocument();
    expect(screen.getByLabelText("Date")).toHaveValue("2026-07-07");
    expect(screen.getByLabelText("Time")).toHaveValue("09:30");
    expect(screen.getByLabelText("End date (optional)")).toHaveValue("2026-07-31");

    fireEvent.input(screen.getByLabelText("Title"), { target: { value: "Updated report" } });
    fireEvent.input(screen.getByLabelText("Prompt"), { target: { value: "Write the updated report" } });
    fireEvent.click(screen.getByRole("combobox", { name: "Assign to" }));
    fireEvent.click(await screen.findByRole("option", { name: /Worker Two/ }));
    fireEvent.click(screen.getByRole("combobox", { name: "Schedule" }));
    fireEvent.click(await screen.findByRole("option", { name: "Weekly" }));
    fireEvent.input(screen.getByLabelText("Date"), { target: { value: "2026-07-08" } });
    fireEvent.input(screen.getByLabelText("Time"), { target: { value: "10:15" } });
    fireEvent.input(screen.getByLabelText("End date (optional)"), { target: { value: "2026-08-01" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(onEditScheduledTask).toHaveBeenCalledWith(item.id, {
        title: "Updated report",
        agent_id: "agent-2",
        prompt: "Write the updated report",
        recurrence: "weekly",
        next_run_at: new Date("2026-07-08T10:15:00").toISOString(),
        expires_at: new Date("2026-08-01T23:59:00").toISOString(),
        enabled: true,
      }),
    );
  });

  it("reactivates a completed one-time scheduled task when editing in a new run time", async () => {
    const onEditScheduledTask = vi.fn();
    const agents = [agent()];
    const item = scheduledTask({
      enabled: false,
      next_run_at: "",
      expires_at: "",
      recurrence: "once",
    });

    render(
      <TasksView
        agents={agents}
        scheduledTasks={[item]}
        selectedScheduledTaskID={item.id}
        editingScheduledTaskID={item.id}
        onEditScheduledTask={onEditScheduledTask}
        t={t}
      />,
    );

    fireEvent.input(screen.getByLabelText("Date"), { target: { value: "2026-07-08" } });
    fireEvent.input(screen.getByLabelText("Time"), { target: { value: "10:15" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(onEditScheduledTask).toHaveBeenCalledWith(
        item.id,
        expect.objectContaining({
          next_run_at: new Date("2026-07-08T10:15:00").toISOString(),
          enabled: true,
        }),
      ),
    );
  });

  it("reactivates a disabled scheduled task when editing to a new run time", async () => {
    const onEditScheduledTask = vi.fn();
    const agents = [agent()];
    const item = scheduledTask({
      enabled: false,
      next_run_at: "2026-07-07T09:30:00Z",
      expires_at: "",
      recurrence: "daily",
    });

    render(
      <TasksView
        agents={agents}
        scheduledTasks={[item]}
        selectedScheduledTaskID={item.id}
        editingScheduledTaskID={item.id}
        onEditScheduledTask={onEditScheduledTask}
        t={t}
      />,
    );

    fireEvent.input(screen.getByLabelText("Time"), { target: { value: "10:15" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(onEditScheduledTask).toHaveBeenCalledWith(
        item.id,
        expect.objectContaining({
          next_run_at: new Date("2026-07-07T10:15:00").toISOString(),
          enabled: true,
        }),
      ),
    );
  });

  it("allows daily scheduled task edits with only a time", async () => {
    const onEditScheduledTask = vi.fn();
    const agents = [agent()];
    const item = scheduledTask({
      enabled: false,
      next_run_at: "",
      expires_at: "",
      recurrence: "once",
    });
    const now = new Date();
    const target = new Date(now.getTime() + 5 * 60 * 1000);
    const targetTime = `${String(target.getHours()).padStart(2, "0")}:${String(target.getMinutes()).padStart(2, "0")}`;
    const expected = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate(),
      target.getHours(),
      target.getMinutes(),
      0,
      0,
    );
    if (expected.getTime() <= now.getTime()) {
      expected.setDate(expected.getDate() + 1);
    }

    render(
      <TasksView
        agents={agents}
        scheduledTasks={[item]}
        selectedScheduledTaskID={item.id}
        editingScheduledTaskID={item.id}
        onEditScheduledTask={onEditScheduledTask}
        t={t}
      />,
    );

    fireEvent.click(screen.getByRole("combobox", { name: "Schedule" }));
    fireEvent.click(await screen.findByRole("option", { name: "Daily" }));
    fireEvent.input(screen.getByLabelText("Time"), { target: { value: targetTime } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(screen.queryByText("Choose a date.")).not.toBeInTheDocument();
    expect(onEditScheduledTask).toHaveBeenCalledWith(
      item.id,
      expect.objectContaining({
        recurrence: "daily",
        next_run_at: expected.toISOString(),
        enabled: true,
      }),
    );
  });

  it("shows a scheduled run task inline instead of opening the task dialog", () => {
    const generatedTask = task({
      id: "task-scheduled-run-1",
      title: "Generated report",
      body: "Generated task body",
      assignment_type: "agent",
      assignment_id: "agent-1",
      agent_id: "agent-1",
      team_id: "",
      created_by: "scheduler",
      status: "completed",
    } as Partial<WorkspaceTask>);

    render(
      <TasksView
        activeView="scheduled"
        tasks={[generatedTask]}
        scheduledTasks={[scheduledTask()]}
        scheduledTaskRuns={[scheduledTaskRun()]}
        selectedScheduledTaskID="scheduled-task-1"
        t={t}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "task-scheduled-run-1" }));

    expect(screen.queryByRole("dialog", { name: "Generated report" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Generated task detail")).toBeInTheDocument();
    expect(screen.getByText("Generated report")).toBeInTheDocument();
    expect(screen.getByText("Generated task body")).toBeInTheDocument();
  });

  it("warns instead of navigating when a scheduled run room no longer exists", () => {
    const onOpenConversation = vi.fn();
    const generatedTask = task({
      id: "task-scheduled-run-1",
      title: "Generated report",
      assignment_type: "agent",
      assignment_id: "agent-1",
      agent_id: "agent-1",
      room_id: "room-deleted-agent",
      status: "completed",
    } as Partial<WorkspaceTask>);

    render(
      <TasksView
        activeView="scheduled"
        tasks={[generatedTask]}
        rooms={[]}
        scheduledTasks={[scheduledTask()]}
        scheduledTaskRuns={[scheduledTaskRun()]}
        selectedScheduledTaskID="scheduled-task-1"
        onOpenConversation={onOpenConversation}
        t={t}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "View history" }));

    expect(onOpenConversation).not.toHaveBeenCalled();
    expect(screen.getByText("Cannot open history: the corresponding agent was deleted.")).toBeInTheDocument();
  });

  it("keeps the generated task detail area reserved before a run is selected", () => {
    render(
      <TasksView
        activeView="scheduled"
        scheduledTasks={[scheduledTask()]}
        selectedScheduledTaskID="scheduled-task-1"
        t={t}
      />,
    );

    expect(screen.getByLabelText("Generated task detail")).toBeInTheDocument();
    expect(screen.getByText("Select a run record to inspect the generated task.")).toBeInTheDocument();
  });

  it("disables run now as soon as a scheduled run is triggered", () => {
    render(
      <TasksView
        activeView="scheduled"
        scheduledTasks={[scheduledTask()]}
        scheduledTaskRuns={[scheduledTaskRun()]}
        selectedScheduledTaskID="scheduled-task-1"
        t={t}
      />,
    );

    expect(screen.getByRole("button", { name: /Task running/ })).toBeDisabled();
  });

  it("confirms before deleting a scheduled task", async () => {
    const onDeleteScheduledTask = vi.fn();
    const item = scheduledTask();

    render(
      <TasksView
        scheduledTasks={[item]}
        selectedScheduledTaskID={item.id}
        activeView="scheduled"
        onDeleteScheduledTask={onDeleteScheduledTask}
        t={t}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    expect(screen.getByRole("dialog", { name: "Delete" })).toBeInTheDocument();
    expect(
      screen.getByText('Delete scheduled task "Morning report"? This action cannot be undone.'),
    ).toBeInTheDocument();
    expect(onDeleteScheduledTask).not.toHaveBeenCalled();

    const deleteButtons = screen.getAllByRole("button", { name: "Delete" });
    fireEvent.click(deleteButtons[deleteButtons.length - 1]);

    await waitFor(() => expect(onDeleteScheduledTask).toHaveBeenCalledWith(item.id));
  });
});
