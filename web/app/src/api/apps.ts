import { del, get, patch, post } from "@/api/client";

export type AppDefinition = {
  app_id: string;
  name: string;
  description: string;
  version: string;
  interface: { displayName?: string; shortDescription?: string; logo?: string };
  config_schema: Record<string, unknown>;
  auth_methods: string[];
  oauth_supported: boolean;
};

export type AppConfig = {
  transport?: "http" | "stdio";
  url?: string;
  command?: string;
  args?: string[];
  cwd?: string;
  env?: Record<string, string>;
  headers?: Record<string, string>;
  auth_mode?: "none" | "bearer" | "header" | "env" | "feishu" | "oauth2";
  credential_source?: "manual" | "feishu_channel";
  token_header?: string;
  token_prefix?: string;
  token_env?: string;
  app_id_header?: string;
  app_secret_header?: string;
  app_id_env?: string;
  app_secret_env?: string;
  startup_timeout_sec?: number;
  tool_timeout_sec?: number;
};

export type AppCredentials = {
  token?: string;
  app_id?: string;
  app_secret?: string;
  headers?: Record<string, string>;
  env?: Record<string, string>;
};

export type AppTool = { name: string; title?: string; description?: string; inputSchema?: Record<string, unknown> };
export type AppInstallation = {
  installation_id: string;
  agent_id: string;
  app_id: string;
  name: string;
  enabled: boolean;
  disconnected: boolean;
  status:
    | "needs_configuration"
    | "connecting"
    | "connected"
    | "error"
    | "authorization_required"
    | "disconnected"
    | "disabled";
  last_error?: string;
  config: AppConfig;
  credentials_set: Record<string, boolean>;
  tools: AppTool[];
  created_at: string;
  updated_at: string;
};

export type AppProbeRequest = {
  app_id: string;
  installation_id?: string;
  config: AppConfig;
  credentials: AppCredentials;
};
export type AppProbeResult = {
  connected: boolean;
  tools: AppTool[];
  server_info?: { name: string; version: string };
  protocol_version?: string;
};
export type AppCreateRequest = AppProbeRequest & { name: string; connect: boolean };
export type AppUpdateRequest = { name?: string; enabled?: boolean; config?: AppConfig; credentials?: AppCredentials };

function agentAppsPath(agentID: string): string {
  return `/api/v1/agents/${encodeURIComponent(agentID)}/apps`;
}

function appPath(agentID: string, installationID: string): string {
  return `${agentAppsPath(agentID)}/${encodeURIComponent(installationID)}`;
}

export async function fetchAppDefinitions(signal?: AbortSignal): Promise<AppDefinition[]> {
  const result = await get<{ items: AppDefinition[] }>("/api/v1/apps", { signal });
  return result.items ?? [];
}

export async function fetchAgentApps(
  agentID: string,
  signal?: AbortSignal,
): Promise<{ items: AppInstallation[]; feishu_channel_available?: boolean }> {
  const result = await get<{ items: AppInstallation[]; feishu_channel_available?: boolean }>(agentAppsPath(agentID), {
    signal,
  });
  return { ...result, items: (result.items ?? []).map(normalizeInstallation) };
}

export async function createAgentApp(agentID: string, payload: AppCreateRequest): Promise<AppInstallation> {
  return normalizeInstallation(await post<AppInstallation>(agentAppsPath(agentID), payload));
}

export async function updateAgentApp(
  agentID: string,
  installationID: string,
  payload: AppUpdateRequest,
): Promise<AppInstallation> {
  return normalizeInstallation(await patch<AppInstallation>(appPath(agentID, installationID), payload));
}

export async function connectAgentApp(agentID: string, installationID: string): Promise<AppInstallation> {
  return normalizeInstallation(await post<AppInstallation>(`${appPath(agentID, installationID)}/connect`));
}

export async function disconnectAgentApp(agentID: string, installationID: string): Promise<AppInstallation> {
  return normalizeInstallation(await post<AppInstallation>(`${appPath(agentID, installationID)}/disconnect`));
}

export function deleteAgentApp(agentID: string, installationID: string): Promise<void> {
  return del(appPath(agentID, installationID));
}

export async function probeAgentApp(agentID: string, payload: AppProbeRequest): Promise<AppProbeResult> {
  const result = await post<AppProbeResult>(`${agentAppsPath(agentID)}:probe`, payload);
  return { ...result, tools: result.tools ?? [] };
}

function normalizeInstallation(value: AppInstallation): AppInstallation {
  return { ...value, tools: value.tools ?? [], credentials_set: value.credentials_set ?? {} };
}
