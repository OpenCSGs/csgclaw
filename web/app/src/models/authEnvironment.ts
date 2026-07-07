import type { AuthStatus } from "@/models/auth";

export type AuthEnvironmentPresetID = "prod" | "stage" | "custom";

export type AuthEnvironmentDraft = {
  preset: AuthEnvironmentPresetID;
  opencsgBaseURL: string;
  csgHubBaseURL: string;
  aiGatewayBaseURL: string;
};

export type AuthEnvironmentLoginPayload = {
  opencsg_base_url: string;
  csghub_base_url: string;
  ai_gateway_base_url?: string;
};

export type AuthEnvironmentPreset = {
  id: AuthEnvironmentPresetID;
  label: string;
  opencsgBaseURL: string;
  csgHubBaseURL: string;
  aiGatewayBaseURL: string;
};

const PRODUCTION_AUTH_ENVIRONMENT_PRESET: AuthEnvironmentPreset = {
  id: "prod",
  label: "opencsg.com",
  opencsgBaseURL: "https://opencsg.com",
  csgHubBaseURL: "https://hub.opencsg.com",
  aiGatewayBaseURL: "https://ai.space.opencsg.com/v1",
};

const STAGE_AUTH_ENVIRONMENT_PRESET: AuthEnvironmentPreset = {
  id: "stage",
  label: "opencsg-stg.com",
  opencsgBaseURL: "https://opencsg-stg.com",
  csgHubBaseURL: "https://opencsg-stg.com",
  aiGatewayBaseURL: "https://aigateway.opencsg-stg.com/v1",
};

export const AUTH_ENVIRONMENT_PRESETS: AuthEnvironmentPreset[] = [
  PRODUCTION_AUTH_ENVIRONMENT_PRESET,
  STAGE_AUTH_ENVIRONMENT_PRESET,
];

const DEFAULT_PRESET = PRODUCTION_AUTH_ENVIRONMENT_PRESET;

export function defaultAuthEnvironmentDraft(): AuthEnvironmentDraft {
  return authEnvironmentDraftFromPreset(DEFAULT_PRESET.id);
}

export function authEnvironmentDraftFromPreset(id: AuthEnvironmentPresetID): AuthEnvironmentDraft {
  const preset = AUTH_ENVIRONMENT_PRESETS.find((item) => item.id === id) || DEFAULT_PRESET;
  return {
    preset: preset.id,
    opencsgBaseURL: preset.opencsgBaseURL,
    csgHubBaseURL: preset.csgHubBaseURL,
    aiGatewayBaseURL: preset.aiGatewayBaseURL,
  };
}

export function normalizeAuthEnvironmentDraft(source: unknown): AuthEnvironmentDraft {
  if (!source || typeof source !== "object") {
    return defaultAuthEnvironmentDraft();
  }
  const value = source as Record<string, unknown>;
  const preset = normalizePresetID(value.preset);
  const opencsgBaseURL = normalizeBaseURL(value.opencsgBaseURL);
  const csgHubBaseURL = normalizeBaseURL(value.csgHubBaseURL);
  const aiGatewayBaseURL = normalizeAIGatewayBaseURL(value.aiGatewayBaseURL);
  return {
    preset,
    opencsgBaseURL,
    csgHubBaseURL,
    aiGatewayBaseURL,
  };
}

export function authEnvironmentDraftFromStatus(
  status: AuthStatus | null | undefined,
  fallback: AuthEnvironmentDraft = defaultAuthEnvironmentDraft(),
): AuthEnvironmentDraft {
  if (!status?.authenticated) {
    return fallback;
  }
  const statusOpenCSGBaseURL = normalizeBaseURL(status.opencsg_base_url);
  const statusBaseURL = normalizeBaseURL(status.base_url);
  const statusAIGatewayBaseURL = normalizeAIGatewayBaseURL(status.ai_gateway_base_url);
  const preset = presetIDForEnvironment({
    opencsgBaseURL: statusOpenCSGBaseURL,
    csgHubBaseURL: statusBaseURL,
    aiGatewayBaseURL: statusAIGatewayBaseURL,
  });
  if (preset) {
    return authEnvironmentDraftFromPreset(preset);
  }
  const statusSiteBaseURL = statusOpenCSGBaseURL || statusBaseURL;
  const next = {
    preset: "custom" as AuthEnvironmentPresetID,
    opencsgBaseURL: statusSiteBaseURL || fallback.opencsgBaseURL,
    csgHubBaseURL: statusBaseURL || (statusSiteBaseURL ? "" : fallback.csgHubBaseURL),
    aiGatewayBaseURL: statusAIGatewayBaseURL || (statusSiteBaseURL ? "" : fallback.aiGatewayBaseURL),
  };
  const resolved = resolveAuthEnvironmentDraft(next);
  return {
    ...resolved,
    preset: presetIDForEnvironment(resolved) || next.preset,
  };
}

