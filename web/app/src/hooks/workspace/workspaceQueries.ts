import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import { fetchAgentProfileModels, fetchAgentWorkspace, fetchAgentWorkspaceFile, fetchAgents } from "@/api/agents";
import type { AgentProfileModelRequest } from "@/api/agents";
import { fetchBootstrap, fetchBootstrapConfig, fetchRuntimeImages, fetchVersion } from "@/api/app";
import type { FetchVersionOptions } from "@/api/app";
import { fetchHubTemplate, fetchHubTemplates, fetchHubWorkspace, fetchHubWorkspaceFile } from "@/api/hub";
import { fetchModelProviders } from "@/api/modelProviders";
import { fetchAgenticHubOfficialSkillsPage, fetchSkillFile, fetchSkills, fetchSkillTree } from "@/api/skills";
import type { AgenticHubSkillsPage } from "@/api/skills";
import { fetchManagerProfile } from "@/api/agents";
import { fetchUpgradeStatus } from "@/api/upgrade";
import {
  modelRequestKey,
  normalizeRuntimeImageMap,
  normalizeRuntimeKind,
  normalizeRuntimeOptionSchemaMap,
  parseJSONMap,
} from "@/models/agents";
import type { AgentLike, AgentProfileLike, AgentProfileModelsResponse, RuntimeBootstrapConfig } from "@/models/agents";
import { normalizeIMData } from "@/models/conversations";
import type { IMData } from "@/models/conversations";
import type { HubTemplate, HubWorkspaceFile, HubWorkspaceListing } from "@/models/hubWorkspace";
import type { ModelProviderCatalog } from "@/models/modelProviders";
import type { SkillFile, SkillSummary, SkillTree } from "@/models/skillhub";
import type { WorkspaceFile, WorkspaceListing } from "@/models/workspace";
import { normalizeUpgradeStatus } from "@/models/upgradeStatus";
import type { UpgradeStatus } from "@/models/upgradeStatus";

const WORKSPACE_QUERY_SCOPE = "workspace";

