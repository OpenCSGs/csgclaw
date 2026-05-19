import type { ProfileURLDraft } from "./types";

export function isBlank(value: unknown): boolean {
  return !String(value ?? "").trim();
}

export function profileBaseURLMissing(draft: ProfileURLDraft | null | undefined): boolean {
  return draft?.provider === "api" && isBlank(draft.base_url);
}
