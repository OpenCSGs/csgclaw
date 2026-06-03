import type { CSSProperties } from "react";

export type WorkspaceEntry = {
  depth?: number;
  name?: string;
  path: string;
  size?: number | null;
  type: "dir" | "file" | string;
};

export type WorkspaceListing = {
  entries?: WorkspaceEntry[];
  kind?: string | null;
  path?: string | null;
};

export type WorkspaceFile = {
  binary?: boolean | null;
  content?: string | null;
  path: string;
  size?: number | null;
  truncated?: boolean | null;
};

export type WorkspaceTreeDepthStyle = CSSProperties & {
  "--workspace-tree-depth"?: number;
};

export type CollapsedWorkspaceDirs = Record<string, boolean>;

export function workspaceAncestorDirs(path: unknown): string[] {
  const normalized = typeof path === "string" ? path.trim() : "";
  if (!normalized) {
    return [];
  }
  const segments = normalized.split("/").filter(Boolean);
  if (segments.length <= 1) {
    return [];
  }
  const ancestors = [];
  for (let index = 1; index < segments.length; index += 1) {
    ancestors.push(segments.slice(0, index).join("/"));
  }
  return ancestors;
}

export function buildVisibleWorkspaceEntries(
  entries: readonly WorkspaceEntry[] | null | undefined,
  collapsedDirs: CollapsedWorkspaceDirs,
): WorkspaceEntry[] {
  const hiddenParents: string[] = [];
  return (entries ?? []).filter((entry) => {
    while (hiddenParents.length && !entry.path.startsWith(`${hiddenParents[hiddenParents.length - 1]}/`)) {
      hiddenParents.pop();
    }
    const visible = hiddenParents.length === 0;
    if (entry.type === "dir" && collapsedDirs[entry.path]) {
      hiddenParents.push(entry.path);
    }
    return visible;
  });
}

export function buildInitialCollapsedWorkspaceDirs(
  entries: readonly WorkspaceEntry[] | null | undefined,
): CollapsedWorkspaceDirs {
  return (entries || []).reduce<CollapsedWorkspaceDirs>((acc, entry) => {
    if (entry?.type === "dir" && entry.path) {
      acc[entry.path] = true;
    }
    return acc;
  }, {});
}

export function firstWorkspaceFilePath(entries: readonly WorkspaceEntry[] | null | undefined): string {
  const file = (entries || []).find((entry) => entry?.type === "file" && typeof entry.path === "string" && entry.path);
  return file?.path || "";
}

export function hasWorkspaceFilePath(
  entries: readonly WorkspaceEntry[] | null | undefined,
  path: string | null | undefined,
): boolean {
  const value = String(path || "").trim();
  if (!value) {
    return false;
  }
  return (entries || []).some((entry) => entry?.type === "file" && entry.path === value);
}
