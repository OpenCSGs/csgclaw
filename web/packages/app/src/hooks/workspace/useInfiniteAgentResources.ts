import { useEffect, useMemo } from "react";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { apiErrorCode } from "@/api/client";
import type { ResourceBatch } from "@/api/agentResources";

export type ResourceContinuation = {
  total: number;
  loading: boolean;
  failed: boolean;
  hasMore: boolean;
  loadMore: () => Promise<boolean>;
  retry: () => void;
};

export function useInfiniteAgentResources<T>(
  queryKey: readonly string[],
  fetchBatch: (cursor: string, signal: AbortSignal) => Promise<ResourceBatch<T>>,
  enabled: boolean,
) {
  const client = useQueryClient();
  const query = useInfiniteQuery({
    queryKey,
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) => fetchBatch(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled,
    retry: (count, error) => apiErrorCode(error) !== "resource_list_changed" && count < 1,
  });
  const changed = apiErrorCode(query.error) === "resource_list_changed";
  useEffect(() => {
    if (changed) void client.resetQueries({ queryKey, exact: true });
  }, [changed, client, queryKey]);
  const items = useMemo(() => query.data?.pages.flatMap((page) => page.items) ?? [], [query.data]);
  const continuation: ResourceContinuation = {
    total: query.data?.pages[0]?.total ?? 0,
    loading: query.isFetching,
    failed: query.isError && !changed,
    hasMore: Boolean(query.hasNextPage),
    loadMore: async () => {
      if (!enabled || query.isFetching || !query.hasNextPage) return false;
      const result = await query.fetchNextPage({ cancelRefetch: false });
      return !result.isError;
    },
    retry: () => {
      if (query.hasNextPage) void query.fetchNextPage({ cancelRefetch: false });
      else void query.refetch();
    },
  };
  return { query, items, continuation, changed };
}
