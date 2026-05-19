import {
  decodePathSegment,
  paneFromLocation,
  pathForPane,
  readCollapsedWorkspaceGroups,
  workspaceTabForPane,
} from "@/models/routing";
import { WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";

describe("routing model helpers", () => {
  it("parses workspace panes from paths", () => {
    expect(paneFromLocation("/computer")).toEqual({ type: "computer", id: "local" });
    expect(paneFromLocation("/agents/worker%201")).toEqual({ type: "agent", id: "worker 1" });
    expect(paneFromLocation("/hub")).toEqual({ type: "hub", id: "hub" });
    expect(paneFromLocation("/rooms/general")).toEqual({ type: "conversation", id: "general" });
    expect(paneFromLocation("/unknown")).toEqual({ type: "conversation", id: "" });
  });

  it("builds paths from panes and room metadata", () => {
    const rooms = [
      { id: "dm-1", is_direct: true },
      { id: "room-1", is_direct: false },
    ];

    expect(pathForPane(null, rooms)).toBe("/computer");
    expect(pathForPane({ type: "agent", id: "worker 1" }, rooms)).toBe("/agents/worker%201");
    expect(pathForPane({ type: "hub", id: "hub" }, rooms)).toBe("/hub");
    expect(pathForPane({ type: "conversation", id: "dm-1" }, rooms)).toBe("/dms/dm-1");
    expect(pathForPane({ type: "conversation", id: "room-1" }, rooms)).toBe("/rooms/room-1");
    expect(pathForPane({ type: "conversation", id: "" }, rooms)).toBe("/");
  });

  it("keeps malformed URI segments readable", () => {
    expect(decodePathSegment("worker%201")).toBe("worker 1");
    expect(decodePathSegment("%E0%A4%A")).toBe("%E0%A4%A");
  });

  it("maps panes to workspace tabs", () => {
    expect(workspaceTabForPane({ type: "hub" })).toBe("hub");
    expect(workspaceTabForPane({ type: "agent" })).toBe("agents");
    expect(workspaceTabForPane({ type: "computer" })).toBe("agents");
    expect(workspaceTabForPane({ type: "conversation" })).toBe("messages");
  });

  it("reads collapsed groups from localStorage defensively", () => {
    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, '{"agents":true}');
    expect(readCollapsedWorkspaceGroups()).toEqual({ agents: true });

    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, "[]");
    expect(readCollapsedWorkspaceGroups()).toEqual({});

    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, "{bad json");
    expect(readCollapsedWorkspaceGroups()).toEqual({});
  });
});
