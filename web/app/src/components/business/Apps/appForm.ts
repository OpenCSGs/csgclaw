import type { AppConfig, AppCredentials, AppDefinition, AppInstallation } from "@/api/apps";
import { localizeAPIError } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";

export type AppValueRow = { key: string; value: string };
export type AppForm = {
  name: string;
  config: AppConfig;
  credentials: AppCredentials;
  args: string;
  headers: AppValueRow[];
  env: AppValueRow[];
};

export function appName(appID: string, t: TranslateFn, fallback?: string): string {
  if (appID === "feishu") return t("appFeishuName");
  if (appID === "llm-wiki") return t("appWikiName");
  if (appID === "gitlab") return "GitLab";
  return fallback || appID;
}

export function appDescription(app: AppDefinition, t: TranslateFn): string {
  const key = { gitlab: "appGitLabDescription", feishu: "appFeishuDescription", "llm-wiki": "appWikiDescription" }[
    app.app_id
  ];
  return key ? t(key) : app.interface?.shortDescription || app.description;
}

export function appStatus(app: AppInstallation, t: TranslateFn): string {
  if (!app.enabled || app.resource_enabled === false) return t("appStatusDisabled");
  if (app.disconnected) return t("appStatusDisconnected");
  const keys = {
    configured: "appStatusConfigured",
    agent_identity_required: "appStatusAgentIdentity",
    needs_configuration: "appStatusNeedsConfiguration",
    connecting: "appStatusConnecting",
    connected: "appStatusConnected",
    error: "appStatusError",
    authorization_required: "appStatusAuthorizationRequired",
    disconnected: "appStatusDisconnected",
    disabled: "appStatusDisabled",
  };
  return t(keys[app.status] ?? "appStatusNeedsConfiguration");
}

// Only connection settings may become defaults. Credentials never come from a catalog.
function appConfigDefaults(definition: AppDefinition): AppConfig {
  const properties = definition.config_schema.properties;
  if (!properties || typeof properties !== "object") return {};
  const types: Record<string, string> = {
    transport: "string",
    url: "string",
    command: "string",
    cwd: "string",
    auth_mode: "string",
    credential_source: "string",
    platform_credential_source: "string",
    token_header: "string",
    token_prefix: "string",
    token_env: "string",
    app_id_env: "string",
    app_secret_env: "string",
    startup_timeout_sec: "number",
    tool_timeout_sec: "number",
  };
  const defaults: AppConfig = {};
  for (const [key, field] of Object.entries(properties)) {
    if (!Object.hasOwn(types, key) || !field || typeof field !== "object") continue;
    const schema = field as Record<string, unknown>;
    if (schema.writeOnly || typeof schema.default !== types[key]) continue;
    if (Array.isArray(schema.enum) && !schema.enum.includes(schema.default)) continue;
    Object.assign(defaults, { [key]: schema.default });
  }
  return defaults;
}

export function initialAppForm(
  definition: AppDefinition,
  existing: AppInstallation | null,
  hasFeishuChannel: boolean,
): AppForm {
  const feishu = definition.app_id === "feishu";
  const gitlab = definition.app_id === "gitlab";
  const defaults = appConfigDefaults(definition);
  const config = existing?.config ?? {
    transport: "http",
    auth_mode: feishu ? "feishu" : "bearer",
    token_header: "Authorization",
    token_prefix: "Bearer ",
    token_env: gitlab ? "GITLAB_PERSONAL_ACCESS_TOKEN" : "MCP_ACCESS_TOKEN",
    app_id_env: "FEISHU_APP_ID",
    app_secret_env: "FEISHU_APP_SECRET",
    startup_timeout_sec: 30,
    tool_timeout_sec: 60,
    ...defaults,
    credential_source: feishu && hasFeishuChannel ? defaults.credential_source || "feishu_channel" : "manual",
  };
  const savedHeaderNames = Object.keys(existing?.credentials_set ?? {})
    .filter((key) => key.startsWith("headers."))
    .map((key) => key.slice("headers.".length));
  const defaultHeaderNames = gitlab && !existing ? ["PRIVATE-TOKEN", "X-GitLab-Base-URL"] : [];
  const headerNames = new Set([
    ...defaultHeaderNames,
    ...Object.keys(existing?.config.headers ?? {}),
    ...savedHeaderNames,
  ]);
  return {
    name: existing?.name ?? definition.interface?.displayName ?? definition.name,
    config: gitlab
      ? {
          ...config,
          transport: "http",
          auth_mode: "connector",
          connector_id: "gitlab",
          platform_credential_source: config.platform_credential_source,
        }
      : config,
    credentials: {},
    args: config.args?.join("\n") ?? "",
    headers: [...headerNames].map((key) => ({ key, value: existing?.config.headers?.[key] ?? "" })),
    env: Object.entries(existing?.config.env ?? {}).map(([key, value]) => ({ key, value })),
  };
}

