import { useQuery } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import { fetchAgentProfileModels, fetchAgentWorkspace, fetchAgentWorkspaceFile, fetchAgents } from "@/api/agents";
import type { AgentProfileModelRequest } from "@/api/agents";
import { fetchBootstrap, fetchBootstrapConfig, fetchVersion } from "@/api/app";
import type { FetchVersionOptions } from "@/api/app";
import { fetchHubTemplate, fetchHubTemplates, fetchHubWorkspaceFile } from "@/api/hub";
import { fetchManagerProfile } from "@/api/agents";
import { fetchUpgradeStatus } from "@/api/upgrade";
import { modelRequestKey, normalizeRuntimeImageMap, normalizeRuntimeKind, parseJSONMap } from "@/models/agents";
import type { AgentLike, AgentProfileLike, AgentProfileModelsResponse, RuntimeBootstrapConfig } from "@/models/agents";
import { normalizeIMData } from "@/models/conversations";
import type { IMData } from "@/models/conversations";
import type { HubTemplate, HubWorkspaceFile } from "@/models/hubWorkspace";
import type { WorkspaceFile, WorkspaceListing } from "@/models/workspace";
import { normalizeUpgradeStatus } from "@/models/upgradeStatus";
import type { UpgradeStatus } from "@/models/upgradeStatus";

const WORKSPACE_QUERY_SCOPE = "workspace";

