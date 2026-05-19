// @ts-nocheck
import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { fetchAgents, fetchManagerProfile } from "@/api/agents";
import {
  fetchWorkspaceAppVersion,
  fetchWorkspaceBootstrapConfig,
  fetchWorkspaceBootstrapData,
  fetchWorkspaceUpgradeStatus,
  useWorkspaceAgentsQuery,
  useWorkspaceAppVersionQuery,
  useWorkspaceBootstrapConfigQuery,
  useWorkspaceBootstrapQuery,
  useWorkspaceHubTemplatesQuery,
  useWorkspaceManagerProfileQuery,
  useWorkspaceUpgradeStatusQuery,
  workspaceQueryKeys,
} from "./workspaceQueries";
import { fetchHubTemplates } from "@/api/hub";

export function useWorkspaceData() {
  const queryClient = useQueryClient();
  const bootstrapQuery = useWorkspaceBootstrapQuery();
  const bootstrapConfigQuery = useWorkspaceBootstrapConfigQuery();
  const managerProfileQuery = useWorkspaceManagerProfileQuery();
  const agentsQuery = useWorkspaceAgentsQuery();
  const hubTemplatesQuery = useWorkspaceHubTemplatesQuery();
  const appVersionQuery = useWorkspaceAppVersionQuery();
  const upgradeStatusQuery = useWorkspaceUpgradeStatusQuery();

  const setBootstrapData = useCallback(
    (value) => {
      queryClient.setQueryData(workspaceQueryKeys.bootstrap(), (current) =>
        typeof value === "function" ? value(current ?? null) : value,
      );
    },
    [queryClient],
  );

  const setManagerProfileData = useCallback(
    (value) => {
      queryClient.setQueryData(workspaceQueryKeys.managerProfile(), (current) =>
        typeof value === "function" ? value(current ?? null) : value,
      );
    },
    [queryClient],
  );

  const setAgentsData = useCallback(
    (value) => {
      queryClient.setQueryData(workspaceQueryKeys.agents(), (current) =>
        typeof value === "function" ? value(current ?? []) : value,
      );
    },
    [queryClient],
  );

  const setHubTemplatesData = useCallback(
    (value) => {
      queryClient.setQueryData(workspaceQueryKeys.hubTemplates(), (current) =>
        typeof value === "function" ? value(current ?? []) : value,
      );
    },
    [queryClient],
  );

  const setAppVersionData = useCallback(
    (value) => {
      queryClient.setQueryData(workspaceQueryKeys.appVersion(), (current) =>
        typeof value === "function" ? value(current ?? "dev") : value,
      );
    },
    [queryClient],
  );

  const setUpgradeStatusData = useCallback(
    (value) => {
      queryClient.setQueryData(workspaceQueryKeys.upgradeStatus(), (current) =>
        typeof value === "function" ? value(current ?? null) : value,
      );
    },
    [queryClient],
  );

  const refreshWorkspaceBootstrap = useCallback(async () => {
    try {
      const normalized = await fetchWorkspaceBootstrapData();
      setBootstrapData(normalized);
      return normalized;
    } catch (_) {
      return null;
    }
  }, [setBootstrapData]);

  const refreshWorkspaceBootstrapConfig = useCallback(async () => {
    try {
      const normalized = await fetchWorkspaceBootstrapConfig();
      queryClient.setQueryData(workspaceQueryKeys.bootstrapConfig(), normalized);
      return normalized;
    } catch (_) {
      return null;
    }
  }, [queryClient]);

  const refreshWorkspaceUpgradeStatus = useCallback(async () => {
    try {
      const payload = await fetchWorkspaceUpgradeStatus();
      setUpgradeStatusData(payload);
      return payload;
    } catch (_) {
      setUpgradeStatusData(null);
      return null;
    }
  }, [setUpgradeStatusData]);

  const refreshWorkspaceAppVersion = useCallback(
    async (options = {}) => {
      const version = await fetchWorkspaceAppVersion(options);
      setAppVersionData(version);
      return version;
    },
    [setAppVersionData],
  );

  const refreshWorkspaceManagerProfile = useCallback(async () => {
    try {
      const profile = await fetchManagerProfile();
      setManagerProfileData(profile);
      return profile;
    } catch (_) {
      return null;
    }
  }, [setManagerProfileData]);

  const refreshWorkspaceAgents = useCallback(
    async (options = {}) => {
      const agents = await fetchAgents(options);
      setAgentsData(agents);
      return agents;
    },
    [setAgentsData],
  );

  const refreshWorkspaceHubTemplates = useCallback(async () => {
    const payload = await fetchHubTemplates();
    const templates = Array.isArray(payload) ? payload : [];
    setHubTemplatesData(templates);
    return templates;
  }, [setHubTemplatesData]);

  return {
    queryClient,
    bootstrapQuery,
    bootstrapConfigQuery,
    managerProfileQuery,
    agentsQuery,
    hubTemplatesQuery,
    appVersionQuery,
    upgradeStatusQuery,
    data: bootstrapQuery.data ?? null,
    bootstrapConfig: bootstrapConfigQuery.data ?? null,
    managerProfile: managerProfileQuery.data ?? null,
    agents: agentsQuery.data ?? [],
    agentsLoaded: agentsQuery.isFetched,
    hubTemplates: hubTemplatesQuery.data ?? [],
    hubLoaded: hubTemplatesQuery.isFetched,
    appVersion: appVersionQuery.data ?? "dev",
    upgradeStatus: upgradeStatusQuery.data ?? null,
    setBootstrapData,
    setManagerProfileData,
    setAgentsData,
    setHubTemplatesData,
    setAppVersionData,
    setUpgradeStatusData,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceUpgradeStatus,
    refreshWorkspaceAppVersion,
    refreshWorkspaceManagerProfile,
    refreshWorkspaceAgents,
    refreshWorkspaceHubTemplates,
  };
}
