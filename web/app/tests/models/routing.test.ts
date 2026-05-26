import {
  DefaultWorkspacePaneIds,
  WorkspacePaneTypes,
  WorkspaceTabs,
  decodePathSegment,
  paneFromLocation,
  pathForPane,
  readCollapsedWorkspaceGroups,
  workspaceTabForPane,
} from "@/models/routing";
import { WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";

describe("routing model helpers", () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  it("parses workspace panes from paths", () => {
    expect(paneFromLocation("/computer")).toEqual({
      type: WorkspacePaneTypes.computer,
      id: DefaultWorkspacePaneIds.computer,
    });
    expect(paneFromLocation("/agents/worker%201")).toEqual({ type: WorkspacePaneTypes.agent, id: "worker 1" });
    expect(paneFromLocation("/hub")).toEqual({ type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub });
    expect(paneFromLocation("/rooms/general")).toEqual({ type: WorkspacePaneTypes.conversation, id: "general" });
    expect(paneFromLocation("/unknown")).toEqual({ type: WorkspacePaneTypes.conversation, id: "" });
  });

  it("builds paths from panes and room metadata", () => {
    const rooms = [
      { id: "dm-1", is_direct: true },
      { id: "room-1", is_direct: false },
    ];

    expect(pathForPane(null, rooms)).toBe("/computer");
    expect(pathForPane({ type: WorkspacePaneTypes.agent, id: "worker 1" }, rooms)).toBe("/agents/worker%201");
    expect(pathForPane({ type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub }, rooms)).toBe("/hub");
    expect(pathForPane({ type: WorkspacePaneTypes.conversation, id: "dm-1" }, rooms)).toBe("/dms/dm-1");
    expect(pathForPane({ type: WorkspacePaneTypes.conversation, id: "room-1" }, rooms)).toBe("/rooms/room-1");
    expect(pathForPane({ type: WorkspacePaneTypes.conversation, id: "" }, rooms)).toBe("/");
  });

  it("keeps malformed URI segments readable", () => {
    expect(decodePathSegment("worker%201")).toBe("worker 1");
    expect(decodePathSegment("%E0%A4%A")).toBe("%E0%A4%A");
  });

  it("maps panes to workspace tabs", () => {
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.hub })).toBe(WorkspaceTabs.hub);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.agent })).toBe(WorkspaceTabs.agents);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.computer })).toBe(WorkspaceTabs.agents);
    expect(workspaceTabForPane({ type: WorkspacePaneTypes.conversation })).toBe(WorkspaceTabs.messages);
  });

  it("reads collapsed groups from localStorage defensively", () => {
    expect(readCollapsedWorkspaceGroups()).toEqual({
      "direct-messages": false,
      rooms: true,
      threads: true,
    });

    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, '{"agents":true}');
    expect(readCollapsedWorkspaceGroups()).toEqual({
      "direct-messages": false,
      agents: true,
      rooms: true,
      threads: true,
    });

    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, "[]");
    expect(readCollapsedWorkspaceGroups()).toEqual({
      "direct-messages": false,
      rooms: true,
      threads: true,
    });

    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, "{bad json");
    expect(readCollapsedWorkspaceGroups()).toEqual({
      "direct-messages": false,
      rooms: true,
      threads: true,
    });
  });
});
