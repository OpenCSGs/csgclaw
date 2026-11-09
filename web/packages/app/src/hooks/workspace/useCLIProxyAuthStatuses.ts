import { useCallback, useMemo } from "react";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { fetchCLIProxyAuthStatus } from "@/api/cliproxy";
import type { CLIProxyAuthStatus } from "@/api/cliproxy";
import { normalizeAuthProviderName, providerNeedsAuth } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { workspaceQueryKeys } from "./workspaceQueries";

export type CLIProxyAuthStatusMap = Record<string, CLIProxyAuthStatus>;

export function useCLIProxyAuthStatuses(providers: readonly unknown[], t: TranslateFn) {
  const queryClient = useQueryClient();
  const normalizedProviders = useMemo(
    () =>
      Array.from(
        new Set(
          providers
            .map((provider) => normalizeAuthProviderName(provider))
            .filter((provider) => providerNeedsAuth(provider)),
        ),
      ),
    [providers],
  );

  const authQueries = useQueries({
    queries: normalizedProviders.map((provider) => ({
      queryKey: workspaceQueryKeys.cliProxyAuthStatus(provider),
      queryFn: () => fetchCLIProxyAuthStatus(provider),
      retry: 0,
    })),
  });

  const cliproxyAuthStatuses = useMemo(() => {
    const result: CLIProxyAuthStatusMap = {};
    normalizedProviders.forEach((provider, index) => {
      const query = authQueries[index];
      if (query?.data) {
        result[provider] = query.data;
        return;
      }
      if (query?.isError) {
        result[provider] = {
          provider,
          authenticated: false,
          login_required: true,
          message: query.error?.message || t("authMissing"),
        };
      }
    });
    return result;
  }, [authQueries, normalizedProviders, t]);

  const setCLIProxyAuthStatus = useCallback(
    (provider: unknown, status: CLIProxyAuthStatus) => {
      const normalized = normalizeAuthProviderName(provider);
      if (!providerNeedsAuth(normalized)) {
        return;
      }
      queryClient.setQueryData(workspaceQueryKeys.cliProxyAuthStatus(normalized), status);
    },
    [queryClient],
  );

  const refreshCLIProxyAuthStatus = useCallback(
    async (provider: unknown): Promise<CLIProxyAuthStatus | null> => {
      const normalized = normalizeAuthProviderName(provider);
      if (!providerNeedsAuth(normalized)) {
        return null;
      }
      try {
        return await queryClient.fetchQuery({
          queryKey: workspaceQueryKeys.cliProxyAuthStatus(normalized),
          queryFn: () => fetchCLIProxyAuthStatus(normalized),
          retry: 0,
        });
      } catch (_) {
        return null;
      }
    },
    [queryClient],
  );

  return {
    cliproxyAuthStatuses,
    setCLIProxyAuthStatus,
    refreshCLIProxyAuthStatus,
  };
}
