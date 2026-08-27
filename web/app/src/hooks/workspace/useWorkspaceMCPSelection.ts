import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import {
  createMCPServerRequest,
  deleteMCPServerRequest,
  installRemoteMCPServerRequest,
  probeMCPServerRequest,
  updateMCPServerRequest,
} from "@/api/mcp";
import { mcpServersFromCatalogResponse } from "@/models/mcp";
import type { MCPProbeResult, MCPServer, MCPServerPayload, RemoteMCPServer } from "@/models/mcp";
import { workspaceQueryKeys, useWorkspaceMCPServersQuery, useWorkspaceRemoteMCPServersQuery } from "./workspaceQueries";

type HubResourceType = "knowledge" | "template" | "skill" | "mcp";

type MCPServerNameSetter = (value: string | ((current: string) => string)) => void;

type UseWorkspaceMCPSelectionArgs = {
  selectedMCPServerName: string;
  selectedHubResourceType: HubResourceType;
  setSelectedMCPServerName: MCPServerNameSetter;
  setSelectedHubResourceType: (value: HubResourceType) => void;
  skillCount: number;
  t: (key: string) => string;
  templateCount: number;
};

export function useWorkspaceMCPSelection({
  selectedMCPServerName,
  selectedHubResourceType,
  setSelectedMCPServerName,
  setSelectedHubResourceType,
  skillCount,
  t,
  templateCount,
}: UseWorkspaceMCPSelectionArgs) {
  const queryClient = useQueryClient();
  const [mcpCreateDialogOpen, setMCPCreateDialogOpen] = useState(false);
  const [mcpCreateInitialDocument, setMCPCreateInitialDocument] = useState("");
  const [mcpCreateError, setMCPCreateError] = useState("");
  const [mcpMutationBusy, setMCPMutationBusy] = useState(false);
  const [mcpMutationError, setMCPMutationError] = useState("");
  const [mcpProbeBusy, setMCPProbeBusy] = useState(false);
  const [mcpProbeError, setMCPProbeError] = useState("");
  const [mcpProbeResult, setMCPProbeResult] = useState<MCPProbeResult | null>(null);
  const mcpProbeRequestID = useRef(0);
  const [remoteMCPServersEnabled, setRemoteMCPServersEnabled] = useState(false);
  const [remoteMCPServersSearch, setRemoteMCPServersSearch] = useState("");
  const [remoteMCPServersSearchQuery, setRemoteMCPServersSearchQuery] = useState("");
  const [remoteMCPInstallBusy, setRemoteMCPInstallBusy] = useState("");
  const mcpServersQuery = useWorkspaceMCPServersQuery();
  const remoteMCPServersQuery = useWorkspaceRemoteMCPServersQuery(remoteMCPServersSearchQuery, {
    enabled: remoteMCPServersEnabled,
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
    () => mcpServers.find((item) => item.name === selectedMCPServerName) || mcpServers[0] || null,
    [mcpServers, selectedMCPServerName],
  );

  useEffect(() => {
    if (!mcpServers.length) {
      setSelectedMCPServerName("");
      return;
    }
    setSelectedMCPServerName((current) =>
      mcpServers.some((item) => item.name === current) ? current : mcpServers[0]?.name || "",
    );
  }, [mcpServers, setSelectedMCPServerName]);

  useEffect(() => {
    mcpProbeRequestID.current += 1;
    setMCPProbeBusy(false);
    setMCPProbeError("");
    setMCPProbeResult(null);
  }, [selectedMCPServerName]);

  useEffect(() => {
    if (selectedHubResourceType === "skill" && !skillCount) {
      setSelectedHubResourceType(mcpServers.length ? "mcp" : "template");
      return;
    }
    if (selectedHubResourceType === "template" && !templateCount) {
      setSelectedHubResourceType(mcpServers.length ? "mcp" : skillCount ? "skill" : "template");
    }
  }, [mcpServers.length, selectedHubResourceType, setSelectedHubResourceType, skillCount, templateCount]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setRemoteMCPServersSearchQuery(remoteMCPServersSearch.trim());
    }, 250);
    return () => window.clearTimeout(timer);
  }, [remoteMCPServersSearch]);

  const openCreateMCPDialog = useCallback(
    (initialDocument = "") => {
      setSelectedHubResourceType("mcp");
      setMCPCreateInitialDocument(initialDocument);
      setMCPCreateError("");
      setMCPCreateDialogOpen(true);
    },
    [setSelectedHubResourceType],
  );

  const changeMCPCreateDialogOpen = useCallback((open: boolean) => {
    setMCPCreateError("");
    if (!open) {
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
        setSelectedHubResourceType("mcp");
        setSelectedMCPServerName(payload.name);
        setMCPCreateDialogOpen(false);
        return true;
      } catch (error) {
        setMCPCreateError(errorMessage(error, t("resourcesMCPSaveFailed")));
        return false;
      } finally {
        setMCPMutationBusy(false);
      }
    },
    [queryClient, setSelectedMCPServerName, setSelectedHubResourceType, t],
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
        setSelectedMCPServerName(payload.name);
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
      setMCPMutationBusy(true);
      setMCPMutationError("");
      setRemoteMCPInstallBusy(id);
      try {
        const name = await installRemoteMCPServerRequest(id);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.mcpServers() }),
          queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() }),
        ]);
        setSelectedHubResourceType("mcp");
        setSelectedMCPServerName(name);
        setMCPCreateDialogOpen(false);
        return true;
      } catch (error) {
        setMCPMutationError(errorMessage(error, t("resourcesMCPRemoteInstallFailed")));
        return false;
      } finally {
        setRemoteMCPInstallBusy("");
        setMCPMutationBusy(false);
      }
    },
    [queryClient, setSelectedMCPServerName, setSelectedHubResourceType, t],
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
        setSelectedHubResourceType("mcp");
        return true;
      } catch (error) {
        setMCPMutationError(errorMessage(error, t("resourcesMCPDeleteFailed")));
        return false;
      } finally {
        setMCPMutationBusy(false);
      }
    },
    [queryClient, setSelectedMCPServerName, setSelectedHubResourceType, t],
  );

  const clearMCPProbe = useCallback(() => {
    mcpProbeRequestID.current += 1;
    setMCPProbeBusy(false);
    setMCPProbeError("");
    setMCPProbeResult(null);
  }, []);

  const probeMCPServer = useCallback(
    async (payload: MCPServerPayload) => {
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
        if (mcpProbeRequestID.current === requestID) {
          setMCPProbeError(errorMessage(error, t("resourcesMCPTestFailed")));
        }
        return null;
      } finally {
        if (mcpProbeRequestID.current === requestID) {
          setMCPProbeBusy(false);
        }
      }
    },
    [t],
  );

  const rawMCPServersError = mcpServersQuery.error
    ? errorMessage(mcpServersQuery.error, t("resourcesMCPLoadFailed"))
    : "";
  const mcpStateError = selectedHubResourceType === "mcp" ? rawMCPServersError : "";
  const remoteMCPServersError =
    remoteMCPServersEnabled && remoteMCPServersQuery.error
      ? errorMessage(remoteMCPServersQuery.error, t("resourcesMCPRemoteServersLoadFailed"))
      : "";
  const loadMoreRemoteMCPServers = useCallback(async () => {
    if (!remoteMCPServersEnabled || !remoteMCPServersQuery.hasNextPage || remoteMCPServersQuery.isFetchingNextPage) {
      return;
    }
    await remoteMCPServersQuery.fetchNextPage();
  }, [remoteMCPServersEnabled, remoteMCPServersQuery]);

  return {
    clearMCPProbe,
    createMCPServer,
    deleteMCPServer,
    installRemoteMCPServer,
    mcpServersFetching: mcpServersQuery.isFetching,
    mcpServers,
    mcpCreateError,
    mcpCreateDialogOpen,
    mcpCreateInitialDocument,
    mcpMutationBusy,
    mcpMutationError,
    mcpProbeBusy,
    mcpProbeError,
    mcpProbeResult,
    mcpStateError,
    openCreateMCPDialog,
    loadMoreRemoteMCPServers,
    refetchRemoteMCPServers: remoteMCPServersQuery.refetch,
    refetchMCPServers: mcpServersQuery.refetch,
    remoteMCPInstallBusy,
    remoteMCPServers,
    remoteMCPServersError,
    remoteMCPServersHasMore: Boolean(remoteMCPServersQuery.hasNextPage),
    remoteMCPServersLoading:
      remoteMCPServersEnabled && remoteMCPServersQuery.isFetching && !remoteMCPServersQuery.isFetchingNextPage,
    remoteMCPServersLoadingMore: remoteMCPServersQuery.isFetchingNextPage,
    remoteMCPServersSearch,
    setRemoteMCPServersEnabled,
    setRemoteMCPServersSearch,
    selectedMCPServer,
    setMCPCreateDialogOpen: changeMCPCreateDialogOpen,
    probeMCPServer,
    updateMCPServer,
  };
}
