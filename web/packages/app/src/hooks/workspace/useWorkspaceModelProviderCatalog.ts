import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { checkModelProvider, type ModelProviderPayload } from "@/api/modelProviders";
import type { ModelProvider, ModelProviderCatalog } from "@/models/modelProviders";
import { useWorkspaceModelProvidersQuery, workspaceQueryKeys } from "./workspaceQueries";

export function useWorkspaceModelProviderCatalog() {
  const queryClient = useQueryClient();
  const query = useWorkspaceModelProvidersQuery();
  const { refetch } = query;

  const refreshWorkspaceModelProviders = useCallback(async () => {
    const result = await refetch();
    return result.isSuccess ? result.data : null;
  }, [refetch]);

  const checkWorkspaceModelProvider = useCallback(
    async (providerID: string, payload: ModelProviderPayload) => {
      const queryKey = workspaceQueryKeys.modelProviders();
      const providerForCheck = (catalog?: ModelProviderCatalog) =>
        catalog?.providers.find(({ id }) => id === providerID);
      const before = providerForCheck(queryClient.getQueryData(queryKey));
      const check = await checkModelProvider(providerID, payload);
      const checkedProvider = check.provider;
      if (!checkedProvider) return { check };

      const catalog = queryClient.getQueryData<ModelProviderCatalog>(queryKey);
      const current = providerForCheck(catalog);
      if (!catalog || !current || current !== before) {
        // The cache has observed another update since this check started.
        void refetch();
        return { check, savedProvider: current };
      }

      const fetchStatus = queryClient.getQueryState(queryKey)?.fetchStatus;
      const replace = (items: ModelProvider[]) =>
        items.map((item) => (item.id === providerID ? checkedProvider : item));
      queryClient.setQueryData<ModelProviderCatalog>(queryKey, {
        providers: replace(catalog.providers),
        builtinProviders: replace(catalog.builtinProviders),
        customProviders: replace(catalog.customProviders),
      });
      // Restart pending reads so they include the saved check and other changes.
      if (fetchStatus === "fetching" || fetchStatus === "paused") void refetch();
      return { check, savedProvider: checkedProvider };
    },
    [queryClient, refetch],
  );

  return { modelProvidersQuery: query, refreshWorkspaceModelProviders, checkWorkspaceModelProvider };
}