export const workspaceQueryKeys = {
  bootstrap: () => [WORKSPACE_QUERY_SCOPE, "bootstrap"] as const,
  bootstrapConfig: () => [WORKSPACE_QUERY_SCOPE, "bootstrap-config"] as const,
  managerProfile: () => [WORKSPACE_QUERY_SCOPE, "manager-profile"] as const,
  agents: () => [WORKSPACE_QUERY_SCOPE, "agents"] as const,
  teams: () => [WORKSPACE_QUERY_SCOPE, "teams"] as const,
  modelProviders: () => [WORKSPACE_QUERY_SCOPE, "model-providers"] as const,
  runtimeImages: () => [WORKSPACE_QUERY_SCOPE, "runtime-images"] as const,
  hubTemplates: () => [WORKSPACE_QUERY_SCOPE, "hub-templates"] as const,
  hubTemplate: (templateID: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "hub-template", templateID || ""] as const,
  hubWorkspace: (templateID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "hub-workspace", templateID || "", workspacePath || ""] as const,
  hubWorkspaceFile: (templateID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "hub-workspace-file", templateID || "", workspacePath || ""] as const,
  officialSkills: (search: string | null | undefined = "") =>
    [WORKSPACE_QUERY_SCOPE, "official-skills", String(search || "").trim()] as const,
  skills: () => [WORKSPACE_QUERY_SCOPE, "skills"] as const,
  skillTree: (path: string | null | undefined) => [WORKSPACE_QUERY_SCOPE, "skill-tree", path || ""] as const,
  skillFile: (path: string | null | undefined) => [WORKSPACE_QUERY_SCOPE, "skill-file", path || ""] as const,
  agentWorkspaceScope: (agentID: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-workspace", agentID || ""] as const,
  agentWorkspace: (agentID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-workspace", agentID || "", workspacePath || ""] as const,
  agentWorkspaceFileScope: (agentID: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-workspace-file", agentID || ""] as const,
  agentWorkspaceFile: (agentID: string | null | undefined, workspacePath: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-workspace-file", agentID || "", workspacePath || ""] as const,
  agentSkills: (agentID: string | null | undefined) => [WORKSPACE_QUERY_SCOPE, "agent-skills", agentID || ""] as const,
  agentProfileModels: (requestKey: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "agent-profile-models", requestKey || ""] as const,
  cliProxyAuthStatus: (provider: string | null | undefined) =>
    [WORKSPACE_QUERY_SCOPE, "cliproxy-auth-status", provider || ""] as const,
  authStatus: () => [WORKSPACE_QUERY_SCOPE, "auth-status"] as const,
  connectors: () => [WORKSPACE_QUERY_SCOPE, "connectors"] as const,
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
    runtime_option_schemas: normalizeRuntimeOptionSchemaMap(payload.runtime_option_schemas),
  };
}

export async function fetchWorkspaceRuntimeImages(): Promise<string[]> {
  const payload = await fetchRuntimeImages();
  if (!Array.isArray(payload)) {
    return [];
  }
  const seen = new Set<string>();
  const images: string[] = [];
  for (const item of payload) {
    const image = String(item ?? "").trim();
    if (!image || seen.has(image)) {
      continue;
    }
    seen.add(image);
    images.push(image);
  }
  return images;
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

export function useWorkspaceModelProvidersQuery(): UseQueryResult<ModelProviderCatalog> {
  return useQuery<ModelProviderCatalog>({
    queryKey: workspaceQueryKeys.modelProviders(),
    queryFn: fetchModelProviders,
  });
}

export function useWorkspaceRuntimeImagesQuery(): UseQueryResult<string[]> {
  return useQuery<string[]>({
    queryKey: workspaceQueryKeys.runtimeImages(),
    queryFn: fetchWorkspaceRuntimeImages,
    retry: 0,
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

export function useWorkspaceHubWorkspaceQuery(
  templateID: string,
  workspacePath = "",
): UseQueryResult<HubWorkspaceListing> {
  return useQuery<HubWorkspaceListing>({
    queryKey: workspaceQueryKeys.hubWorkspace(templateID, workspacePath),
    queryFn: () => fetchHubWorkspace(templateID, workspacePath),
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

export function useWorkspaceSkillsQuery(): UseQueryResult<SkillSummary[]> {
  return useQuery<SkillSummary[]>({
    queryKey: workspaceQueryKeys.skills(),
    queryFn: async () => {
      const payload = await fetchSkills();
      return Array.isArray(payload) ? payload : [];
    },
  });
}

export function useWorkspaceOfficialSkillsQuery(search = "", options: { enabled?: boolean } = {}) {
  const normalizedSearch = String(search || "").trim();
  return useInfiniteQuery<AgenticHubSkillsPage>({
    queryKey: workspaceQueryKeys.officialSkills(normalizedSearch),
    queryFn: ({ pageParam }) =>
      fetchAgenticHubOfficialSkillsPage(
        typeof pageParam === "number" ? pageParam : Number(pageParam) || 1,
        normalizedSearch,
      ),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => lastPage.nextPage ?? undefined,
    enabled: Boolean(options.enabled),
    retry: 0,
  });
}

export function useWorkspaceSkillTreeQuery(path: string | null | undefined): UseQueryResult<SkillTree> {
  return useQuery<SkillTree>({
    queryKey: workspaceQueryKeys.skillTree(path),
    queryFn: () => fetchSkillTree(String(path || "")),
    enabled: Boolean(path),
  });
}

export function useWorkspaceSkillFileQuery(path: string | null | undefined): UseQueryResult<SkillFile> {
  return useQuery<SkillFile>({
    queryKey: workspaceQueryKeys.skillFile(path),
    queryFn: () => fetchSkillFile(String(path || "")),
    enabled: Boolean(path),
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
