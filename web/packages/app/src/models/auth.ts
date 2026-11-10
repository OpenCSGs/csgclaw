export type AuthStatus = {
  authenticated: boolean;
  user_id: string;
  user_uuid: string;
  name?: string;
  avatar: string;
  opencsg_base_url: string;
  base_url: string;
  ai_gateway_base_url: string;
  portal_url: string;
  logged_in_at: string;
};

export type LoginResponse = {
  login_url: string;
};

export function emptyAuthStatus(): AuthStatus {
  return {
    authenticated: false,
    user_id: "",
    user_uuid: "",
    name: "",
    avatar: "",
    opencsg_base_url: "",
    base_url: "",
    ai_gateway_base_url: "",
    portal_url: "",
    logged_in_at: "",
  };
}

export function normalizeAuthStatus(source: unknown): AuthStatus {
  if (!source || typeof source !== "object") {
    return emptyAuthStatus();
  }
  const value = source as Record<string, unknown>;
  const authenticated = value.authenticated === true;
  return {
    authenticated,
    user_id: authenticated ? stringFromUnknown(value.user_id) : "",
    user_uuid: authenticated ? stringFromUnknown(value.user_uuid) : "",
    name: authenticated ? stringFromUnknown(value.name) : "",
    avatar: authenticated ? stringFromUnknown(value.avatar) : "",
    opencsg_base_url: authenticated ? normalizeBaseURL(value.opencsg_base_url) : "",
    base_url: authenticated ? normalizeBaseURL(value.base_url) : "",
    ai_gateway_base_url: authenticated ? normalizeBaseURL(value.ai_gateway_base_url || value.aigateway_base_url) : "",
    portal_url: authenticated ? stringFromUnknown(value.portal_url) : "",
    logged_in_at: authenticated ? stringFromUnknown(value.logged_in_at) : "",
  };
}

export function normalizeLoginResponse(source: unknown): LoginResponse {
  if (!source || typeof source !== "object") {
    return { login_url: "" };
  }
  return { login_url: stringFromUnknown((source as Record<string, unknown>).login_url) };
}

export function isAuthenticated(status: AuthStatus | null | undefined): boolean {
  return Boolean(status?.authenticated);
}

function normalizeBaseURL(source: unknown): string {
  return stringFromUnknown(source).replace(/\/+$/, "");
}

function stringFromUnknown(source: unknown): string {
  return typeof source === "string" ? source.trim() : "";
}
