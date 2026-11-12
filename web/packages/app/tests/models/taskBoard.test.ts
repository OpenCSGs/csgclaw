import { agentNameLookup, displayTaskAssignmentTargetName, taskActiveWorker } from "@/models/taskBoard";
import { normalizeTaskList } from "@/models/tasks";
import { buildUsersById } from "@/models/conversations";

describe("taskBoard", () => {
  it("使用智能体及房间成员的名称并保留任务返回的名称", () => {
    const names = agentNameLookup(
      [{ id: "developer", user_id: "u-developer", name: "开发助手" }],
      buildUsersById([{ id: "u-tester", name: "测试助手" }]),
    );
    const tasks = normalizeTaskList([
      { id: "task-1", assignment_type: "agent", assignment_id: "u-developer" },
      { id: "task-2", assignment_type: "agent", assignment_id: "u-tester" },
      {
        id: "task-3",
        assignment_type: "agent",
        assignment_id: "u-developer",
        assigned_to_agent_name: "任务执行者",
      },
    ]);
    expect(Object.fromEntries(tasks.map((task) => [task.id, displayTaskAssignmentTargetName(task, names)]))).toEqual({
      "task-1": "开发助手",
      "task-2": "测试助手",
      "task-3": "任务执行者",
    });
  });

  it("显示正在执行的子任务成员并忽略已经完成的任务", () => {
    const [parent, assigned, running, completed] = normalizeTaskList([
      { id: "1", team_id: "team", assigned_to: "manager" },
      { id: "2", parent_id: "1", team_id: "team", status: "assigned", assigned_to: "tester" },
      { id: "3", parent_id: "1", team_id: "team", status: "in_progress", assigned_to: "developer" },
      { id: "4", parent_id: "1", team_id: "team", status: "completed", assigned_to: "reviewer" },
    ]);
    const names = new Map([
      ["developer", "开发助手"],
      ["tester", "测试助手"],
    ]);
    expect(taskActiveWorker(parent, [completed, assigned, running], (key) => key, names)?.name).toBe("开发助手");
    expect(taskActiveWorker(parent, [completed], (key) => key, names)).toBeNull();
  });
});
