// @ts-nocheck
import { useQuery } from "@tanstack/react-query";
import { fetchAgentProfileModels, fetchAgents } from "@/api/agents";
import { fetchBootstrap, fetchBootstrapConfig, fetchVersion } from "@/api/app";
import { fetchHubTemplate, fetchHubTemplates, fetchHubWorkspaceFile } from "@/api/hub";
import { fetchManagerProfile } from "@/api/agents";
import { fetchUpgradeStatus } from "@/api/upgrade";
import { modelRequestKey, normalizeRuntimeImageMap, normalizeRuntimeKind, parseJSONMap } from "@/models/agents";
import { normalizeIMData } from "@/models/conversations";
import { normalizeUpgradeStatus } from "@/models/upgradeStatus";

export const workspaceQueryKeys = {
  bootstrap: () => ["workspace", "bootstrap"],
  bootstrapConfig: () => ["workspace", "bootstrap-config"],
  managerProfile: () => ["workspace", "manager-profile"],
  agents: () => ["workspace", "agents"],
  hubTemplates: () => ["workspace", "hub-templates"],
  hubTemplate: (templateID) => ["workspace", "hub-template", templateID || ""],
  hubWorkspaceFile: (templateID, workspacePath) => [
    "workspace",
    "hub-workspace-file",
    templateID || "",
    workspacePath || "",
  ],
  agentProfileModels: (requestKey) => ["workspace", "agent-profile-models", requestKey || ""],
  cliProxyAuthStatus: (provider) => ["workspace", "cliproxy-auth-status", provider || ""],
  appVersion: () => ["workspace", "app-version"],
  upgradeStatus: () => ["workspace", "upgrade-status"],
};

export async function fetchWorkspaceBootstrapData() {
  return normalizeIMData(await fetchBootstrap());
}

export async function fetchWorkspaceBootstrapConfig() {
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

export async function fetchWorkspaceAppVersion(options = {}) {
  const payload = await fetchVersion(options);
  const version = typeof payload?.version === "string" ? payload.version.trim() : "";
  return version || "dev";
}

export async function fetchWorkspaceUpgradeStatus() {
  return normalizeUpgradeStatus(await fetchUpgradeStatus());
}

export async function fetchWorkspaceAgentProfileModels(draft) {
  if (!draft?.provider) {
    return { models: [] };
  }
  return fetchAgentProfileModels({
    ...draft,
    headers: parseJSONMap(draft.headersText),
  });
}

export function useWorkspaceBootstrapQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.bootstrap(),
    queryFn: fetchWorkspaceBootstrapData,
  });
}

export function useWorkspaceBootstrapConfigQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.bootstrapConfig(),
    queryFn: fetchWorkspaceBootstrapConfig,
  });
}

export function useWorkspaceManagerProfileQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.managerProfile(),
    queryFn: fetchManagerProfile,
    retry: 0,
  });
}

export function useWorkspaceAgentsQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.agents(),
    queryFn: () => fetchAgents(),
  });
}

export function useWorkspaceHubTemplatesQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.hubTemplates(),
    queryFn: async () => {
      const payload = await fetchHubTemplates();
      return Array.isArray(payload) ? payload : [];
    },
  });
}

export function useWorkspaceHubTemplateQuery(templateID) {
  return useQuery({
    queryKey: workspaceQueryKeys.hubTemplate(templateID),
    queryFn: () => fetchHubTemplate(templateID),
    enabled: Boolean(templateID),
  });
}

export function useWorkspaceHubWorkspaceFileQuery(templateID, workspacePath) {
  return useQuery({
    queryKey: workspaceQueryKeys.hubWorkspaceFile(templateID, workspacePath),
    queryFn: () => fetchHubWorkspaceFile(templateID, workspacePath),
    enabled: Boolean(templateID && workspacePath),
  });
}

export function useWorkspaceAgentProfileModelsQuery(draft, options = {}) {
  const requestKey = modelRequestKey(draft);
  return useQuery({
    queryKey: workspaceQueryKeys.agentProfileModels(requestKey),
    queryFn: () => fetchWorkspaceAgentProfileModels(draft),
    enabled: Boolean(options.enabled && requestKey),
    retry: 0,
  });
}

export function useWorkspaceAppVersionQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.appVersion(),
    queryFn: fetchWorkspaceAppVersion,
  });
}

export function useWorkspaceUpgradeStatusQuery() {
  return useQuery({
    queryKey: workspaceQueryKeys.upgradeStatus(),
    queryFn: fetchWorkspaceUpgradeStatus,
  });
}
