import { useCallback, useEffect, useMemo, useState } from "react";
import { errorMessage } from "@/api/client";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { configuredKnowledgeBases, mergeRemoteKnowledgeBasePages } from "@/models/knowledgeBases";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { formatMCPServerDocument } from "@/models/mcp";
import { useWorkspaceKnowledgeBasesQuery } from "./workspaceQueries";

type KnowledgeBaseIDSetter = (value: string | ((current: string) => string)) => void;

type UseWorkspaceKnowledgeBaseSelectionArgs = {
  authenticated: boolean;
  enabled: boolean;
  openCreateMCPDialog: (initialDocument?: string) => void;
  selectedKnowledgeBaseID: string;
  setSelectedKnowledgeBaseID: KnowledgeBaseIDSetter;
  t: (key: string) => string;
};

export function useWorkspaceKnowledgeBaseSelection({
  authenticated,
  enabled,
  openCreateMCPDialog,
  selectedKnowledgeBaseID,
  setSelectedKnowledgeBaseID,
  t,
}: UseWorkspaceKnowledgeBaseSelectionArgs) {
  const [search, setSearch] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [copyBusyID, setCopyBusyID] = useState("");
  const [copyError, setCopyError] = useState("");
  const [pendingMCPKnowledgeBase, setPendingMCPKnowledgeBase] = useState<RemoteKnowledgeBase | null>(null);
  const catalogQuery = useWorkspaceKnowledgeBasesQuery("", { enabled: enabled && authenticated });
  const discoveryQuery = useWorkspaceKnowledgeBasesQuery(searchQuery, { enabled: enabled && authenticated });
  const catalogItems = useMemo(
    () => mergeRemoteKnowledgeBasePages(catalogQuery.data?.pages ?? []),
    [catalogQuery.data?.pages],
  );
  const discoveryItems = useMemo(
    () => mergeRemoteKnowledgeBasePages(discoveryQuery.data?.pages ?? []),
    [discoveryQuery.data?.pages],
  );
  const items = useMemo(() => configuredKnowledgeBases(catalogItems), [catalogItems]);
  const selected = useMemo(
    () => items.find((item) => item.id === selectedKnowledgeBaseID) || items[0] || null,
    [items, selectedKnowledgeBaseID],
  );

  useEffect(() => {
    const timer = window.setTimeout(() => setSearchQuery(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

  const catalogPageCount = catalogQuery.data?.pages.length ?? 0;
  const fetchNextCatalogPage = catalogQuery.fetchNextPage;
  const catalogHasNextPage = catalogQuery.hasNextPage;
  const catalogIsFetchNextPageError = catalogQuery.isFetchNextPageError;
  const catalogIsFetchingNextPage = catalogQuery.isFetchingNextPage;
  useEffect(() => {
    if (!enabled || !authenticated || !catalogHasNextPage || catalogIsFetchingNextPage || catalogIsFetchNextPageError) {
      return;
    }
    void fetchNextCatalogPage();
  }, [
    authenticated,
    catalogPageCount,
    catalogHasNextPage,
    catalogIsFetchNextPageError,
    catalogIsFetchingNextPage,
    enabled,
    fetchNextCatalogPage,
  ]);

  useEffect(() => {
    if (!items.length) {
      setSelectedKnowledgeBaseID("");
      return;
    }
    setSelectedKnowledgeBaseID((current) => (items.some((item) => item.id === current) ? current : items[0]?.id || ""));
  }, [items, setSelectedKnowledgeBaseID]);

  const prepareMCPConfig = useCallback(
    async (id: string) => {
      const normalizedID = String(id || "").trim();
      if (!normalizedID) {
        return false;
      }
      setCopyBusyID(normalizedID);
      setCopyError("");
      try {
        const result = await fetchRemoteKnowledgeBaseMCPConfig(normalizedID);
        openCreateMCPDialog(formatMCPServerDocument(result.name, result.config));
        return true;
      } catch (error) {
        setCopyError(errorMessage(error, t("resourcesKnowledgeBaseConfigFailed")));
        await Promise.all([catalogQuery.refetch(), discoveryQuery.refetch()]);
        return false;
      } finally {
        setCopyBusyID("");
      }
    },
    [catalogQuery, discoveryQuery, openCreateMCPDialog, t],
  );

  const requestMCPConfig = useCallback(
    async (id: string) => {
      const normalizedID = String(id || "").trim();
      const item = [...catalogItems, ...discoveryItems].find((candidate) => candidate.id === normalizedID);
      if (!item || item.availability !== "available" || item.configuredMCPName) {
        return false;
      }
      setCopyError("");
      setPendingMCPKnowledgeBase(item);
      return true;
    },
    [catalogItems, discoveryItems],
  );

  const cancelMCPConfig = useCallback(() => {
    setPendingMCPKnowledgeBase(null);
  }, []);

  const confirmMCPConfig = useCallback(async () => {
    if (!pendingMCPKnowledgeBase) {
      return false;
    }
    const prepared = await prepareMCPConfig(pendingMCPKnowledgeBase.id);
    if (prepared) {
      setPendingMCPKnowledgeBase(null);
    }
    return prepared;
  }, [pendingMCPKnowledgeBase, prepareMCPConfig]);

  const loginRequired = enabled && !authenticated;
  const loginError = loginRequired ? t("resourcesKnowledgeBasesLoginRequired") : "";
  const fetchNextDiscoveryPage = discoveryQuery.fetchNextPage;
  const discoveryHasNextPage = discoveryQuery.hasNextPage;
  const discoveryIsFetchingNextPage = discoveryQuery.isFetchingNextPage;
  const loadMoreDiscovery = useCallback(async () => {
    if (!discoveryHasNextPage || discoveryIsFetchingNextPage) {
      return;
    }
    await fetchNextDiscoveryPage();
  }, [discoveryHasNextPage, discoveryIsFetchingNextPage, fetchNextDiscoveryPage]);

  return {
    copyBusyID,
    copyError,
    cancelMCPConfig,
    confirmMCPConfig,
    discoveryItems,
    discoveryHasMore: Boolean(discoveryHasNextPage),
    discoveryLoadError:
      loginError ||
      (discoveryQuery.error ? errorMessage(discoveryQuery.error, t("resourcesKnowledgeBasesLoadFailed")) : ""),
    discoveryLoading: enabled && authenticated && discoveryQuery.isFetching && !discoveryIsFetchingNextPage,
    discoveryLoadingMore: discoveryIsFetchingNextPage,
    discoveryLoadMore: loadMoreDiscovery,
    discoveryRefetch: discoveryQuery.refetch,
    items,
    loginRequired,
    loading: enabled && authenticated && catalogQuery.isFetching,
    loadError:
      loginError ||
      (catalogQuery.error ? errorMessage(catalogQuery.error, t("resourcesKnowledgeBasesLoadFailed")) : ""),
    pendingMCPKnowledgeBase,
    refetch: catalogQuery.refetch,
    requestMCPConfig,
    search,
    selected,
    setSearch,
  };
}
