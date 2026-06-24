import {
  DefaultWorkspacePaneIds,
  WorkspacePaneTypes,
  WorkspaceTabs,
  paneFromLocation,
  pathForPane,
  workspaceTabForPane,
} from "@/models/routing";

describe("task routing", () => {
  it("uses the conversation pane as the default workspace pane", () => {
    expect(paneFromLocation("/")).toEqual({ type: WorkspacePaneTypes.conversation, id: "" });
  });

  it("parses the teams route as a team pane", () => {
    expect(paneFromLocation("/teams/team-7")).toEqual({ type: WorkspacePaneTypes.team, id: "team-7" });
  });

  it("builds the teams route from a team pane", () => {
    expect(pathForPane({ type: WorkspacePaneTypes.team, id: "team-7" })).toBe("/teams/team-7");
  });

  it("parses the humans route as a human pane", () => {
    expect(paneFromLocation("/humans/u-admin")).toEqual({ type: WorkspacePaneTypes.human, id: "u-admin" });
    expect(paneFromLocation("/human/u-admin")).toEqual({ type: WorkspacePaneTypes.human, id: "u-admin" });
  });

  it("builds the humans route from a human pane", () => {
    expect(pathForPane({ type: WorkspacePaneTypes.human, id: "u-admin" })).toBe("/humans/u-admin");
  });

  it("parses model provider routes as agent-tab model panes", () => {
    expect(paneFromLocation("/models/csghub-lite")).toEqual({
      type: WorkspacePaneTypes.modelProvider,
      id: "csghub-lite",
    });
    expect(pathForPane({ type: WorkspacePaneTypes.modelProvider, id: "openai" })).toBe("/models/openai");
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.modelProvider, id: "openai" })).toBe(WorkspaceTabs.agents);
  });

  it("parses the tasks route as a task pane", () => {
    expect(paneFromLocation("/tasks/task-7")).toEqual({ type: WorkspacePaneTypes.task, id: "task-7" });
  });

  it("builds the tasks route from a task pane", () => {
    expect(pathForPane({ type: WorkspacePaneTypes.task, id: "task-7" })).toBe("/tasks/task-7");
    expect(pathForPane({ type: WorkspacePaneTypes.task })).toBe("/tasks");
  });

  it("parses hub resource routes as hub panes", () => {
    expect(paneFromLocation("/templates/builtin%2Fdemo")).toEqual({
      type: WorkspacePaneTypes.hub,
      id: "builtin/demo",
      resourceType: "template",
    });
    expect(paneFromLocation("/skills/demo-skill")).toEqual({
      type: WorkspacePaneTypes.hub,
      id: "demo-skill",
      resourceType: "skill",
    });
  });

  it("builds hub resource routes from hub panes", () => {
    expect(pathForPane({ type: WorkspacePaneTypes.hub, id: "builtin/demo", resourceType: "template" })).toBe(
      "/templates/builtin%2Fdemo",
    );
    expect(pathForPane({ type: WorkspacePaneTypes.hub, id: "demo-skill", resourceType: "skill" })).toBe(
      "/skills/demo-skill",
    );
  });

  it("maps the task pane to the tasks tab", () => {
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.team, id: "team-7" })).toBe(WorkspaceTabs.agents);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.human, id: "u-admin" })).toBe(WorkspaceTabs.agents);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.task, id: "task-7" })).toBe(WorkspaceTabs.tasks);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub })).toBe(
      WorkspaceTabs.hub,
    );
  });
});
