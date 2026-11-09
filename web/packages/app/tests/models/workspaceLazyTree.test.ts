import { describe, expect, it } from "vitest";
import { flattenWorkspaceDirectoryListings } from "@/models/workspace";

describe("flattenWorkspaceDirectoryListings", () => {
  it("places loaded children directly after their directory", () => {
    expect(
      flattenWorkspaceDirectoryListings({
        "": [
          { path: "skills", name: "skills", type: "dir" },
          { path: "USER.md", name: "USER.md", type: "file" },
        ],
        skills: [
          { path: "skills/custom", name: "custom", type: "dir" },
          { path: "skills/README.md", name: "README.md", type: "file" },
        ],
        "skills/custom": [{ path: "skills/custom/SKILL.md", name: "SKILL.md", type: "file" }],
      }).map((entry) => entry.path),
    ).toEqual(["skills", "skills/custom", "skills/custom/SKILL.md", "skills/README.md", "USER.md"]);
  });

  it("does not invent children for directories that have not been loaded", () => {
    expect(
      flattenWorkspaceDirectoryListings({
        "": [{ path: "skills", name: "skills", type: "dir" }],
      }).map((entry) => entry.path),
    ).toEqual(["skills"]);
  });
});
