import { describe, expect, it } from "vitest";
import { roomTaskMessageAnchors } from "@/models/roomTasks";
import type { IMMessage } from "@/models/conversations";
import { normalizeTaskList } from "@/models/tasks";
import {
  roomTaskExecutionEnded,
  roomTaskGroups,
  roomTaskParent,
  roomTaskState,
  roomTaskPath,
} from "@/models/roomTasks";

describe("room tasks", () => {
  it("only groups persisted tasks owned by this room, in stable plan order", () => {
    const tasks = normalizeTaskList([
      { id: "root", assignment_type: "room", assignment_id: "r" },
      { id: "task-10", parent_id: "root", assignment_type: "room", assignment_id: "r" },
      { id: "task-2", parent_id: "root", assignment_type: "room", assignment_id: "r" },
      { id: "outside", parent_id: "root", assignment_type: "room", assignment_id: "other" },
      { id: "direct", assignment_type: "agent", assignment_id: "r", room_id: "r" },
    ]);
    const groups = roomTaskGroups(tasks, "r");
    expect(groups).toHaveLength(1);
    expect(groups[0].children.map((child) => child.id)).toEqual(["task-2", "task-10"]);
    expect(roomTaskGroups(tasks, "empty")).toEqual([]);
  });

  it("keeps active parents first and orders each status group newest first", () => {
    const tasks = normalizeTaskList([
      {
        id: "task-34",
        assignment_type: "room",
        assignment_id: "r",
        status: "completed",
        created_at: "2026-09-11T09:00:00Z",
      },
      {
        id: "task-52",
        assignment_type: "room",
        assignment_id: "r",
        status: "in_progress",
        created_at: "2026-09-11T12:00:00Z",
      },
      {
        id: "task-39",
        assignment_type: "room",
        assignment_id: "r",
        status: "completed",
        created_at: "2026-09-11T10:00:00Z",
      },
      {
        id: "task-50",
        assignment_type: "room",
        assignment_id: "r",
        status: "completed",
        created_at: "2026-09-11T11:00:00Z",
      },
    ]);

    expect(roomTaskGroups(tasks, "r").map((group) => group.task.id)).toEqual([
      "task-52",
      "task-50",
      "task-39",
      "task-34",
    ]);
  });

  it("does not confuse execution ending with a successful goal", () => {
    const [task] = normalizeTaskList([
      { id: "root", assignment_type: "room", assignment_id: "r", status: "completed", goal_outcome: "issues" },
    ]);
    expect(roomTaskExecutionEnded(task)).toBe(true);
    expect(task.goal_outcome).toBe("issues");
  });
});

describe("room task references", () => {
  it("resolves the exact historical parent recursively without guessing the newest task", () => {
    const tasks = normalizeTaskList([
      { id: "old", assignment_type: "room", assignment_id: "r", status: "completed" },
      { id: "new", assignment_type: "room", assignment_id: "r", status: "in_progress" },
      { id: "child", parent_id: "old", assignment_type: "room", assignment_id: "r" },
      { id: "nested", parent_id: "child", assignment_type: "room", assignment_id: "r" },
      { id: "outside", parent_id: "old", assignment_type: "room", assignment_id: "other" },
      { id: "cycle", parent_id: "cycle", assignment_type: "room", assignment_id: "r" },
    ]);
    expect(roomTaskParent(tasks, "nested")?.id).toBe("old");
    expect(roomTaskParent(tasks, "missing")).toBeUndefined();
    expect(roomTaskParent(tasks, "outside")).toBeUndefined();
    expect(roomTaskParent(tasks, "cycle")).toBeUndefined();
    expect(roomTaskPath("room-1", "child/2")).toBe("/rooms/room-1?task=child%2F2");
  });

  it("requires accepted predecessors and distinguishes a parent awaiting summary", () => {
    const tasks = normalizeTaskList([
      { id: "root", assignment_type: "room", assignment_id: "r", status: "pending_review" },
      { id: "dev", parent_id: "root", assignment_type: "room", assignment_id: "r", status: "pending_review" },
      {
        id: "qa",
        parent_id: "root",
        assignment_type: "room",
        assignment_id: "r",
        status: "pending",
        depends_on: ["dev"],
      },
    ]);
    expect(roomTaskState(tasks.find((task) => task.id === "root")!, tasks)).toBe("summarizing");
    expect(roomTaskState(tasks.find((task) => task.id === "dev")!, tasks)).toBe("pending_review");
    expect(roomTaskState(tasks.find((task) => task.id === "qa")!, tasks)).toBe("waiting_dependency");
    tasks.find((task) => task.id === "dev")!.status = "completed";
    expect(roomTaskState(tasks.find((task) => task.id === "qa")!, tasks)).toBe("pending");
  });
});

it("anchors each task only to its first Manager plan message, including replayed history", () => {
  const message = (id: string, kind: string, task = "root", sender = "manager"): IMMessage => ({
    id,
    sender_id: sender,
    content: "Task update",
    created_at: id,
    metadata: { task_id: task, csgclaw: { delivery_kind: kind } },
  });
  const messages = [
    message("3", "task_planned"),
    message("2", "task_reported"),
    message("1", "task_planned"),
    message("4", "final"),
    message("5", "task_plan_updated"),
    message("6", "task_planned", "other"),
    message("0", "task_planned", "root", "worker"),
  ];
  expect([...roomTaskMessageAnchors(messages, "manager")]).toEqual([
    ["1", "root"],
    ["6", "other"],
  ]);
});
