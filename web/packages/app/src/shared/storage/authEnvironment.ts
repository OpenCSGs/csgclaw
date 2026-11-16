import {
  authEnvironmentLoginReady,
  defaultAuthEnvironmentDraft,
  normalizeAuthEnvironmentDraft,
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

export function readStoredAuthCustomSites(): string[] {
  try {
    const values: unknown = JSON.parse(window.localStorage.getItem(AUTH_CUSTOM_SITES_STORAGE_KEY) || "[]");
    if (!Array.isArray(values)) return [];
    const urls = values.flatMap((value: unknown) => {
      const draft = normalizeAuthEnvironmentDraft({ preset: "custom", opencsgBaseURL: value });
      return authEnvironmentLoginReady(draft) ? [draft.opencsgBaseURL] : [];
    });
    return [...new Set(urls)];
  } catch (_) {
    return [];
  }
}

export function saveStoredAuthCustomSite(draft: AuthEnvironmentDraft): string[] {
  const { opencsgBaseURL } = resolveAuthEnvironmentDraft(draft);
  const sites = [...new Set([...readStoredAuthCustomSites(), opencsgBaseURL])];
  window.localStorage.setItem(AUTH_CUSTOM_SITES_STORAGE_KEY, JSON.stringify(sites));
  return sites;
}
