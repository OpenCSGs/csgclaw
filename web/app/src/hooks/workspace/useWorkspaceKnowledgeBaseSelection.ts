import { useCallback, useEffect, useMemo, useState } from "react";
import { errorMessage } from "@/api/client";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { configuredKnowledgeBases } from "@/models/knowledgeBases";
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
  const catalogQuery = useWorkspaceKnowledgeBasesQuery("", { enabled: enabled && authenticated });
  const discoveryQuery = useWorkspaceKnowledgeBasesQuery(searchQuery, { enabled: enabled && authenticated });
  const catalogItems = useMemo(() => catalogQuery.data?.items ?? [], [catalogQuery.data?.items]);
  const discoveryItems = useMemo(() => discoveryQuery.data?.items ?? [], [discoveryQuery.data?.items]);
  const items = useMemo(() => configuredKnowledgeBases(catalogItems), [catalogItems]);
  const selected = useMemo(
    () => items.find((item) => item.id === selectedKnowledgeBaseID) || items[0] || null,
    [items, selectedKnowledgeBaseID],
  );

  useEffect(() => {
    const timer = window.setTimeout(() => setSearchQuery(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

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

  const loginRequired = enabled && !authenticated;
  const loginError = loginRequired ? t("resourcesKnowledgeBasesLoginRequired") : "";

  return {
    copyBusyID,
    copyError,
    discoveryItems,
    discoveryLoadError:
      loginError ||
      (discoveryQuery.error ? errorMessage(discoveryQuery.error, t("resourcesKnowledgeBasesLoadFailed")) : ""),
    discoveryLoading: enabled && authenticated && discoveryQuery.isFetching,
    discoveryRefetch: discoveryQuery.refetch,
    items,
    loginRequired,
    loading: enabled && authenticated && catalogQuery.isFetching,
    loadError:
      loginError ||
      (catalogQuery.error ? errorMessage(catalogQuery.error, t("resourcesKnowledgeBasesLoadFailed")) : ""),
    prepareMCPConfig,
    refetch: catalogQuery.refetch,
    search,
    selected,
    setSearch,
  };
}
