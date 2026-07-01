import {
  boardColumnsForTask,
  formatTaskUpdatedAt,
  formatTaskUpdatedRelative,
  groupTasksByParent,
  normalizeTaskList,
  normalizeTeamEventList,
  normalizeTeamList,
  resolveTaskSidebarPhase,
  rootTasks,
  taskChildren,
  taskExecutionRoomID,
  taskUsesExecutionRoom,
} from "@/models/tasks";

describe("tasks model", () => {
  it("formats task timestamps in locale-neutral numeric form", () => {
    const value = "2026-06-04T13:13:00Z";
    const formatted = formatTaskUpdatedAt(value, "en");
    expect(formatted).toMatch(/^\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}$/);
    expect(formatTaskUpdatedAt(value, "zh")).toBe(formatted);
    expect(formatTaskUpdatedAt("", "en")).toBe("-");
    expect(formatTaskUpdatedAt("invalid", "en")).toBe("-");
  });

  it("formats task timestamps as relative time for cards", () => {
    const now = new Date("2026-06-04T13:43:00Z");

    expect(formatTaskUpdatedRelative("2026-06-04T13:13:00Z", "en", now)).toBe("30 minutes ago");
    expect(formatTaskUpdatedRelative("", "en", now)).toBe("-");
    expect(formatTaskUpdatedRelative("invalid", "en", now)).toBe("-");
  });

  it("normalizes participant display names returned by task APIs", () => {
    const tasks = normalizeTaskList([
      {
        id: "task-1",
        team_id: "team-1",
        room_id: "room-1",
        title: "Fetch weather",
        created_by: "manager",
        created_by_agent_name: "manager",
        assigned_to: "agent-ymkx7q",
        assigned_to_agent_name: "data-2-worker",
        claimed_by: "agent-ymkx7q",
        claimed_by_agent_name: "data-2-worker",
      },
    ]);
    expect(tasks[0]).toMatchObject({
      created_by: "manager",
      created_by_agent_name: "manager",
      assigned_to: "agent-ymkx7q",
      assigned_to_agent_name: "data-2-worker",
      claimed_by: "agent-ymkx7q",
      claimed_by_agent_name: "data-2-worker",
    });

    const events = normalizeTeamEventList([
      {
        seq: 1,
        team_id: "team-1",
        type: "task.dispatched",
        actor_id: "manager",
        actor_agent_name: "manager",
        target_id: "agent-ymkx7q",
        target_agent_name: "data-2-worker",
      },
    ]);
    expect(events[0]).toMatchObject({
      actor_id: "manager",
      actor_agent_name: "manager",
      target_id: "agent-ymkx7q",
      target_agent_name: "data-2-worker",
    });
  });

  it("normalizes team lead agent ids", () => {
    const teams = normalizeTeamList([
      {
        id: "team-1",
        lead_agent_id: "u-manager",
        member_agent_ids: ["u-worker"],
      },
      {
        id: "team-2",
        lead_agent_id: "u-other-manager",
      },
    ]);

    expect(teams.map((team) => team.lead_agent_id)).toEqual(["u-manager", "u-other-manager"]);
    expect(teams[0]?.member_agent_ids).toEqual(["u-worker"]);
    expect(teams[1]?.member_agent_ids).toEqual([]);
  });

  it("normalizes parent ids and groups child tasks under their parent", () => {
    const tasks = normalizeTaskList([
      {
        id: "task-1",
        team_id: "team-1",
        room_id: "room-1",
        title: "Release v1",
        updated_at: "2026-05-30T10:00:00Z",
      },
      {
        id: "task-2",
        team_id: "team-1",
        room_id: "room-1",
        parent_id: "task-1",
        title: "Draft release note",
        updated_at: "2026-05-30T09:00:00Z",
      },
      {
        id: "task-3",
        team_id: "team-1",
        room_id: "room-1",
        parent_id: "task-1",
        title: "Smoke test",
        updated_at: "2026-05-30T08:00:00Z",
      },
    ]);

    expect(tasks[1]?.parent_id).toBe("task-1");

    const groups = groupTasksByParent(tasks);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.task.id).toBe("task-1");
    expect(groups[0]?.children.map((item) => item.id)).toEqual(["task-2", "task-3"]);
  });

  it("treats orphan child tasks as top-level groups", () => {
    const tasks = normalizeTaskList([
      {
        id: "task-2",
        team_id: "team-1",
        room_id: "room-1",
        parent_id: "missing",
        title: "Standalone after reload",
        updated_at: "2026-05-30T09:00:00Z",
      },
    ]);

    const groups = groupTasksByParent(tasks);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.task.id).toBe("task-2");
    expect(groups[0]?.children).toEqual([]);
  });

  it("counts only parent tasks and groups child tasks into board columns", () => {
    const tasks = normalizeTaskList([
      {
        id: "task-1",
        team_id: "team-1",
        room_id: "room-1",
        title: "Beta 1",
        updated_at: "2026-05-30T10:00:00Z",
      },
      {
        id: "task-2",
        team_id: "team-1",
        room_id: "room-1",
        parent_id: "task-1",
        status: "in_progress",
        title: "Build board",
        updated_at: "2026-05-30T09:00:00Z",
      },
      {
        id: "task-3",
        team_id: "team-1",
        room_id: "room-1",
        parent_id: "task-1",
        status: "blocked",
        title: "Review copy",
        updated_at: "2026-05-30T08:00:00Z",
      },
    ]);

    expect(rootTasks(tasks).map((item) => item.id)).toEqual(["task-1"]);

    const columns = boardColumnsForTask(tasks, "task-1");
    expect(columns.find((column) => column.status === "in_progress")?.tasks.map((item) => item.id)).toEqual(["task-2"]);
    expect(columns.find((column) => column.status === "blocked")?.tasks.map((item) => item.id)).toEqual(["task-3"]);
    expect(columns.find((column) => column.status === "completed")?.tasks).toEqual([]);
  });

  it("resolves execution room directly from the parent task", () => {
    const teams = normalizeTeamList([
      {
        id: "team-1",
        title: "Team",
      },
    ]);
    const tasks = normalizeTaskList([
      {
        id: "task-1",
        team_id: "team-1",
        execution_channel: "csgclaw",
        room_id: "room-exec",
        title: "Parent",
        updated_at: "2026-05-30T10:00:00Z",
      },
      {
        id: "task-2",
        team_id: "team-1",
        room_id: "room-exec",
        parent_id: "task-1",
        title: "Child",
        updated_at: "2026-05-30T09:00:00Z",
      },
    ]);
    const parent = tasks.find((task) => task.id === "task-1");
    expect(parent).toBeTruthy();
    const children = taskChildren(tasks, "task-1");

    expect(taskExecutionRoomID(parent!, children, teams)).toBe("room-exec");
    expect(taskUsesExecutionRoom(parent!, teams, children)).toBe(true);
  });

  it("resolves sidebar phases for planning and dispatching parent tasks", () => {
    const planningTask = normalizeTaskList([
      {
        id: "task-1",
        team_id: "team-1",
        room_id: "room-1",
        status: "pending",
        title: "Planning",
      },
    ])[0];
    expect(resolveTaskSidebarPhase(planningTask, [])).toBe("planning");
    expect(resolveTaskSidebarPhase(planningTask, [], { planningTaskID: "task-1" })).toBe("planning");

    const dispatchTasks = normalizeTaskList([
      {
        id: "task-1",
        team_id: "team-1",
        room_id: "room-1",
        status: "pending",
        plan_summary: "Split work",
        title: "Parent",
      },
      {
        id: "task-2",
        team_id: "team-1",
        room_id: "room-1",
        parent_id: "task-1",
        status: "pending",
        assigned_to: "bot-alice",
        title: "Child",
      },
    ]);
    const parent = dispatchTasks[0];
    const children = taskChildren(dispatchTasks, "task-1");
    expect(resolveTaskSidebarPhase(parent, children)).toBe("idle");
    expect(resolveTaskSidebarPhase(parent, children, { startingTaskID: "task-1" })).toBe("dispatching");

    const dispatchedChild = {
      ...children[0],
      status: "assigned",
      dispatched_at: "2026-05-30T10:00:00Z",
    };
    expect(resolveTaskSidebarPhase(parent, [dispatchedChild])).toBe("idle");
  });

  it("normalizes team events in sequence order", () => {
    const events = normalizeTeamEventList([
      {
        seq: 2,
        team_id: "team-1",
        type: "task.completed",
        task_id: "task-2",
        summary: "done",
      },
      {
        seq: 1,
        team_id: "team-1",
        type: "task.created",
        task_id: "task-2",
        summary: "Draft release note",
      },
      {
        type: "task.created",
      },
    ]);

    expect(events.map((item) => item.type)).toEqual(["task.created", "task.completed"]);
    expect(events[0]).toMatchObject({
      seq: 1,
      team_id: "team-1",
      task_id: "task-2",
      summary: "Draft release note",
    });
  });
});
