import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiErrorCode, errorMessage } from "@/api/client";
import { fetchAgentRuntimes, installAgentRuntime } from "@/api/agentRuntimes";
import type { AgentRuntimeInstallation } from "@/api/agentRuntimes";
import { agentRuntimeByName, normalizeAgentRuntimeList, upsertAgentRuntime } from "@/models/agentRuntimes";
import type { AgentRuntime } from "@/models/agentRuntimes";
import type { TranslateFn } from "@/models/conversations";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { localizeAPIError } from "@/shared/i18n";
import { useGlobalNotice } from "@/components/ui";

export type AgentRuntimesController = {
  error: string;
  loading: boolean;
  installError: string;
  installProgress: AgentRuntimeInstallation | null;
  installingRuntime: string;
  install: (name: string) => Promise<void>;
  refresh: () => Promise<void>;
  refreshing: boolean;
  runtimes: AgentRuntime[];
};

export function useAgentRuntimes(t: TranslateFn): AgentRuntimesController {
  const queryClient = useQueryClient();
  const { showNotice } = useGlobalNotice();
  const [installingRuntime, setInstallingRuntime] = useState("");
  const [installError, setInstallError] = useState("");
  const [installProgress, setInstallProgress] = useState<AgentRuntimeInstallation | null>(null);
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

  const install = useCallback(
    async (name: string) => {
      const runtimeName = String(name || "").trim();
      if (!runtimeName || installingRuntime) {
        return;
      }
      setInstallingRuntime(runtimeName);
      setInstallError("");
      setInstallProgress(null);
      try {
        const installed = await installAgentRuntime(runtimeName, setInstallProgress);
        queryClient.setQueryData<AgentRuntime[]>(workspaceQueryKeys.agentRuntimes(), (current) =>
          upsertAgentRuntime(current ?? [], installed),
        );
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrapConfig() });
      } catch (error) {
        const message = localizeAPIError(error, t, t("computerRuntimeInstallFailed"));
        setInstallError(message);
        const code = apiErrorCode(error);
        if (code === "dsh_node_required" || code === "dsh_npm_required") {
          showNotice({ title: t("dshNodeRequiredTitle"), message, closeLabel: t("close"), tone: "warning" });
        } else if (code === "dsh_node_version_unsupported") {
          showNotice({ title: t("dshNodeUnsupportedTitle"), message, closeLabel: t("close"), tone: "warning" });
        }
      } finally {
        setInstallingRuntime("");
      }
    },
    [installingRuntime, queryClient, showNotice, t],
  );

  return {
    error: runtimesQuery.isError ? errorMessage(runtimesQuery.error, t("computerRuntimesLoadFailed")) : "",
    install,
    installError,
    installProgress,
    installingRuntime,
    loading: runtimesQuery.isPending,
    refresh,
    refreshing: runtimesQuery.isFetching && !runtimesQuery.isPending,
    runtimes,
  };
}

async function fetchNormalizedAgentRuntimes(): Promise<AgentRuntime[]> {
  return normalizeAgentRuntimeList(await fetchAgentRuntimes());
}
