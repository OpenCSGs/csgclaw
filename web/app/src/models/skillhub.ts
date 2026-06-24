import type { WorkspaceFile, WorkspaceListing } from "@/models/workspace";

export type SkillSummary = {
  description?: string;
  name: string;
};

export type SkillTree = WorkspaceListing;
export type SkillFile = WorkspaceFile;

export function hasSkillName(
  skills: readonly SkillSummary[] | null | undefined,
  name: string | null | undefined,
): boolean {
  const value = String(name || "").trim();
  if (!value) {
    return false;
  }
  return (skills || []).some((item) => String(item?.name || "").trim() === value);
}
