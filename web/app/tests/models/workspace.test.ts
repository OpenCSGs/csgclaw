import { firstWorkspaceFilePath, hasWorkspaceFilePath, workspaceAncestorDirs } from "@/models/workspace";

describe("workspace helpers", () => {
  it("derives ancestor directories from workspace paths", () => {
    expect(workspaceAncestorDirs("skills/demo/SKILL.md")).toEqual(["skills", "skills/demo"]);
    expect(workspaceAncestorDirs("AGENT.md")).toEqual([]);
  });

  it("finds the first selectable file path", () => {
    const entries = [
      { path: "skills", type: "dir" },
      { path: "skills/demo", type: "dir" },
      { path: "skills/demo/SKILL.md", type: "file" },
      { path: "AGENT.md", type: "file" },
    ];

    expect(firstWorkspaceFilePath(entries)).toBe("skills/demo/SKILL.md");
    expect(hasWorkspaceFilePath(entries, "skills/demo/SKILL.md")).toBe(true);
    expect(hasWorkspaceFilePath(entries, "skills/demo")).toBe(false);
  });
});
