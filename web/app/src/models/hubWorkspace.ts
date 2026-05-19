import type { CSSProperties } from "react";
import type { AgentTemplateLike } from "@/models/agents";
import type { LocaleCode, TranslateFn } from "@/models/conversations";

export type HubWorkspaceEntry = {
  depth?: number;
  name?: string;
  path: string;
  type: "dir" | "file" | string;
};

export type HubWorkspaceFile = {
  binary?: boolean | null;
  content?: string | null;
  path: string;
  size?: number | null;
};

export type HubTemplateSource = {
  name?: string | null;
};

export type HubTemplate = AgentTemplateLike & {
  source?: HubTemplateSource | null;
  updated_at?: string | null;
  workspace?: {
    entries?: HubWorkspaceEntry[];
    kind?: string | null;
  } | null;
};

export type HubTreeDepthStyle = CSSProperties & {
  "--hub-tree-depth"?: number;
};

export type CollapsedHubWorkspaceDirs = Record<string, boolean>;

export function hubWorkspaceAncestorDirs(path: unknown): string[] {
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

export function buildVisibleHubWorkspaceEntries(
  entries: readonly HubWorkspaceEntry[] | null | undefined,
  collapsedDirs: CollapsedHubWorkspaceDirs,
): HubWorkspaceEntry[] {
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

export function buildInitialCollapsedHubWorkspaceDirs(
  entries: readonly HubWorkspaceEntry[] | null | undefined,
): CollapsedHubWorkspaceDirs {
  return (entries || []).reduce<CollapsedHubWorkspaceDirs>((acc, entry) => {
    if (entry?.type === "dir" && entry.path) {
      acc[entry.path] = true;
    }
    return acc;
  }, {});
}

export function formatHubDate(value: string | number | Date | null | undefined, locale: LocaleCode): string {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    timeZone: "UTC",
  }).format(new Date(value));
}

export function formatHubDateTime(value: string | number | Date | null | undefined, locale: LocaleCode): string {
  if (!value) {
    return "-";
  }
  return `${new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZone: "UTC",
  }).format(new Date(value))} (UTC)`;
}

export function formatHubTemplateCount(count: number, locale: LocaleCode, t: TranslateFn): string {
  if (locale === "zh") {
    return `共 ${count} ${t("hubTemplateCountSuffix")}`;
  }
  return `${count} ${t("hubTemplateCountSuffix")}`;
}
