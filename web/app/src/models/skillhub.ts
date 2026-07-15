import type { WorkspaceFile, WorkspaceListing } from "@/models/workspace";

export const SKILL_SOURCE_BUILTIN = "builtin";
export const SKILL_SOURCE_LOCAL = "local";
export const SKILL_SOURCE_OFFICIAL = "official";
export const SKILL_SOURCE_PERSONAL = "personal";
export const SKILL_SOURCE_SYSTEM = "system";

export type SkillSummary = {
  description?: string;
  name: string;
  readonly?: boolean;
  remoteRef?: string;
  remotePath?: string;
  remoteURL?: string;
  source?: string;
};

export type SkillTree = WorkspaceListing;
export type SkillFile = WorkspaceFile;

export function isOfficialSkill(skill: SkillSummary | null | undefined): boolean {
  return normalizeSkillSource(skill?.source) === SKILL_SOURCE_OFFICIAL;
}

export function isPersonalSkill(skill: SkillSummary | null | undefined): boolean {
  return normalizeSkillSource(skill?.source) === SKILL_SOURCE_PERSONAL;
}

export function isReadonlySkill(skill: SkillSummary | null | undefined): boolean {
  const source = normalizeSkillSource(skill?.source);
  return Boolean(
    skill?.readonly ||
    source === SKILL_SOURCE_SYSTEM ||
    source === SKILL_SOURCE_OFFICIAL ||
    source === SKILL_SOURCE_PERSONAL,
  );
}

export function skillSourceBadgeName(skill: SkillSummary | null | undefined): string {
  const source = normalizeSkillSource(skill?.source);
  if (source === SKILL_SOURCE_OFFICIAL) {
    return SKILL_SOURCE_OFFICIAL;
  }
  if (source === SKILL_SOURCE_PERSONAL) {
    return SKILL_SOURCE_PERSONAL;
  }
  if (source === SKILL_SOURCE_BUILTIN || source === SKILL_SOURCE_SYSTEM || skill?.readonly) {
    return SKILL_SOURCE_BUILTIN;
  }
  return skill ? SKILL_SOURCE_LOCAL : "";
}

export function hasSkillName(
  skills: readonly SkillSummary[] | null | undefined,
  name: string | null | undefined,
): boolean {
  const value = normalizeSkillName(name);
  if (!value) {
    return false;
  }
  return (skills || []).some((item) => normalizeSkillName(item?.name) === value);
}

export function remoteSkillInstallName(skill: SkillSummary | null | undefined): string {
  const remotePathName = skillNameFromPath(skill?.remotePath);
  return remotePathName || normalizeSkillName(skill?.name);
}

function normalizeSkillName(value: unknown): string {
  return String(value || "").trim();
}

function normalizeSkillSource(value: unknown): string {
  return String(value || "")
    .trim()
    .toLowerCase();
}

function skillNameFromPath(value: unknown): string {
  const path = String(value || "").trim();
  if (!path) {
    return "";
  }
  return (
    path
      .split("/")
      .map((part) => part.trim())
      .filter(Boolean)
      .at(-1) || ""
  );
}
