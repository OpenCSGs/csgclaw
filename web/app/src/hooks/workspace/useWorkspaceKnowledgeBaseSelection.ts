import { useCallback, useEffect, useMemo, useState } from "react";
import { errorMessage } from "@/api/client";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { resolveHubListSelection } from "@/models/hubSelection";
import { configuredKnowledgeBases, mergeRemoteKnowledgeBasePages } from "@/models/knowledgeBases";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { formatMCPServerDocument } from "@/models/mcp";
import { useWorkspaceKnowledgeBasesQuery } from "./workspaceQueries";

// TODO: remove mock data before production
const MOCK_KNOWLEDGE_BASES: RemoteKnowledgeBase[] = [
  {
    id: "mock-kb-1",
    contentID: "mock-content-1",
    name: "OpenCSG 文档中心",
    description: "OpenCSG 平台核心文档与 API 参考知识库",
    availability: "available",
  },
  {
    id: "mock-kb-2",
    contentID: "mock-content-2",
    name: "Go 语言规范",
    description: "Go 编程语言完整规范与最佳实践",
    availability: "available",
  },
  {
    id: "mock-kb-3",
    contentID: "mock-content-3",
    name: "React 18 设计模式",
    description: "React 18 并发特性、Suspense 与 Server Components 实战指南",
    availability: "available",
  },
  {
    id: "mock-kb-4",
    contentID: "mock-content-4",
    name: "MCP 协议手册",
    description: "Model Context Protocol 服务端与客户端开发完整参考",
    availability: "unavailable",
    unavailableReason: "需要升级到专业版",
  },
];

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
  const items = useMemo(
    () => {
      const real = configuredKnowledgeBases(catalogItems);
      // TODO: remove mock injection before production
      if (real.length) return real;
      return MOCK_KNOWLEDGE_BASES;
    },
    [catalogItems],
  );
  const selected = useMemo(
    () => resolveHubListSelection(items, selectedKnowledgeBaseID, (item) => item.id),
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
    setSelectedKnowledgeBaseID((current) => (items.some((item) => item.id === current) ? current : ""));
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
