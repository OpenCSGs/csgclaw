import { MANAGER_AGENT_ROLE, WORKER_AGENT_ROLE } from "@/shared/constants/agents";
import type { HubTemplate } from "@/models/hubWorkspace";

export type ConfigSettings = {
  path: string;
  listen_addr: string;
  advertise_base_url?: string;
  advertise_base_url_effective?: string;
  access_token?: string;
  access_token_set?: boolean;
  access_token_preview?: string;
  show_upgrade: boolean;
  sandbox_provider: string;
  supported_sandbox_providers?: string[];
  default_manager_template: string;
  default_worker_template: string;
};

export type ConfigSettingsDraft = {
  listen_host: string;
  listen_port: string;
  advertise_base_url: string;
  advertise_base_url_effective: string;
  access_token: string;
  access_token_set: boolean;
  access_token_preview: string;
  show_upgrade: boolean;
  sandbox_provider: string;
  default_manager_template: string;
  default_worker_template: string;
};

export type ConfigSettingsUpdatePayload = {
  listen_addr: string;
  advertise_base_url: string;
  access_token?: string;
  show_upgrade: boolean;
  sandbox_provider: string;
  default_manager_template: string;
  default_worker_template: string;
};

export type ConfigTemplateOption = {
  label: string;
  value: string;
};

const BOOTSTRAP_MANAGER_RUNTIME = "picoclaw_sandbox";

export function isValidConfigBootstrapTemplate(
  template: HubTemplate | null | undefined,
  role: "manager" | "worker",
): boolean {
  const runtimeKind = String(template?.runtime_kind ?? "")
    .trim()
    .toLowerCase();
  if (role === "manager") {
    return runtimeKind === BOOTSTRAP_MANAGER_RUNTIME;
  }
  return runtimeKind !== "";
}

export function normalizeConfigSettings(source: unknown): ConfigSettings | null {
  if (!source || typeof source !== "object") {
    return null;
  }
  const value = source as Partial<ConfigSettings>;
  const accessToken = String(value.access_token || "").trim();
  const accessTokenSet = value.access_token_set ?? accessToken !== "";
  return {
    path: String(value.path || "").trim(),
    listen_addr: String(value.listen_addr || "").trim(),
    advertise_base_url: String(value.advertise_base_url || "").trim(),
    advertise_base_url_effective: String(value.advertise_base_url_effective || "").trim(),
    access_token: accessToken,
    access_token_set: accessTokenSet,
    access_token_preview: String(value.access_token_preview || "").trim(),
    show_upgrade: value.show_upgrade !== false,
    sandbox_provider: String(value.sandbox_provider || "").trim(),
    supported_sandbox_providers: Array.isArray(value.supported_sandbox_providers)
      ? value.supported_sandbox_providers.map((item) => String(item || "").trim()).filter(Boolean)
      : [],
    default_manager_template: String(value.default_manager_template || "").trim(),
    default_worker_template: String(value.default_worker_template || "").trim(),
  };
}

export function parseListenAddress(addr: string): { host: string; port: string } {
  const value = String(addr || "").trim();
  if (!value) {
    return { host: "0.0.0.0", port: "18080" };
  }
  if (value.startsWith("[")) {
    const separator = value.lastIndexOf("]:");
    if (separator >= 0) {
      return {
        host: value.slice(0, separator + 1),
        port: value.slice(separator + 2).trim() || "18080",
      };
    }
  }
  const separator = value.lastIndexOf(":");
  if (separator <= 0) {
    return { host: "0.0.0.0", port: value };
  }
  return {
    host: value.slice(0, separator).trim() || "0.0.0.0",
    port: value.slice(separator + 1).trim() || "18080",
  };
}

export function formatListenAddress(host: string, port: string): string {
  const normalizedHost = String(host || "").trim() || "0.0.0.0";
  const normalizedPort = String(port || "").trim() || "18080";
  if (normalizedHost.startsWith("[")) {
    return `${normalizedHost}:${normalizedPort}`;
  }
  return `${normalizedHost}:${normalizedPort}`;
}

export function normalizeAdvertiseBaseURL(value: string): string {
  return String(value || "").trim().replace(/\/+$/, "");
}

export function configSettingsToDraft(settings: ConfigSettings): ConfigSettingsDraft {
  const { host, port } = parseListenAddress(settings.listen_addr);
  return {
    listen_host: host,
    listen_port: port,
    advertise_base_url: normalizeAdvertiseBaseURL(settings.advertise_base_url || ""),
    advertise_base_url_effective: normalizeAdvertiseBaseURL(settings.advertise_base_url_effective || ""),
    access_token: "",
    access_token_set: settings.access_token_set ?? Boolean(settings.access_token),
    access_token_preview: settings.access_token_preview || "",
    show_upgrade: settings.show_upgrade,
    sandbox_provider: settings.sandbox_provider,
    default_manager_template: settings.default_manager_template,
    default_worker_template: settings.default_worker_template,
  };
}

export function configDraftToUpdatePayload(draft: ConfigSettingsDraft): ConfigSettingsUpdatePayload {
  const payload: ConfigSettingsUpdatePayload = {
    listen_addr: formatListenAddress(draft.listen_host, draft.listen_port),
    advertise_base_url: normalizeAdvertiseBaseURL(draft.advertise_base_url),
    show_upgrade: draft.show_upgrade,
    sandbox_provider: draft.sandbox_provider,
    default_manager_template: draft.default_manager_template,
    default_worker_template: draft.default_worker_template,
  };
  if (draft.access_token.trim()) {
    payload.access_token = draft.access_token.trim();
  }
  return payload;
}

export function configAdvertiseBaseURLPlaceholder(draft: ConfigSettingsDraft): string {
  if (draft.advertise_base_url.trim()) {
    return "";
  }
  return draft.advertise_base_url_effective.trim();
}

export function configTemplateOptions(
  templates: readonly HubTemplate[] | null | undefined,
  role: "manager" | "worker",
  currentValue = "",
): ConfigTemplateOption[] {
  const filtered = (templates ?? []).filter((template) => {
    const templateRole = String(template?.role ?? "")
      .trim()
      .toLowerCase();
    const roleMatches =
      role === "manager"
        ? templateRole === MANAGER_AGENT_ROLE
        : templateRole === WORKER_AGENT_ROLE || templateRole === "";
    return roleMatches && isValidConfigBootstrapTemplate(template, role);
  });
  const options = filtered
    .map((template) => {
      const value = String(template?.id || "").trim();
      if (!value) {
        return null;
      }
      const label = String(template?.name || value).trim() || value;
      return { value, label };
    })
    .filter(Boolean) as ConfigTemplateOption[];
  const current = String(currentValue || "").trim();
  if (current && !options.some((item) => item.value === current)) {
    options.unshift({ value: current, label: current });
  }
  return options;
}

export function sandboxProviderLabel(provider: string, t: (key: string) => string): string {
  switch (provider) {
    case "boxlite":
      return t("configSettingsSandboxBoxlite");
    case "docker":
      return t("configSettingsSandboxDocker");
    case "csghub":
      return t("configSettingsSandboxCSGHub");
    default:
      return provider;
  }
}
