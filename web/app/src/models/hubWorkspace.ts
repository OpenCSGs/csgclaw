import type { AgentTemplateLike } from "@/models/agents";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import type { WorkspaceEntry, WorkspaceFile, WorkspaceListing } from "@/models/workspace";

export type HubWorkspaceEntry = WorkspaceEntry;
export type HubWorkspaceFile = WorkspaceFile;
export type HubWorkspaceListing = WorkspaceListing;

export const HUB_REGISTRY_KIND_LOCAL = "local";
export const HUB_REGISTRY_KIND_REMOTE = "remote";
export const OFFICIAL_HUB_REGISTRY_NAME = "official";

export type HubTemplateSource = {
  name?: string | null;
  kind?: string | null;
};

export function isDeletableHubTemplate(template: HubTemplate | null | undefined): boolean {
  return (
    String(template?.source?.kind ?? "")
      .trim()
      .toLowerCase() === HUB_REGISTRY_KIND_LOCAL
  );
}

export function isVisibleInHubTemplateList(template: HubTemplate | null | undefined): boolean {
  const sourceKind = String(template?.source?.kind ?? "")
    .trim()
    .toLowerCase();
  if (sourceKind === HUB_REGISTRY_KIND_REMOTE) {
    return (
      String(template?.source?.name ?? "")
        .trim()
        .toLowerCase() === OFFICIAL_HUB_REGISTRY_NAME
    );
  }
  return (
    String(template?.role ?? "")
      .trim()
      .toLowerCase() === "worker"
  );
}

export type HubTemplate = AgentTemplateLike & {
  source?: HubTemplateSource | null;
  updated_at?: string | null;
  workspace?: {
    entries?: HubWorkspaceEntry[];
    kind?: string | null;
  } | null;
};

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
    return `共 ${count} ${t("resourcesTemplateCountSuffix")}`;
  }
  return `${count} ${t("resourcesTemplateCountSuffix")}`;
}
