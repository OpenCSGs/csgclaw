import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import {
  createMCPServerRequest,
  deleteMCPServerRequest,
  fetchMCPServerSourceStatus,
  installRemoteMCPServerRequest,
  probeMCPServerRequest,
  syncMCPServerSource,
  updateMCPServerRequest,
} from "@/api/mcp";
import { resolveHubListSelection } from "@/models/hubSelection";
import { mcpManagedKnowledgeBaseSource, mcpServersFromCatalogResponse } from "@/models/mcp";
import type { MCPProbeResult, MCPServer, MCPServerPayload, MCPServerSourceStatus, RemoteMCPServer } from "@/models/mcp";
import { workspaceQueryKeys, useWorkspaceMCPServersQuery, useWorkspaceRemoteMCPServersQuery } from "./workspaceQueries";
import { isOpenCSGAuthenticationError, type OpenCSGAuthGuard } from "./useOpenCSGAuthGuard";

type HubResourceType = "knowledge" | "template" | "skill" | "mcp";

type MCPServerNameSetter = (value: string | ((current: string) => string)) => void;

type UseWorkspaceMCPSelectionArgs = {
  selectedMCPServerName: string;
  selectedHubResourceType: HubResourceType;
  setSelectedMCPServerName: MCPServerNameSetter;
  setSelectedHubResourceType: (value: HubResourceType) => void;
  skillCount: number;
  skillsLoaded?: boolean;
  t: (key: string) => string;
  templateCount: number;
  templatesLoaded?: boolean;
  enabled?: boolean;
  openCSGAuthGuard: OpenCSGAuthGuard;
};