export const workspaceQueryKeys = {
  bootstrap: () => [WORKSPACE_QUERY_SCOPE, "bootstrap"] as const,
  bootstrapConfig: () => [WORKSPACE_QUERY_SCOPE, "bootstrap-config"] as const,
  managerProfile: () => [WORKSPACE_QUERY_SCOPE, "manager-profile"] as const,
  agents: () => [WORKSPACE_QUERY_SCOPE, "agents"] as const,
  hubTemplates: () => [WORKSPACE_QUERY_SCOPE, "hub-templates"] as const,
  hubTemplate: (templateID: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "hub-template", templateID || ""] as const,
  hubWorkspaceFile: (templateID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "hub-workspace-file", templateID || "", workspacePath || ""] as const,
  agentWorkspace: (agentID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-workspace", agentID || "", workspacePath || ""] as const,
  agentWorkspaceFile: (agentID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-workspace-file", agentID || "", workspacePath || ""] as const,
  agentProfileModels: (requestKey: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-profile-models", requestKey || ""] as const,
  cliProxyAuthStatus: (provider: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "cliproxy-auth-status", provider || ""] as const,
  appVersion: () => [WORKSPACE_QUERY_SCOPE, "app-version"] as const,
  upgradeStatus: () => [WORKSPACE_QUERY_SCOPE, "upgrade-status"] as const,
};

export async function fetchWorkspaceBootstrapData(): Promise<IMData> {
  return normalizeIMData(await fetchBootstrap()) as IMData;
}

export async function fetchWorkspaceBootstrapConfig(): Promise<RuntimeBootstrapConfig> {
  const payload = await fetchBootstrapConfig();
  return {
    ...payload,
    runtime_kind: normalizeRuntimeKind(payload.runtime_kind),
    supported_runtime_kinds: Array.isArray(payload.supported_runtime_kinds)
      ? payload.supported_runtime_kinds
          .map((item) => normalizeRuntimeKind(item))
          .filter((item, index, array) => item && array.indexOf(item) === index)
      : [],
    runtime_default_images: normalizeRuntimeImageMap(payload.runtime_default_images),
  };
}

export async function fetchWorkspaceAppVersion(options: FetchVersionOptions = {}): Promise<string> {
  const payload = await fetchVersion(options);
  const version = typeof payload?.version === "string" ? payload.version.trim() : "";
  return version || "dev";
}

export async function fetchWorkspaceUpgradeStatus(): Promise<UpgradeStatus | null> {
  return normalizeUpgradeStatus(await fetchUpgradeStatus());
}

export async function fetchWorkspaceAgentProfileModels(
  draft: (AgentProfileModelRequest & { headersText?: string }) | null | undefined,
): Promise<AgentProfileModelsResponse> {
  if (!draft?.provider) {
    return { models: [] };
  }
  return fetchAgentProfileModels({
    ...draft,
    headers: parseJSONMap(draft.headersText),
  });
}

export function useWorkspaceBootstrapQuery(): UseQueryResult<IMData> {
  return useQuery<IMData>({
    queryKey: workspaceQueryKeys.bootstrap(),
    queryFn: fetchWorkspaceBootstrapData,
  });
}

export function useWorkspaceBootstrapConfigQuery(): UseQueryResult<RuntimeBootstrapConfig> {
  return useQuery<RuntimeBootstrapConfig>({
    queryKey: workspaceQueryKeys.bootstrapConfig(),
    queryFn: fetchWorkspaceBootstrapConfig,
  });
}

export function useWorkspaceManagerProfileQuery(): UseQueryResult<AgentProfileLike> {
  return useQuery<AgentProfileLike>({
    queryKey: workspaceQueryKeys.managerProfile(),
    queryFn: fetchManagerProfile,
    retry: 0,
  });
}

export function useWorkspaceAgentsQuery(): UseQueryResult<AgentLike[]> {
  return useQuery<AgentLike[]>({
    queryKey: workspaceQueryKeys.agents(),
    queryFn: () => fetchAgents(),
  });
}

export function useWorkspaceHubTemplatesQuery(): UseQueryResult<HubTemplate[]> {
  return useQuery<HubTemplate[]>({
    queryKey: workspaceQueryKeys.hubTemplates(),
    queryFn: async () => {
      const payload = await fetchHubTemplates();
      return Array.isArray(payload) ? payload : [];
    },
  });
}

export function useWorkspaceHubTemplateQuery(templateID: string): UseQueryResult<HubTemplate> {
  return useQuery<HubTemplate>({
    queryKey: workspaceQueryKeys.hubTemplate(templateID),
    queryFn: () => fetchHubTemplate(templateID),
    enabled: Boolean(templateID),
  });
}

export function useWorkspaceHubWorkspaceFileQuery(
  templateID: string,
  workspacePath: string,
): UseQueryResult<HubWorkspaceFile> {
  return useQuery<HubWorkspaceFile>({
    queryKey: workspaceQueryKeys.hubWorkspaceFile(templateID, workspacePath),
    queryFn: () => fetchHubWorkspaceFile(templateID, workspacePath),
    enabled: Boolean(templateID && workspacePath),
  });
}

export function useWorkspaceAgentWorkspaceQuery(
  agentID: string | null | undefined,
  workspacePath = "",
): UseQueryResult<WorkspaceListing> {
  return useQuery<WorkspaceListing>({
    queryKey: workspaceQueryKeys.agentWorkspace(agentID, workspacePath),
    queryFn: () => fetchAgentWorkspace(String(agentID || ""), workspacePath),
    enabled: Boolean(agentID),
  });
}

export function useWorkspaceAgentWorkspaceFileQuery(
  agentID: string | null | undefined,
  workspacePath: string | null | undefined,
): UseQueryResult<WorkspaceFile> {
  return useQuery<WorkspaceFile>({
    queryKey: workspaceQueryKeys.agentWorkspaceFile(agentID, workspacePath),
    queryFn: () => fetchAgentWorkspaceFile(String(agentID || ""), String(workspacePath || "")),
    enabled: Boolean(agentID && workspacePath),
  });
}

export function useWorkspaceAgentProfileModelsQuery(
  draft: (AgentProfileModelRequest & { headersText?: string }) | null | undefined,
  options: { enabled?: boolean } = {},
): UseQueryResult<AgentProfileModelsResponse> {
  const requestKey = modelRequestKey(draft);
  return useQuery<AgentProfileModelsResponse>({
    queryKey: workspaceQueryKeys.agentProfileModels(requestKey),
    queryFn: () => fetchWorkspaceAgentProfileModels(draft),
    enabled: Boolean(options.enabled && requestKey),
    retry: 0,
  });
}

export function useWorkspaceAppVersionQuery(): UseQueryResult<string> {
  return useQuery<string>({
    queryKey: workspaceQueryKeys.appVersion(),
    queryFn: () => fetchWorkspaceAppVersion(),
  });
}

export function useWorkspaceUpgradeStatusQuery(): UseQueryResult<UpgradeStatus | null> {
  return useQuery<UpgradeStatus | null>({
    queryKey: workspaceQueryKeys.upgradeStatus(),
    queryFn: fetchWorkspaceUpgradeStatus,
  });
}