export function defaultPlatformCredentialSource(config: AppConfig): "manual" | "opencsg_login" {
  if (config.transport === "stdio" || config.auth_mode === "env" || config.auth_mode === "oauth2") return "manual";
  try {
    const url = new URL(config.url || "");
    if (url.protocol !== "https:" || url.username || url.password || (url.port && url.port !== "443")) return "manual";
    return ["opencsg.com", "opencsg-stg.com"].some(
      (host) => url.hostname === host || url.hostname.endsWith(`.public.${host}`),
    )
      ? "opencsg_login"
      : "manual";
  } catch {
    return "manual";
  }
}

export function appFormPayload(form: AppForm): { name: string; config: AppConfig; credentials: AppCredentials } {
  const config: AppConfig = {
    ...form.config,
    platform_credential_source: form.config.platform_credential_source || defaultPlatformCredentialSource(form.config),
  };
  const credentials: AppCredentials = { ...form.credentials };
  if (config.auth_mode === "connector") {
    config.transport = "http";
    config.connector_id = "gitlab";
    config.platform_credential_source = config.platform_credential_source || defaultPlatformCredentialSource(config);
    delete config.command;
    delete config.args;
    delete config.cwd;
    delete config.credential_source;
    delete config.token_header;
    delete config.token_prefix;
    delete config.token_env;
    delete config.app_id_env;
    delete config.app_secret_env;
    delete config.headers;
    delete config.env;
    credentials.headers = rowsToValues(form.headers);
    if (config.platform_credential_source === "opencsg_login") {
      for (const key of Object.keys(credentials.headers))
        if (key.toLowerCase() === "authorization") delete credentials.headers[key];
    }
    return {
      name: form.name.trim(),
      config,
      credentials: {
        ...(form.credentials.token ? { token: form.credentials.token } : {}),
        ...(Object.keys(credentials.headers).length ? { headers: credentials.headers } : {}),
      },
    };
  }
  if (config.transport === "stdio") {
    delete config.url;
    config.args = form.args
      .split("\n")
      .map((arg) => arg.replace(/\r$/, ""))
      .filter(Boolean);
  } else {
    delete config.command;
    delete config.args;
    delete config.cwd;
  }
  // Custom values can contain credentials, so submit them to the secret store.
  credentials.headers = rowsToValues(form.headers);
  if (config.platform_credential_source === "opencsg_login") {
    if (config.auth_mode !== "header") delete credentials.token;
    for (const key of Object.keys(credentials.headers))
      if (key.toLowerCase() === "authorization") delete credentials.headers[key];
  }
  credentials.env = rowsToValues(form.env);
  delete config.headers;
  delete config.env;
  for (const key of ["token", "app_id", "app_secret"] as const) {
    if (!credentials[key]) delete credentials[key];
  }
  return { name: form.name.trim(), config, credentials };
}

function rowsToValues(rows: AppValueRow[]): Record<string, string> {
  return Object.fromEntries(
    rows.filter((row) => row.key.trim() && row.value).map((row) => [row.key.trim(), row.value]),
  );
}

export function appConnectionError(app: AppInstallation, t: TranslateFn): string {
  const message = localizeAPIError({ code: app.last_error_code, message: app.last_error }, t);
  return app.last_error_code && app.last_error_http_status
    ? `${message} (HTTP ${app.last_error_http_status})`
    : message;
}
