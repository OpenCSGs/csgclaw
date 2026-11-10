import { SIDEBAR_COLLAPSED_STORAGE_KEY, WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY } from "./keys";
import type { CollapsedWorkspaceGroups } from "@/models/routing";

const DEFAULT_COLLAPSED_WORKSPACE_GROUPS: CollapsedWorkspaceGroups = {
  "direct-messages": false,
  rooms: true,
  threads: true,
};

export function readCollapsedWorkspaceGroups(): CollapsedWorkspaceGroups {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY) || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ...DEFAULT_COLLAPSED_WORKSPACE_GROUPS };
    }
    return { ...DEFAULT_COLLAPSED_WORKSPACE_GROUPS, ...(parsed as CollapsedWorkspaceGroups) };
  } catch (_) {
    return { ...DEFAULT_COLLAPSED_WORKSPACE_GROUPS };
  }
}

export function readSidebarCollapsed(): boolean {
  try {
    return window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

export function writeWorkspacePreference(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Preferences still apply in memory when browser storage is unavailable.
  }
}
