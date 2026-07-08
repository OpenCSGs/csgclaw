import { describe, expect, it } from "vitest";
import { WorkspacePaneTypes, WorkspaceTabs, paneFromLocation, pathForPane, workspaceTabForPane } from "./routing";

describe("workspace section routes", () => {
  it("supports the notification section without requiring a notification bot id", () => {
    const pane = paneFromLocation("/notifications");

    expect(pane).toEqual({ type: WorkspacePaneTypes.notifications, id: "" });
    expect(pathForPane(pane)).toBe("/notifications");
    expect(workspaceTabForPane(pane)).toBe(WorkspaceTabs.agents);
  });

  it("supports the teams section without requiring a team id", () => {
    const pane = paneFromLocation("/teams");

    expect(pane).toEqual({ type: WorkspacePaneTypes.team, id: "" });
    expect(pathForPane(pane)).toBe("/teams");
    expect(workspaceTabForPane(pane)).toBe(WorkspaceTabs.agents);
  });
});