export function useWorkspaceMCPSelection({
  selectedMCPServerName,
  selectedHubResourceType,
  setSelectedMCPServerName,
  setSelectedHubResourceType,
  skillCount,
  skillsLoaded = false,
  t,
  templateCount,
  templatesLoaded = false,
  enabled = true,
  openCSGAuthGuard,
}: UseWorkspaceMCPSelectionArgs) {
  const {
    authenticated: openCSGAuthenticated,
    handleAuthenticationError: handleOpenCSGAuthenticationError,
    requireAuthentication: requireOpenCSGAuthentication,
  } = openCSGAuthGuard;
  const queryClient = useQueryClient();
  const [mcpCreateDialogOpen, setMCPCreateDialogOpen] = useState(false);
  const [mcpCreateInitialDocument, setMCPCreateInitialDocument] = useState("");
  const [mcpCreateSource, setMCPCreateSource] = useState<"mcp" | "knowledge">("mcp");
  const [knowledgeBaseAdded, setKnowledgeBaseAdded] = useState(false);
  const [mcpAdded, setMCPAdded] = useState(false);
  const [mcpCreateError, setMCPCreateError] = useState("");
  const [mcpMutationBusy, setMCPMutationBusy] = useState(false);
  const [mcpMutationError, setMCPMutationError] = useState("");
  const [mcpProbeBusy, setMCPProbeBusy] = useState(false);
  const [mcpProbeError, setMCPProbeError] = useState("");
  const [mcpProbeResult, setMCPProbeResult] = useState<MCPProbeResult | null>(null);
  const mcpProbeRequestID = useRef(0);
  const [mcpSourceBusy, setMCPSourceBusy] = useState(false);
  const [mcpSourceError, setMCPSourceError] = useState("");
  const [mcpSourceStatus, setMCPSourceStatus] = useState<MCPServerSourceStatus | null>(null);
  const [mcpSourceSyncBusy, setMCPSourceSyncBusy] = useState(false);
  const mcpSourceRequestID = useRef(0);
  const [remoteMCPServersEnabled, setRemoteMCPServersEnabled] = useState(false);
  const [remoteMCPServersSearch, setRemoteMCPServersSearch] = useState("");
  const [remoteMCPServersSearchQuery, setRemoteMCPServersSearchQuery] = useState("");
  const [remoteMCPInstallBusy, setRemoteMCPInstallBusy] = useState("");
  const mcpServersQuery = useWorkspaceMCPServersQuery({ enabled });
  const remoteMCPServersQuery = useWorkspaceRemoteMCPServersQuery(remoteMCPServersSearchQuery, {
    enabled: remoteMCPServersEnabled && openCSGAuthenticated,
  });

  const mcpServers = useMemo(() => mcpServersFromCatalogResponse(mcpServersQuery.data ?? null), [mcpServersQuery.data]);
  const remoteMCPServers = useMemo(() => {
    const pages = remoteMCPServersQuery.data?.pages ?? [];
    const seen = new Set<string>();
    return pages.flatMap((page) =>
      page.items.filter((item) => {
        const key = item.id || item.name;
        if (!key || seen.has(key)) {
          return false;
        }
        seen.add(key);
        return true;
      }),
    );
  }, [remoteMCPServersQuery.data]);
  const selectedMCPServer = useMemo(
    () => resolveHubListSelection(mcpServers, selectedMCPServerName, (item) => item.name),
    [mcpServers, selectedMCPServerName],
  );
  const selectedMCPSource = useMemo(
    () => mcpManagedKnowledgeBaseSource(selectedMCPServer?.config),
    [selectedMCPServer?.config],
  );

  const checkMCPServerSource = useCallback(
    async (name: string, options: { prompt?: boolean } = {}) => {
      const normalizedName = String(name || "").trim();
      if (!normalizedName) {
        return null;
      }
      if (!openCSGAuthenticated) {
        if (options.prompt !== false) {
          requireOpenCSGAuthentication();
        }
        return null;
      }
      const requestID = mcpSourceRequestID.current + 1;
      mcpSourceRequestID.current = requestID;
      setMCPSourceBusy(true);
      setMCPSourceError("");
      try {
        const status = await fetchMCPServerSourceStatus(normalizedName);
        if (mcpSourceRequestID.current === requestID) {
          setMCPSourceStatus(status);
        }
        return status;
      } catch (error) {
        if (mcpSourceRequestID.current === requestID) {
          setMCPSourceStatus(null);
          if (
            !handleOpenCSGAuthenticationError(error, {
              openDialog: options.prompt !== false,
            })
          ) {
            setMCPSourceError(errorMessage(error, t("resourcesMCPSourceCheckFailed")));
          }
        }
        return null;
      } finally {
        if (mcpSourceRequestID.current === requestID) {
          setMCPSourceBusy(false);
        }
      }
    },
    [handleOpenCSGAuthenticationError, openCSGAuthenticated, requireOpenCSGAuthentication, t],
  );

  useEffect(() => {
    if (!mcpServers.length) {
      setSelectedMCPServerName("");
      return;
    }
    setSelectedMCPServerName((current) => (mcpServers.some((item) => item.name === current) ? current : ""));
  }, [mcpServers, setSelectedMCPServerName]);

  useEffect(() => {
    mcpProbeRequestID.current += 1;
    setMCPProbeBusy(false);
    setMCPProbeError("");
    setMCPProbeResult(null);
  }, [selectedMCPServerName]);

  useEffect(() => {
    mcpSourceRequestID.current += 1;
    setMCPSourceBusy(false);
    setMCPSourceError("");
    setMCPSourceStatus(null);
    if (!selectedMCPServerName || !selectedMCPSource || !openCSGAuthenticated) {
      return;
    }
    void checkMCPServerSource(selectedMCPServerName, { prompt: false });
  }, [checkMCPServerSource, openCSGAuthenticated, selectedMCPServerName, selectedMCPSource]);

  useEffect(() => {
    if (selectedHubResourceType === "skill" && skillsLoaded && !skillCount) {
      setSelectedHubResourceType(mcpServers.length ? "mcp" : "template");
      return;
    }
    if (selectedHubResourceType === "template" && templatesLoaded && !templateCount) {
      setSelectedHubResourceType(mcpServers.length ? "mcp" : skillCount ? "skill" : "template");
    }
  }, [
    mcpServers.length,
    selectedHubResourceType,
    setSelectedHubResourceType,
    skillCount,
    skillsLoaded,
    templateCount,
    templatesLoaded,
  ]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setRemoteMCPServersSearchQuery(remoteMCPServersSearch.trim());
    }, 250);
    return () => window.clearTimeout(timer);
  }, [remoteMCPServersSearch]);

  const openCreateMCPDialog = useCallback(
    (initialDocument = "", source: "mcp" | "knowledge" = "mcp") => {
      setMCPCreateSource(source);
      setKnowledgeBaseAdded(false);
      setMCPAdded(false);
      setSelectedHubResourceType(source);
      if (source === "knowledge") setSelectedMCPServerName("");
      setMCPCreateInitialDocument(initialDocument);
      setMCPCreateError("");
      setMCPCreateDialogOpen(true);
    },
    [setSelectedHubResourceType, setSelectedMCPServerName],
  );

  const changeMCPCreateDialogOpen = useCallback((open: boolean) => {
    setMCPCreateError("");
    if (open) {
      setMCPCreateSource("mcp");
      setMCPAdded(false);
    } else {
      setMCPCreateInitialDocument("");
    }
    setMCPCreateDialogOpen(open);
  }, []);

  const createMCPServer = useCallback(
    async (payload: MCPServerPayload) => {
      setMCPMutationBusy(true);
      setMCPCreateError("");
      try {
        const state = await createMCPServerRequest(payload);
        queryClient.setQueryData(workspaceQueryKeys.mcpServers(), state);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
        setSelectedHubResourceType(mcpCreateSource);
        if (mcpCreateSource === "knowledge") {
          setKnowledgeBaseAdded(true);
        } else {
          setSelectedMCPServerName("");
          setMCPAdded(true);
        }
        setMCPCreateDialogOpen(false);
        return true;
      } catch (error) {
        setMCPCreateError(errorMessage(error, t("resourcesMCPSaveFailed")));
        return false;
      } finally {
        setMCPMutationBusy(false);
      }
    },
    [mcpCreateSource, queryClient, setSelectedMCPServerName, setSelectedHubResourceType, t],
  );

  const updateMCPServer = useCallback(
    async (currentName: string, payload: MCPServerPayload) => {
      setMCPMutationBusy(true);
      setMCPMutationError("");
      try {
        const state = await updateMCPServerRequest(currentName, payload);
        queryClient.setQueryData(workspaceQueryKeys.mcpServers(), state);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
        setSelectedHubResourceType("mcp");
        setSelectedMCPServerName(currentName);
        return true;
      } catch (error) {
        setMCPMutationError(errorMessage(error, t("resourcesMCPSaveFailed")));
        return false;
      } finally {
        setMCPMutationBusy(false);
      }
    },
    [queryClient, setSelectedMCPServerName, setSelectedHubResourceType, t],
  );

  const installRemoteMCPServer = useCallback(
    async (item: RemoteMCPServer | null | undefined) => {
      const id = String(item?.id || "").trim();
      if (!id) {
        setMCPMutationError(t("resourcesMCPRemoteInstallFailed"));
        return false;
      }
      if (!requireOpenCSGAuthentication()) {
        return false;
      }
      setMCPMutationBusy(true);
      setMCPMutationError("");
      setRemoteMCPInstallBusy(id);
      try {
        await installRemoteMCPServerRequest(id);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.mcpServers() }),
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() }),
        ]);
        setSelectedHubResourceType("mcp");
        setSelectedMCPServerName("");
        setMCPAdded(true);
        setMCPCreateDialogOpen(false);
        return true;
      } catch (error) {
        if (!handleOpenCSGAuthenticationError(error)) {
          setMCPMutationError(errorMessage(error, t("resourcesMCPRemoteInstallFailed")));
        }
        return false;
      } finally {
        setRemoteMCPInstallBusy("");
        setMCPMutationBusy(false);
      }
    },
    [
      handleOpenCSGAuthenticationError,
      queryClient,
      requireOpenCSGAuthentication,
      setSelectedMCPServerName,
      setSelectedHubResourceType,
      t,
    ],
  );

  const deleteMCPServer = useCallback(
    async (item: MCPServer | null | undefined) => {
      const name = String(item?.name || "").trim();
      if (!name) {
        return false;
      }
      setMCPMutationBusy(true);
      setMCPMutationError("");
      try {
        const state = await deleteMCPServerRequest(name);
        queryClient.setQueryData(workspaceQueryKeys.mcpServers(), state);
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
        setSelectedMCPServerName("");
        return true;
      } catch (error) {
        setMCPMutationError(errorMessage(error, t("resourcesMCPDeleteFailed")));
        return false;
      } finally {
        setMCPMutationBusy(false);
      }
    },
    [queryClient, setSelectedMCPServerName, t],
  );

  const syncSelectedMCPServerSource = useCallback(async () => {
    const name = String(selectedMCPServer?.name || "").trim();
    if (!name || !selectedMCPSource) {
      return false;
    }
    if (!requireOpenCSGAuthentication()) {
      return false;
    }
    setMCPSourceSyncBusy(true);
    setMCPSourceError("");
    try {
      const result = await syncMCPServerSource(name);
      queryClient.setQueryData(workspaceQueryKeys.mcpServers(), result.state);
      setMCPSourceStatus(result.source);
      if (result.source.globalServerName) {
        setSelectedMCPServerName(result.source.globalServerName);
      }
      await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
      return true;
    } catch (error) {
      if (!handleOpenCSGAuthenticationError(error)) {
        setMCPSourceError(errorMessage(error, t("resourcesMCPSourceSyncFailed")));
      }
      return false;
    } finally {
      setMCPSourceSyncBusy(false);
    }
  }, [
    handleOpenCSGAuthenticationError,
    queryClient,
    requireOpenCSGAuthentication,
    selectedMCPServer?.name,
    selectedMCPSource,
    setSelectedMCPServerName,
    t,
  ]);

  const clearMCPProbe = useCallback(() => {
    mcpProbeRequestID.current += 1;
    setMCPProbeBusy(false);
    setMCPProbeError("");
    setMCPProbeResult(null);
  }, []);

  const probeMCPServer = useCallback(
    async (payload: MCPServerPayload) => {
      const knowledgeBaseSource = mcpManagedKnowledgeBaseSource(payload.config);
      if (knowledgeBaseSource && !requireOpenCSGAuthentication()) {
        return null;
      }
      const requestID = mcpProbeRequestID.current + 1;
      mcpProbeRequestID.current = requestID;
      setMCPProbeBusy(true);
      setMCPProbeError("");
      setMCPProbeResult(null);
      try {
        const result = await probeMCPServerRequest(payload);
        if (mcpProbeRequestID.current === requestID) {
          setMCPProbeResult(result);
        }
        return result;
      } catch (error) {
        const intercepted = Boolean(knowledgeBaseSource && handleOpenCSGAuthenticationError(error));
        if (mcpProbeRequestID.current === requestID) {
          if (!intercepted) {
            setMCPProbeError(errorMessage(error, t("resourcesMCPTestFailed")));
          }
        }
        if (knowledgeBaseSource && !intercepted) {
          void checkMCPServerSource(payload.name, { prompt: false });
        }
        return null;
      } finally {
        if (mcpProbeRequestID.current === requestID) {
          setMCPProbeBusy(false);
        }
      }
    },
    [checkMCPServerSource, handleOpenCSGAuthenticationError, requireOpenCSGAuthentication, t],
  );

  const rawMCPServersError = mcpServersQuery.error
    ? errorMessage(mcpServersQuery.error, t("resourcesMCPLoadFailed"))
    : "";
  const mcpStateError = selectedHubResourceType === "mcp" ? rawMCPServersError : "";
  const remoteMCPServersError =
    remoteMCPServersEnabled && remoteMCPServersQuery.error && !isOpenCSGAuthenticationError(remoteMCPServersQuery.error)
      ? errorMessage(remoteMCPServersQuery.error, t("resourcesMCPRemoteServersLoadFailed"))
      : "";
  const loadMoreRemoteMCPServers = useCallback(async () => {
    if (
      !remoteMCPServersEnabled ||
      !requireOpenCSGAuthentication() ||
      !remoteMCPServersQuery.hasNextPage ||
      remoteMCPServersQuery.isFetchingNextPage
    ) {
      return;
    }
    await remoteMCPServersQuery.fetchNextPage();
  }, [remoteMCPServersEnabled, remoteMCPServersQuery, requireOpenCSGAuthentication]);

  const changeRemoteMCPServersEnabled = useCallback(
    (visible: boolean) => {
      if (visible && !requireOpenCSGAuthentication()) {
        return;
      }
      setRemoteMCPServersEnabled(visible);
    },
    [requireOpenCSGAuthentication],
  );

  const refreshRemoteMCPServers = useCallback(async () => {
    if (!requireOpenCSGAuthentication()) {
      return;
    }
    const result = await remoteMCPServersQuery.refetch();
    if (result.error) {
      handleOpenCSGAuthenticationError(result.error);
    }
    return result;
  }, [handleOpenCSGAuthenticationError, remoteMCPServersQuery, requireOpenCSGAuthentication]);

  useEffect(() => {
    if (remoteMCPServersQuery.error) {
      handleOpenCSGAuthenticationError(remoteMCPServersQuery.error);
    }
  }, [handleOpenCSGAuthenticationError, remoteMCPServersQuery.error]);

  return {
    clearMCPProbe,
    createMCPServer,
    deleteMCPServer,
    installRemoteMCPServer,
    mcpServersFetching: mcpServersQuery.isFetching,
    mcpServersLoaded: mcpServersQuery.isFetched,
    mcpServers,
    mcpCreateError,
    mcpCreateSource,
    knowledgeBaseAdded,
    mcpAdded,
    mcpCreateDialogOpen,
    mcpCreateInitialDocument,
    mcpMutationBusy,
    mcpMutationError,
    mcpProbeBusy,
    mcpProbeError,
    mcpProbeResult,
    mcpSourceBusy,
    mcpSourceError,
    mcpSourceStatus,
    mcpSourceSyncBusy,
    mcpStateError,
    openCreateMCPDialog,
    loadMoreRemoteMCPServers,
    refetchRemoteMCPServers: refreshRemoteMCPServers,
    refetchMCPServers: mcpServersQuery.refetch,
    remoteMCPInstallBusy,
    remoteMCPServers,
    remoteMCPServersError,
    remoteMCPServersHasMore: Boolean(remoteMCPServersQuery.hasNextPage),
    remoteMCPServersLoading:
      remoteMCPServersEnabled && remoteMCPServersQuery.isFetching && !remoteMCPServersQuery.isFetchingNextPage,
    remoteMCPServersLoadingMore: remoteMCPServersQuery.isFetchingNextPage,
    remoteMCPServersSearch,
    setRemoteMCPServersEnabled: changeRemoteMCPServersEnabled,
    setRemoteMCPServersSearch,
    selectedMCPServer,
    setMCPCreateDialogOpen: changeMCPCreateDialogOpen,
    probeMCPServer,
    checkSelectedMCPServerSource: () => checkMCPServerSource(selectedMCPServer?.name || ""),
    syncSelectedMCPServerSource,
    updateMCPServer,
  };
}
