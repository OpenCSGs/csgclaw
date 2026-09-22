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
  platform_credential_source?: "manual" | "opencsg_login";
  transport?: "http" | "stdio";
  url?: string;
  gitlab_base_url?: string;
  connector_id?: string;
  command?: string;
  args?: string[];
  cwd?: string;
  env?: Record<string, string>;
  headers?: Record<string, string>;
  auth_mode?: "none" | "bearer" | "header" | "env" | "feishu" | "oauth2" | "connector";
  credential_source?: "manual";
  token_header?: string;
  token_prefix?: string;
  token_env?: string;
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
  feishu_app_id?: string;
  resource_id?: string;
  resource_enabled?: boolean;
  bindings?: AppBindingSummary[];
  installation_id: string;
  agent_id: string;
  app_id: string;
  name: string;
  enabled: boolean;
  disconnected: boolean;
  status:
    | "configured"
    | "needs_configuration"
    | "connecting"
    | "connected"
    | "error"
    | "authorization_required"
    | "disconnected"
    | "disabled";
  last_error?: string;
  last_error_code?: string;
  last_error_http_status?: number;
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
  return `/api/v1/agents/${encodeURIComponent(agentID)}/connectors`;
}

function appPath(agentID: string, installationID: string): string {
  return `${agentAppsPath(agentID)}/${encodeURIComponent(installationID)}`;
}

export async function fetchAppDefinitions(signal?: AbortSignal): Promise<AppDefinition[]> {
  const result = await get<{ items: AppDefinition[] }>("/api/v1/connectors/catalog", { signal });
  return result.items ?? [];
}

export async function fetchAgentApps(agentID: string, signal?: AbortSignal): Promise<{ items: AppInstallation[] }> {
  const result = await get<{ items: AppInstallation[] }>(agentAppsPath(agentID), {
    signal,
  });
  return { ...result, items: (result.items ?? []).map(normalizeInstallation) };
}

export async function updateAgentApp(
  agentID: string,
  installationID: string,
  payload: Pick<AppUpdateRequest, "enabled">,
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

export type AppBindingSummary = {
  agent_name?: string;
  installation_id: string;
  agent_id: string;
  enabled: boolean;
  status: AppInstallation["status"];
  tool_count: number;
};
export async function fetchAppResources(signal?: AbortSignal): Promise<AppInstallation[]> {
  const result = await get<{ items: AppInstallation[] }>("/api/v1/connectors/resources", { signal });
  return (result.items ?? []).map(normalizeInstallation);
}
export async function createAppResource(payload: AppCreateRequest): Promise<AppInstallation> {
  return normalizeInstallation(await post<AppInstallation>("/api/v1/connectors/resources", payload));
}
export async function updateAppResource(id: string, payload: AppUpdateRequest): Promise<AppInstallation> {
  return normalizeInstallation(
    await patch<AppInstallation>(`/api/v1/connectors/resources/${encodeURIComponent(id)}`, payload),
  );
}
export function deleteAppResource(id: string): Promise<void> {
  return del(`/api/v1/connectors/resources/${encodeURIComponent(id)}`);
}
export async function probeAppResource(payload: AppProbeRequest): Promise<AppProbeResult> {
  const result = await post<AppProbeResult>("/api/v1/connectors/resources:probe", payload);
  return { ...result, tools: result.tools ?? [] };
}
export async function bindAgentApp(agentID: string, resourceID: string): Promise<AppInstallation> {
  return normalizeInstallation(
    await post<AppInstallation>(agentAppsPath(agentID), { resource_id: resourceID, connect: true }),
  );
}
