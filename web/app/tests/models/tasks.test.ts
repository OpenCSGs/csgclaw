import { groupTasksByParent, normalizeTaskList } from "@/models/tasks";

describe("tasks model", () => {
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
});
