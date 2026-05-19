import {
  buildInitialCollapsedHubWorkspaceDirs,
  buildVisibleHubWorkspaceEntries,
  formatHubDate,
  formatHubDateTime,
  formatHubTemplateCount,
  hubWorkspaceAncestorDirs,
} from "@/models/hubWorkspace";

describe("hub workspace helpers", () => {
  it("derives ancestor directories from workspace paths", () => {
    expect(hubWorkspaceAncestorDirs("src/pages/App.tsx")).toEqual(["src", "src/pages"]);
    expect(hubWorkspaceAncestorDirs("/src//pages/")).toEqual(["src"]);
    expect(hubWorkspaceAncestorDirs("README.md")).toEqual([]);
    expect(hubWorkspaceAncestorDirs("")).toEqual([]);
  });

  it("filters entries hidden by collapsed parent directories", () => {
    const entries = [
      { path: "src", type: "dir" },
      { path: "src/App.tsx", type: "file" },
      { path: "src/components", type: "dir" },
      { path: "src/components/Button.tsx", type: "file" },
      { path: "README.md", type: "file" },
    ];

    expect(buildVisibleHubWorkspaceEntries(entries, { src: true }).map((entry) => entry.path)).toEqual([
      "src",
      "README.md",
    ]);
    expect(buildVisibleHubWorkspaceEntries(entries, { "src/components": true }).map((entry) => entry.path)).toEqual([
      "src",
      "src/App.tsx",
      "src/components",
      "README.md",
    ]);
  });

  it("initially collapses every directory entry", () => {
    expect(
      buildInitialCollapsedHubWorkspaceDirs([
        { path: "src", type: "dir" },
        { path: "src/App.tsx", type: "file" },
        { path: "tests", type: "dir" },
      ]),
    ).toEqual({ src: true, tests: true });
  });

  it("formats hub dates in a stable UTC timezone", () => {
    expect(formatHubDate("", "en")).toBe("-");
    expect(formatHubDate("2026-05-15T12:34:56Z", "en")).toBe("05/15/2026");
    expect(formatHubDateTime("2026-05-15T12:34:56Z", "en")).toContain("12:34:56");
    expect(formatHubDateTime("2026-05-15T12:34:56Z", "en")).toContain("(UTC)");
  });

  it("formats localized template counts", () => {
    const t = () => "templates";
    expect(formatHubTemplateCount(3, "en", t)).toBe("3 templates");
    expect(formatHubTemplateCount(3, "zh", t)).toBe("共 3 templates");
  });
});
