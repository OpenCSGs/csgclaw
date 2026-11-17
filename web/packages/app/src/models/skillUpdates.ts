export type SkillFileChange = {
  path: string;
  kind: "added" | "modified" | "deleted";
  before?: string;
  after?: string;
  previewUnavailable?: boolean;
};

export type SkillUpdateTarget = {
  id: string;
  name: string;
  version: string;
  state: string;
  changes?: SkillFileChange[];
};

export type SkillUpdateInfo = {
  name: string;
  available: boolean;
  skipped?: boolean;
  localVersion: string;
  latestVersion: string;
  commit: string;
  changelog?: string;
  publishedAt: string;
  checkedAt: string;
  sourceUrl: string;
  remotePath: string;
  targets: SkillUpdateTarget[];
  state: string;
  error?: string;
};

export type SkillUpdateStrategy = "update" | "keep-local";
export type SkillUpdateResult = {
  name: string;
  targetId: string;
  targetName: string;
  status: "updated" | "kept" | "failed";
  error?: string;
};

export function skillUpdateErrorKey(code: string | undefined): string {
  if (code === "skill_installation_changed" || code === "skill_release_changed") return "skillUpdatesChanged";
  if (code === "skill_update_source_unavailable") return "skillUpdatesSourceUnavailable";
  return "skillUpdatesFailed";
}
import type { SkillSummary } from "./skillhub";

export function hasDifferentSkillOrigin(local: SkillSummary | undefined, remote: SkillSummary): boolean {
  const path = local?.install?.remote_path;
  if (path) return path !== remote.remotePath;
  const source = local?.install?.source_url;
  if (!source || !remote.remotePath) return false;
  try {
    return !new URL(source).pathname.endsWith(`/skills/${remote.remotePath}`);
  } catch {
    return true;
  }
}
