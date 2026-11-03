import { skillOriginKind, type SkillSummary } from "./skillhub";

export function skillDirectory(skill: SkillSummary): string {
  return skill.directory?.trim() || skill.discovery?.path.trim() || "";
}

export function skillLibrarySource(skill: SkillSummary): string {
  if (skill.discovery) return skill.discovery.tool;
  const kind = skillOriginKind(skill);
  return kind === "unknown" ? "local" : kind;
}

// A deduplicated skill remains discoverable under every app that installed it,
// including sources whose version was not selected as the library's winner.
export function skillLibrarySources(skill: SkillSummary): string[] {
  return [...new Set([skillLibrarySource(skill), ...(skill.discovery?.sources.map((source) => source.tool) ?? [])])];
}

export function skillSourceFilters(skills: readonly SkillSummary[]): { source: string; count: number }[] {
  const counts = new Map<string, number>();
  for (const skill of skills) {
    for (const source of skillLibrarySources(skill)) counts.set(source, (counts.get(source) ?? 0) + 1);
  }
  const categories = ["market", "local", "builtin"];
  const rank = (source: string) => categories.indexOf(source) + 1 || 0;
  return [...counts]
    .map(([source, count]) => ({ source, count }))
    .sort((a, b) => rank(a.source) - rank(b.source) || a.source.localeCompare(b.source));
}

export function matchesSkillQuery(skill: SkillSummary, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase();
  return (
    !needle ||
    [skill.name, skill.description, ...skillLibrarySources(skill)].some((value) =>
      value?.toLocaleLowerCase().includes(needle),
    )
  );
}

export type SkillSourceDirectory = {
  path: string;
  tools: string[];
  version?: string;
  updatedAt: string;
  active: boolean;
};

// Software aliases pointing to one canonical directory are one physical skill,
// not competing versions. The selected metadata is authoritative for that path.
export function skillSourceDirectories(skill: SkillSummary): SkillSourceDirectory[] {
  const discovery = skill.discovery;
  if (!discovery) return [];
  const directories = new Map<string, SkillSourceDirectory>();
  for (const source of [discovery, ...discovery.sources]) {
    let directory = directories.get(source.path);
    if (!directory) {
      directory = {
        path: source.path,
        tools: [],
        version: source.version,
        updatedAt: source.updated_at,
        active: source.path === discovery.path,
      };
      directories.set(source.path, directory);
    }
    if (!directory.tools.includes(source.tool)) directory.tools.push(source.tool);
  }
  return [...directories.values()];
}

export function skillVersionLabel(version: string | undefined): string {
  const value = version?.trim() || "";
  return /^\d/.test(value) ? `v${value}` : value;
}
