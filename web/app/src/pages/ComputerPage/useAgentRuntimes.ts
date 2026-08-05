import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { fetchAgentRuntimes } from "@/api/agentRuntimes";
import { agentRuntimeByName, normalizeAgentRuntimeList } from "@/models/agentRuntimes";
import type { AgentRuntime } from "@/models/agentRuntimes";
import type { TranslateFn } from "@/models/conversations";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";

export type AgentRuntimesController = {
  error: string;
  loading: boolean;
  refresh: () => Promise<void>;
  refreshing: boolean;
  runtimes: AgentRuntime[];
};

export function useAgentRuntimes(t: TranslateFn): AgentRuntimesController {
  const queryClient = useQueryClient();
  const bootstrapSyncedForInstalledCodexRef = useRef(false);
  const runtimesQuery = useQuery({
    queryKey: workspaceQueryKeys.agentRuntimes(),
    queryFn: fetchNormalizedAgentRuntimes,
    retry: 0,
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
    void queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrapConfig() });
  }, [codexInstalled, queryClient]);

  const refresh = useCallback(async () => {
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

  return {
    error: runtimesQuery.isError ? errorMessage(runtimesQuery.error, t("computerRuntimesLoadFailed")) : "",
    loading: runtimesQuery.isPending,
    refresh,
    refreshing: runtimesQuery.isFetching && !runtimesQuery.isPending,
    runtimes,
  };
}

async function fetchNormalizedAgentRuntimes(): Promise<AgentRuntime[]> {
  return normalizeAgentRuntimeList(await fetchAgentRuntimes());
}
