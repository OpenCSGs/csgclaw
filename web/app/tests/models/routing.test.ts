import {
  DefaultWorkspacePaneIds,
  WorkspacePaneTypes,
  WorkspaceTabs,
  paneFromLocation,
  pathForPane,
  workspaceTabForPane,
} from "@/models/routing";

describe("task routing", () => {
  it("parses the teams route as a team pane", () => {
    expect(paneFromLocation("/teams/team-7")).toEqual({ type: WorkspacePaneTypes.team, id: "team-7" });
  });

  it("builds the teams route from a team pane", () => {
    expect(pathForPane({ type: WorkspacePaneTypes.team, id: "team-7" })).toBe("/teams/team-7");
  });

  it("parses the tasks route as a task pane", () => {
    expect(paneFromLocation("/tasks/task-7")).toEqual({ type: WorkspacePaneTypes.task, id: "task-7" });
  });

  it("builds the tasks route from a task pane", () => {
    expect(pathForPane({ type: WorkspacePaneTypes.task, id: "task-7" })).toBe("/tasks/task-7");
    expect(pathForPane({ type: WorkspacePaneTypes.task })).toBe("/tasks");
  });

  it("maps the task pane to the tasks tab", () => {
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.team, id: "team-7" })).toBe(WorkspaceTabs.agents);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.task, id: "task-7" })).toBe(WorkspaceTabs.tasks);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub })).toBe(
      WorkspaceTabs.hub,
    );
  });
});
