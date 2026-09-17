import type { AppConfig, AppCredentials, AppDefinition, AppInstallation } from "@/api/apps";
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
  if (!app.enabled) return t("appStatusDisabled");
  if (app.disconnected) return t("appStatusDisconnected");
  const keys = {
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

export function initialAppForm(
  definition: AppDefinition,
  existing: AppInstallation | null,
  hasFeishuChannel: boolean,
): AppForm {
  const feishu = definition.app_id === "feishu";
  return {
    name: existing?.name ?? definition.interface?.displayName ?? definition.name,
    config: existing?.config ?? {
      transport: "http",
      auth_mode: feishu ? "feishu" : "bearer",
      credential_source: feishu && hasFeishuChannel ? "feishu_channel" : "manual",
      token_header: "Authorization",
      token_prefix: "Bearer ",
      token_env: definition.app_id === "gitlab" ? "GITLAB_PERSONAL_ACCESS_TOKEN" : "MCP_ACCESS_TOKEN",
      app_id_env: "FEISHU_APP_ID",
      app_secret_env: "FEISHU_APP_SECRET",
    },
    credentials: {},
    args: existing?.config.args?.join("\n") ?? "",
    headers: Object.entries(existing?.config.headers ?? {}).map(([key, value]) => ({ key, value })),
    env: Object.entries(existing?.config.env ?? {}).map(([key, value]) => ({ key, value })),
  };
}

export function appFormPayload(form: AppForm): { name: string; config: AppConfig; credentials: AppCredentials } {
  const config: AppConfig = { ...form.config };
  const credentials: AppCredentials = { ...form.credentials };
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