export function resolveAuthEnvironmentDraft(draft: AuthEnvironmentDraft): AuthEnvironmentDraft {
  const opencsgBaseURL = normalizeBaseURL(draft.opencsgBaseURL);
  const custom = draft.preset === "custom";
  const csgHubBaseURL = custom ? opencsgBaseURL : normalizeBaseURL(draft.csgHubBaseURL) || opencsgBaseURL;
  const aiGatewayBaseURL = custom
    ? derivedAIGatewayBaseURL(opencsgBaseURL)
    : normalizeAIGatewayBaseURL(draft.aiGatewayBaseURL) || derivedAIGatewayBaseURL(opencsgBaseURL);
  const preset = draft.preset;
  return {
    preset,
    opencsgBaseURL,
    csgHubBaseURL,
    aiGatewayBaseURL,
  };
}

export function authEnvironmentLoginPayload(draft: AuthEnvironmentDraft): AuthEnvironmentLoginPayload {
  const resolved = resolveAuthEnvironmentDraft(draft);
  return {
    opencsg_base_url: resolved.opencsgBaseURL,
    csghub_base_url: resolved.csgHubBaseURL,
    ...(resolved.aiGatewayBaseURL ? { ai_gateway_base_url: resolved.aiGatewayBaseURL } : {}),
  };
}

export function authEnvironmentLoginReady(draft: AuthEnvironmentDraft): boolean {
  return Boolean(resolveAuthEnvironmentDraft(draft).opencsgBaseURL);
}

export function authEnvironmentDisplayLabel(draft: AuthEnvironmentDraft, customLabel = "Custom"): string {
  const resolved = resolveAuthEnvironmentDraft(draft);
  const preset = AUTH_ENVIRONMENT_PRESETS.find((item) => item.id === resolved.preset);
  if (preset) {
    return preset.label;
  }
  return hostnameFromURL(resolved.opencsgBaseURL) || customLabel;
}

function presetIDForEnvironment(
  draft: Pick<AuthEnvironmentDraft, "opencsgBaseURL" | "csgHubBaseURL" | "aiGatewayBaseURL">,
) {
  const opencsgBaseURL = normalizeBaseURL(draft.opencsgBaseURL);
  const csgHubBaseURL = normalizeBaseURL(draft.csgHubBaseURL);
  const aiGatewayBaseURL = normalizeAIGatewayBaseURL(draft.aiGatewayBaseURL);
  return AUTH_ENVIRONMENT_PRESETS.find(
    (preset) =>
      preset.opencsgBaseURL === opencsgBaseURL &&
      preset.csgHubBaseURL === csgHubBaseURL &&
      preset.aiGatewayBaseURL === aiGatewayBaseURL,
  )?.id;
}

function hostnameFromURL(raw: string): string {
  try {
    return new URL(raw).hostname;
  } catch (_) {
    return raw.replace(/^https?:\/\//, "").replace(/\/.*$/, "");
  }
}

function normalizePresetID(source: unknown): AuthEnvironmentPresetID {
  if (source === "prod" || source === "stage" || source === "custom") {
    return source;
  }
  return DEFAULT_PRESET.id;
}

function normalizeBaseURL(source: unknown): string {
  const raw = typeof source === "string" ? source.trim().replace(/\/+$/, "") : "";
  if (!raw) {
    return "";
  }
  try {
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      return raw;
    }
    url.search = "";
    url.hash = "";
    return url.toString().replace(/\/+$/, "");
  } catch (_) {
    return raw;
  }
}

function derivedAIGatewayBaseURL(opencsgBaseURL: string): string {
  const baseURL = normalizeBaseURL(opencsgBaseURL);
  if (!baseURL) {
    return "";
  }
  try {
    const url = new URL(baseURL);
    const path = url.pathname.replace(/\/+$/, "");
    url.pathname = `${path}/aigateway`;
    return normalizeAIGatewayBaseURL(url.toString());
  } catch (_) {
    return normalizeAIGatewayBaseURL(`${baseURL}/aigateway`);
  }
}

function normalizeAIGatewayBaseURL(source: unknown): string {
  const baseURL = normalizeBaseURL(source);
  if (!baseURL) {
    return "";
  }
  try {
    const url = new URL(baseURL);
    const path = url.pathname.replace(/\/+$/, "");
    if (!path.endsWith("/v1")) {
      url.pathname = `${path}/v1`;
    }
    return url.toString().replace(/\/+$/, "");
  } catch (_) {
    return baseURL;
  }
}
