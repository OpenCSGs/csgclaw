import {
  authEnvironmentLoginReady,
  defaultAuthEnvironmentDraft,
  normalizeAuthEnvironmentDraft,
  normalizeBaseURL,
  resolveAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import { AUTH_CUSTOM_SITES_STORAGE_KEY, AUTH_ENVIRONMENT_STORAGE_KEY } from "@/shared/storage/keys";

export function readStoredAuthEnvironmentDraft(): AuthEnvironmentDraft {
  if (typeof window === "undefined") {
    return defaultAuthEnvironmentDraft();
  }
  try {
    return normalizeAuthEnvironmentDraft(
      JSON.parse(window.localStorage.getItem(AUTH_ENVIRONMENT_STORAGE_KEY) || "null"),
    );
  } catch (_) {
    return defaultAuthEnvironmentDraft();
  }
}

export function writeStoredAuthEnvironmentDraft(draft: AuthEnvironmentDraft) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(AUTH_ENVIRONMENT_STORAGE_KEY, JSON.stringify(resolveAuthEnvironmentDraft(draft)));
  } catch (_) {
    // Local storage can be unavailable in restricted browser contexts.
  }
}

export type SavedAuthSite = { name: string; url: string };

export function readStoredAuthCustomSites(): SavedAuthSite[] {
  try {
    const values: unknown = JSON.parse(window.localStorage.getItem(AUTH_CUSTOM_SITES_STORAGE_KEY) || "[]");
    if (!Array.isArray(values)) return [];
    const sites = new Map<string, SavedAuthSite>();
    for (const value of values) {
      const url = normalizeBaseURL(typeof value === "string" ? value : value?.url);
      const draft = normalizeAuthEnvironmentDraft({ preset: "custom", opencsgBaseURL: url });
      if (!authEnvironmentLoginReady(draft) || sites.has(url)) continue;
      const name = typeof value?.name === "string" ? value.name.trim() : "";
      sites.set(url, { url, name });
    }
    return [...sites.values()];
  } catch (_) {
    return [];
  }
}

export function writeStoredAuthCustomSites(sites: SavedAuthSite[]) {
  window.localStorage.setItem(AUTH_CUSTOM_SITES_STORAGE_KEY, JSON.stringify(sites));
}
