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

export function canPublishHubTemplateToCommunity(template: HubTemplate | null | undefined): boolean {
  if (!isDeletableHubTemplate(template)) {
    return false;
  }
  const runtimeKind = String(template?.runtime_kind || template?.workspace?.kind || "")
    .trim()
    .toLowerCase();
  return runtimeKind === "codex";
}

export function isHubTemplateMemoryEnabled(template: HubTemplate | null | undefined): boolean {
  const runtimeKind = String(template?.runtime_kind ?? "")
    .trim()
    .toLowerCase();
  if (runtimeKind !== "codex") return false;
  return (
    String(template?.runtime_options?.memory_mode ?? "enabled")
      .trim()
      .toLowerCase() !== "disabled"
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

export const HubTemplateErrorCodes = {
  accountEmailMissing: "USER-ERR-18",
  communityNameConflict: "SYS-ERR-4",
  deployResourceUnavailable: "RESOURCE-ERR-1",
  reviewFailed: "AGENT-ERR-23",
  reviewPending: "AGENT-ERR-22",
  sensitiveInformation: "SENSITIVE-ERR-0",
  templateAlreadyExists: "template_already_exists",
} as const;

export function hubTemplateErrorCode(error: unknown): string {
  if (!error || typeof error !== "object" || !("code" in error)) {
    return "";
  }
  return String((error as { code?: unknown }).code ?? "").trim();
}

export type HubTemplate = AgentTemplateLike & {
  namespace?: string;
  metadata?: HubTemplateMetadata | null;
  source?: HubTemplateSource | null;
  updated_at?: string | null;
  workspace?: {
    entries?: HubWorkspaceEntry[];
    kind?: string | null;
  } | null;
};

export type HubTemplateMetadata = {
  sensitive_check?: {
    status?: string | null;
    failure_details?: Array<{
      path?: string | null;
      status?: string | null;
      message?: string | null;
    }> | null;
  } | null;
};

export type HubTemplateReviewState = {
  kind: "pending" | "exception";
  paths: string[];
};

export function upsertHubTemplateReviewState(
  templates: readonly HubTemplate[] | undefined,
  templateID: string,
  status: "Pending" | "Fail",
  message = "",
): HubTemplate[] {
  const id = templateID.trim();
  if (!id) return [...(templates ?? [])];
  const separator = id.indexOf("/");
  const namespace = separator > 0 ? id.slice(0, separator) : "";
  const name = separator >= 0 ? id.slice(separator + 1) : id;
  const sensitiveCheck = {
    status,
    failure_details: message.trim() ? [{ message: message.trim() }] : [],
  };
  const current = templates ?? [];
  const existingIndex = current.findIndex((template) => template.id === id);
  if (existingIndex < 0) {
    return [
      ...current,
      {
        id,
        name,
        namespace: namespace || undefined,
        role: "worker",
        source: { kind: HUB_REGISTRY_KIND_REMOTE, name: OFFICIAL_HUB_REGISTRY_NAME },
        metadata: { sensitive_check: sensitiveCheck },
      },
    ];
  }
  return current.map((template, index) =>
    index === existingIndex
      ? {
          ...template,
          metadata: {
            ...template.metadata,
            sensitive_check: (template.metadata?.sensitive_check?.failure_details ?? []).some((detail) =>
              Boolean(String(detail?.path ?? "").trim()),
            )
              ? template.metadata?.sensitive_check
              : sensitiveCheck,
          },
        }
      : template,
  );
}

export function hubTemplateReviewState(template: HubTemplate | null | undefined): HubTemplateReviewState | null {
  const check = template?.metadata?.sensitive_check;
  const status = String(check?.status ?? "")
    .trim()
    .toLowerCase();
  if (status !== "pending" && status !== "exception" && status !== "fail") {
    return null;
  }
  const paths = (check?.failure_details ?? []).map((detail) => String(detail?.path ?? "").trim()).filter(Boolean);
  return { kind: status === "pending" ? "pending" : "exception", paths };
}

export function mergeHubTemplateDetail(
  detail: HubTemplate | null | undefined,
  summary: HubTemplate | null | undefined,
): HubTemplate | null {
  if (!detail) return summary ?? null;
  if (!summary || detail.id !== summary.id || detail.metadata?.sensitive_check) return detail;
  return summary.metadata?.sensitive_check ? { ...detail, metadata: summary.metadata } : detail;
}

export function hubTemplateFullName(template: HubTemplate | null | undefined): string {
  const name = String(template?.name ?? "").trim();
  const namespace = String(template?.namespace ?? "").trim();
  if (namespace && name) {
    return `${namespace}/${name}`;
  }
  return name || String(template?.id ?? "").trim();
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
    return `共 ${count} ${t("resourcesTemplateCountSuffix")}`;
  }
  return `${count} ${t("resourcesTemplateCountSuffix")}`;
}
