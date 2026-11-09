import { describe, expect, it } from "vitest";
import {
  matchesSkillQuery,
  skillDirectory,
  skillLibrarySources,
  skillSourceFilters,
  skillSourceDirectories,
  skillVersionLabel,
} from "@/models/skillLibrary";
import type { SkillSummary } from "@/models/skillhub";
const shared: SkillSummary = {
  name: "pdf",
  discovery: {
    tool: "Codex",
    path: "/test/.codex/skills/pdf",
    key: "pdf",
    updated_at: "2026-10-30",
    sources: [
      { tool: "Codex", path: "/test/.codex/skills/pdf", updated_at: "2026-10-30" },
      { tool: "Claude Code", path: "/test/.claude/skills/pdf", updated_at: "2026-10-30" },
    ],
  },
};
describe("skill software filters", () => {
  it("keeps every recorded app available after deduplication and counts each skill only once per app", () => {
    const skills = [
      shared,
      { name: "local", source: "local" },
      { name: "untracked" },
      { name: "market", source: "market" },
      { name: "builtin", source: "system" },
    ];
    expect(skillSourceFilters(skills)).toEqual([
      { source: "Claude Code", count: 1 },
      { source: "Codex", count: 1 },
      { source: "market", count: 1 },
      { source: "local", count: 2 },
      { source: "builtin", count: 1 },
    ]);
    for (const skill of skills) expect(skillLibrarySources(skill).length).toBeGreaterThan(0);
    expect(skillLibrarySources(shared)).toEqual(["Codex", "Claude Code"]);
    expect(skills.map((s) => s.name)).toEqual(["pdf", "local", "untracked", "market", "builtin"]);
  });
  it("matches secondary software sources as well as names and descriptions", () => {
    expect(matchesSkillQuery(shared, "claude")).toBe(true);
    expect(matchesSkillQuery(shared, "PDF")).toBe(true);
    expect(matchesSkillQuery(shared, "nothing")).toBe(false);
    expect(skillDirectory(shared)).toBe(shared.discovery?.path);
  });
});

describe("selected skill version sources", () => {
  it("merges multiple app aliases for the same physical directory", () => {
    const skill: SkillSummary = {
      name: "baocut",
      discovery: {
        tool: "Agents",
        path: "/shared/baocut",
        version: "0.9.1",
        updated_at: "2026-07-27",
        key: "selected",
        sources: [
          { tool: "Agents", path: "/shared/baocut", version: "0.9.1", updated_at: "2026-07-27" },
          { tool: "Codex", path: "/shared/baocut", version: "0.9.1", updated_at: "2026-07-27" },
        ],
      },
    };
    expect(skillSourceDirectories(skill)).toEqual([
      { path: "/shared/baocut", version: "0.9.1", updatedAt: "2026-07-27", active: true, tools: ["Agents", "Codex"] },
    ]);
  });
  it("retains the selected version when another alias has older scanned metadata for its directory", () => {
    const skill: SkillSummary = {
      ...shared,
      discovery: {
        ...shared.discovery!,
        version: "2.0.0",
        sources: [
          { tool: "Codex", path: shared.discovery!.path, version: "1.0.0", updated_at: "2026-07-01" },
          { tool: "Claude Code", path: "/other/pdf", version: "1.9.0", updated_at: "2026-10-30" },
        ],
      },
    };
    const sources = skillSourceDirectories(skill);
    expect(sources[0].version).toBe("2.0.0");
    expect(sources[0].active).toBe(true);
    expect(sources[1].active).toBe(false);
  });
  it("formats declared versions without inventing a version for unversioned skills", () => {
    expect(skillVersionLabel("0.9.1")).toBe("v0.9.1");
    expect(skillVersionLabel("v0.9.1")).toBe("v0.9.1");
    expect(skillVersionLabel("custom")).toBe("custom");
    expect(skillVersionLabel(undefined)).toBe("");
  });
});
