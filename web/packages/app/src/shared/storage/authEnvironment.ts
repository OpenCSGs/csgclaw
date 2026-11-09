import {
  defaultAuthEnvironmentDraft,
  normalizeAuthEnvironmentDraft,
  resolveAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import { AUTH_ENVIRONMENT_STORAGE_KEY } from "@/shared/storage/keys";

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
