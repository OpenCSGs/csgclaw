import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { fetchAgentRuntimes, installAgentRuntimeRequest } from "@/api/agentRuntimes";
import {
  agentRuntimeByName,
  normalizeAgentRuntime,
  normalizeAgentRuntimeList,
  normalizeAgentRuntimeName,
  shouldPollAgentRuntimeInstallation,
  upsertAgentRuntime,
} from "@/models/agentRuntimes";
import type { AgentRuntime } from "@/models/agentRuntimes";
import type { TranslateFn } from "@/models/conversations";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";

const INSTALL_STATUS_POLL_INTERVAL_MS = 1500;

export type AgentRuntimesController = {
  busyRuntimeName: string;
  error: string;
  installError: string;
  installRuntime: (runtimeName: string) => Promise<void>;
  loading: boolean;
  refresh: () => Promise<void>;
  refreshing: boolean;
  runtimes: AgentRuntime[];
};

export function useAgentRuntimes(t: TranslateFn): AgentRuntimesController {
  const queryClient = useQueryClient();
  const installInFlightRef = useRef("");
  const bootstrapSyncedForInstalledCodexRef = useRef(false);
  const [busyRuntimeName, setBusyRuntimeName] = useState("");
  const [installError, setInstallError] = useState("");

  const runtimesQuery = useQuery({
    queryKey: workspaceQueryKeys.agentRuntimes(),
    queryFn: fetchNormalizedAgentRuntimes,
    retry: 0,
    refetchInterval: (query) =>
      shouldPollAgentRuntimeInstallation(query.state.data) ? INSTALL_STATUS_POLL_INTERVAL_MS : false,
  });

  const runtimes = useMemo(() => runtimesQuery.data ?? [], [runtimesQuery.data]);
  const codexInstalled = Boolean(agentRuntimeByName(runtimes, "codex")?.installed);

  useEffect(() => {
    if (!codexInstalled) {
      bootstrapSyncedForInstalledCodexRef.current = false;
      return;
    }
    if (bootstrapSyncedForInstalledCodexRef.current) {
      return;
    }
    bootstrapSyncedForInstalledCodexRef.current = true;
    setInstallError("");
    void queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrapConfig() });
  }, [codexInstalled, queryClient]);

  const refresh = useCallback(async () => {
    setInstallError("");
    try {
      await queryClient.fetchQuery({
        queryKey: workspaceQueryKeys.agentRuntimes(),
        queryFn: fetchNormalizedAgentRuntimes,
        retry: 0,
      });
    } catch (_) {
      // The query exposes the localized load error to the page.
    }
  }, [queryClient]);

  const installRuntime = useCallback(
    async (runtimeName: string) => {
      const name = normalizeAgentRuntimeName(runtimeName);
      if (!name || installInFlightRef.current) {
        return;
      }
      installInFlightRef.current = name;
      setBusyRuntimeName(name);
      setInstallError("");
      try {
        const runtime = normalizeAgentRuntime(await installAgentRuntimeRequest(name));
        if (!runtime) {
          throw new Error(t("computerRuntimeInstallFailed"));
        }
        queryClient.setQueryData<AgentRuntime[]>(workspaceQueryKeys.agentRuntimes(), (current = []) =>
          upsertAgentRuntime(current, runtime),
        );
      } catch (error) {
        setInstallError(errorMessage(error, t("computerRuntimeInstallFailed")));
        try {
          await queryClient.fetchQuery({
            queryKey: workspaceQueryKeys.agentRuntimes(),
            queryFn: fetchNormalizedAgentRuntimes,
            retry: 0,
          });
        } catch (_) {
          // Preserve the install error when refreshing the authoritative status also fails.
        }
      } finally {
        installInFlightRef.current = "";
        setBusyRuntimeName("");
      }
    },
    [queryClient, t],
  );

  return {
    busyRuntimeName,
    error: runtimesQuery.isError ? errorMessage(runtimesQuery.error, t("computerRuntimesLoadFailed")) : "",
    installError,
    installRuntime,
    loading: runtimesQuery.isPending,
    refresh,
    refreshing: runtimesQuery.isFetching && !runtimesQuery.isPending,
    runtimes,
  };
}

async function fetchNormalizedAgentRuntimes(): Promise<AgentRuntime[]> {
  return normalizeAgentRuntimeList(await fetchAgentRuntimes());
}
