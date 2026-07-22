import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { fetchHubWorkspace, fetchHubWorkspaceFile, updateHubWorkspaceFile } from "@/api/hub";
import { hasSkillName, isOfficialSkill, isPersonalSkill } from "@/models/skillhub";
import type { HubWorkspaceFile } from "@/models/hubWorkspace";
import { flattenWorkspaceDirectoryListings } from "@/models/workspace";
import type { WorkspaceDirectoryListings } from "@/models/workspace";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useWorkspaceMCPSelection } from "./useWorkspaceMCPSelection";
import {
  workspaceQueryKeys,
  useWorkspaceHubTemplateQuery,
  useWorkspaceHubWorkspaceQuery,
  useWorkspaceHubWorkspaceFileQuery,
  useWorkspaceOfficialSkillsQuery,
  useWorkspaceSkillFileQuery,
  useWorkspaceSkillsQuery,
  useWorkspaceSkillTreeQuery,
} from "./workspaceQueries";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { UseWorkspaceHubSelectionArgs } from "./types";

export function useWorkspaceHubSelection({
  templates,
  templatesQuery,
  loaded,
  manualError = "",
  refreshTemplates,
  t,
}: UseWorkspaceHubSelectionArgs) {
  const queryClient = useQueryClient();
  const resourcesTemplates = useMemo(() => templates ?? [], [templates]);
  const selectedHubTemplateId = useWorkspaceUiStore((state) => state.selectedHubTemplateId);
  const setSelectedHubTemplateId = useWorkspaceUiStore((state) => state.setSelectedHubTemplateId);
  const selectedHubWorkspacePath = useWorkspaceUiStore((state) => state.selectedHubWorkspacePath);
  const setSelectedHubWorkspacePath = useWorkspaceUiStore((state) => state.setSelectedHubWorkspacePath);
  const selectedHubSkillName = useWorkspaceUiStore((state) => state.selectedHubSkillName);
  const setSelectedHubSkillName = useWorkspaceUiStore((state) => state.setSelectedHubSkillName);
  const selectedHubSkillPath = useWorkspaceUiStore((state) => state.selectedHubSkillPath);
  const setSelectedHubSkillPath = useWorkspaceUiStore((state) => state.setSelectedHubSkillPath);
  const selectedMCPServerName = useWorkspaceUiStore((state) => state.selectedMCPServerName);
  const setSelectedMCPServerName = useWorkspaceUiStore((state) => state.setSelectedMCPServerName);
  const selectedHubResourceType = useWorkspaceUiStore((state) => state.selectedHubResourceType);
  const setSelectedHubResourceType = useWorkspaceUiStore((state) => state.setSelectedHubResourceType);
  const [remoteSkillsEnabled, setRemoteSkillsEnabled] = useState(false);
  const [remoteSkillsSearch, setRemoteSkillsSearch] = useState("");
  const [remoteSkillsSearchQuery, setRemoteSkillsSearchQuery] = useState("");
  const skillsQuery = useWorkspaceSkillsQuery();
  const officialSkillsQuery = useWorkspaceOfficialSkillsQuery(remoteSkillsSearchQuery, {
    enabled: remoteSkillsEnabled,
  });
  const skills = useMemo(
    () => (skillsQuery.data ?? []).filter((item) => !isOfficialSkill(item) && !isPersonalSkill(item)),
    [skillsQuery.data],
  );
  const remoteSkills = useMemo(() => {
    const pages = officialSkillsQuery.data?.pages ?? [];
    const seen = new Set<string>();
    return pages.flatMap((page) =>
      page.items.filter((item) => {
        const key = item.remotePath || item.name;
        if (!key || seen.has(key)) {
          return false;
        }
        seen.add(key);
        return true;
      }),
    );
  }, [officialSkillsQuery.data]);
  const selectedHubTemplate = useMemo(
    () => resourcesTemplates.find((item) => item.id === selectedHubTemplateId) || resourcesTemplates[0] || null,
    [resourcesTemplates, selectedHubTemplateId],
  );
  const selectedHubSkill = useMemo(
    () => skills.find((item) => item.name === selectedHubSkillName) || skills[0] || null,
    [selectedHubSkillName, skills],
  );
  const [workspaceListingsState, setWorkspaceListingsState] = useState<{
    templateID: string;
    listings: WorkspaceDirectoryListings;
  }>({ templateID: "", listings: {} });
  const [loadingWorkspaceDirs, setLoadingWorkspaceDirs] = useState<ReadonlySet<string>>(new Set());
  const [workspaceDirectoryError, setWorkspaceDirectoryError] = useState("");
  const [templateWorkspaceFilesState, setTemplateWorkspaceFilesState] = useState<{
    files: Record<string, HubWorkspaceFile>;
    templateID: string;
  }>({ files: {}, templateID: "" });

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setRemoteSkillsSearchQuery(remoteSkillsSearch.trim());
    }, 250);
    return () => window.clearTimeout(timer);
  }, [remoteSkillsSearch]);

  useEffect(() => {
    if (!resourcesTemplates.length) {
      setSelectedHubTemplateId("");
      setSelectedHubWorkspacePath("");
      return;
    }
    setSelectedHubTemplateId((current) =>
      resourcesTemplates.some((item) => item.id === current) ? current : (resourcesTemplates[0]?.id ?? ""),
    );
  }, [resourcesTemplates, setSelectedHubTemplateId, setSelectedHubWorkspacePath]);

  useEffect(() => {
    setSelectedHubWorkspacePath("");
    setTemplateWorkspaceFilesState({ files: {}, templateID: selectedHubTemplateId });
  }, [selectedHubTemplateId, setSelectedHubWorkspacePath]);

  useEffect(() => {
    if (!skills.length) {
      setSelectedHubSkillName("");
      setSelectedHubSkillPath("");
      return;
    }
    setSelectedHubSkillName((current) => (hasSkillName(skills, current) ? current : (skills[0]?.name ?? "")));
  }, [setSelectedHubSkillName, setSelectedHubSkillPath, skills]);

  useEffect(() => {
    if (selectedHubSkillName) {
      setSelectedHubSkillPath(`${selectedHubSkillName}/SKILL.md`);
    } else {
      setSelectedHubSkillPath("");
    }
  }, [selectedHubSkillName, setSelectedHubSkillPath]);

  const hubTemplateDetailQuery = useWorkspaceHubTemplateQuery(selectedHubTemplateId);
  const hubWorkspaceQuery = useWorkspaceHubWorkspaceQuery(selectedHubTemplateId);
  const hubWorkspaceFileQuery = useWorkspaceHubWorkspaceFileQuery(selectedHubTemplateId, selectedHubWorkspacePath);
  const skillTreeQuery = useWorkspaceSkillTreeQuery(selectedHubSkillName);
  const skillFileQuery = useWorkspaceSkillFileQuery(selectedHubSkillPath);
  const refetchHubTemplateDetail = hubTemplateDetailQuery.refetch;
  const refetchHubWorkspace = hubWorkspaceQuery.refetch;
  const refetchSkills = skillsQuery.refetch;
  const refetchRemoteSkills = officialSkillsQuery.refetch;
  const refetchSkillTree = skillTreeQuery.refetch;
  const loadMoreRemoteSkills = useCallback(async () => {
    if (!remoteSkillsEnabled || !officialSkillsQuery.hasNextPage || officialSkillsQuery.isFetchingNextPage) {
      return;
    }
    await officialSkillsQuery.fetchNextPage();
  }, [officialSkillsQuery, remoteSkillsEnabled]);

  const selectedHubTemplateView =
    hubTemplateDetailQuery.data?.id === selectedHubTemplateId ? hubTemplateDetailQuery.data : selectedHubTemplate;
  const workspaceListings = useMemo(
    () => (workspaceListingsState.templateID === selectedHubTemplateId ? workspaceListingsState.listings : {}),
    [selectedHubTemplateId, workspaceListingsState],
  );
  const workspaceEntries = useMemo(() => flattenWorkspaceDirectoryListings(workspaceListings), [workspaceListings]);

  useEffect(() => {
    if (!selectedHubTemplateId) {
      setTemplateWorkspaceFilesState({ files: {}, templateID: "" });
      return;
    }
    const desiredPaths = [
      "instructions/AGENTS.md",
      ...workspaceEntries
        .filter(
          (entry) =>
            entry.type === "file" &&
            (entry.path === "mcps/mcp.json" || (entry.path.startsWith("skills/") && entry.path.endsWith("/SKILL.md"))),
        )
        .map((entry) => entry.path),
    ];
    if (!desiredPaths.length) {
      return;
    }
    const state = templateWorkspaceFilesState.templateID === selectedHubTemplateId ? templateWorkspaceFilesState : null;
    const missingPaths = desiredPaths.filter((path) => !state?.files[path]);
    if (!missingPaths.length) {
      return;
    }
    let canceled = false;
    void Promise.all(
      missingPaths.map(async (path) => {
        try {
          const file = await fetchHubWorkspaceFile(selectedHubTemplateId, path);
          return [path, file] as const;
        } catch {
          return null;
        }
      }),
    ).then((items) => {
      if (canceled) {
        return;
      }
      setTemplateWorkspaceFilesState((current) => {
        const files = current.templateID === selectedHubTemplateId ? current.files : {};
        const nextFiles = { ...files };
        items.forEach((item) => {
          if (item) {
            nextFiles[item[0]] = item[1];
          }
        });
        return { files: nextFiles, templateID: selectedHubTemplateId };
      });
    });
    return () => {
      canceled = true;
    };
  }, [selectedHubTemplateId, templateWorkspaceFilesState, workspaceEntries]);

  useEffect(() => {
    setWorkspaceListingsState({ templateID: selectedHubTemplateId, listings: {} });
    setLoadingWorkspaceDirs(new Set());
    setWorkspaceDirectoryError("");
  }, [selectedHubTemplateId]);

  useEffect(() => {
    if (!selectedHubTemplateId || !hubWorkspaceQuery.data) {
      return;
    }
    setWorkspaceListingsState({
      templateID: selectedHubTemplateId,
      listings: { "": hubWorkspaceQuery.data.entries ?? [] },
    });
  }, [hubWorkspaceQuery.data, selectedHubTemplateId]);

  const loadWorkspaceDirectory = useCallback(
    async (workspacePath: string) => {
      if (!selectedHubTemplateId || Object.hasOwn(workspaceListings, workspacePath)) {
        return;
      }
      setLoadingWorkspaceDirs((current) => new Set(current).add(workspacePath));
      setWorkspaceDirectoryError("");
      try {
        const listing = await queryClient.fetchQuery({
          queryKey: workspaceQueryKeys.hubWorkspace(selectedHubTemplateId, workspacePath),
          queryFn: () => fetchHubWorkspace(selectedHubTemplateId, workspacePath),
        });
        setWorkspaceListingsState((current) => {
          if (current.templateID !== selectedHubTemplateId) {
            return current;
          }
          return {
            ...current,
            listings: {
              ...current.listings,
              [workspacePath]: listing.entries ?? [],
            },
          };
        });
      } catch (error) {
        setWorkspaceDirectoryError(errorMessage(error, t("resourcesWorkspaceLoadFailed")));
      } finally {
        setLoadingWorkspaceDirs((current) => {
          const next = new Set(current);
          next.delete(workspacePath);
          return next;
        });
      }
    },
    [queryClient, selectedHubTemplateId, t, workspaceListings],
  );

  const selectWorkspaceFile = useCallback(
    (workspacePath: string) => {
      setSelectedHubWorkspacePath(workspacePath);
    },
    [setSelectedHubWorkspacePath],
  );

  const saveTemplateInstructions = useCallback(
    async (content: string) => {
      if (!selectedHubTemplateId) return false;
      try {
        const file = await updateHubWorkspaceFile(selectedHubTemplateId, "instructions/AGENTS.md", content);
        setTemplateWorkspaceFilesState((current) => ({
          files: { ...(current.templateID === selectedHubTemplateId ? current.files : {}), [file.path]: file },
          templateID: selectedHubTemplateId,
        }));
        queryClient.setQueryData(workspaceQueryKeys.hubWorkspaceFile(selectedHubTemplateId, file.path), file);
        return true;
      } catch {
        return false;
      }
    },
    [queryClient, selectedHubTemplateId],
  );
  const selectSkillFile = useCallback(
    (skillPath: string) => {
      setSelectedHubSkillPath(skillPath);
    },
    [setSelectedHubSkillPath],
  );

  const listError = manualError || (templatesQuery?.isError ? t("resourcesLoadFailed") : "");
  const detailError = hubTemplateDetailQuery.error
    ? errorMessage(hubTemplateDetailQuery.error, t("resourcesWorkspaceLoadFailed"))
    : "";
  const workspaceTreeError = hubWorkspaceQuery.error
    ? errorMessage(hubWorkspaceQuery.error, t("resourcesWorkspaceLoadFailed"))
    : workspaceDirectoryError;
  const workspaceFileError = hubWorkspaceFileQuery.error
    ? errorMessage(hubWorkspaceFileQuery.error, t("resourcesWorkspaceFileLoadFailed"))
    : "";
  const skillsError = skillsQuery.error ? errorMessage(skillsQuery.error, t("resourcesSkillsLoadFailed")) : "";
  const remoteSkillsError =
    remoteSkillsEnabled && officialSkillsQuery.error
      ? errorMessage(officialSkillsQuery.error, t("resourcesSkillRemoteSkillsLoadFailed"))
      : "";
  const skillTreeError = skillTreeQuery.error
    ? errorMessage(skillTreeQuery.error, t("resourcesSkillFilesLoadFailed"))
    : "";
  const skillFileError = skillFileQuery.error
    ? errorMessage(skillFileQuery.error, t("resourcesSkillFileLoadFailed"))
    : "";
  const {
    createMCPServer,
    deleteMCPServer,
    mcpServers,
    mcpServersFetching,
    mcpCreateDialogOpen,
    mcpMutationBusy,
    mcpMutationError,
    mcpStateError,
    openCreateMCPDialog,
    refetchMCPServers,
    selectedMCPServer,
    setMCPCreateDialogOpen,
    updateMCPServer,
  } = useWorkspaceMCPSelection({
    selectedMCPServerName,
    selectedHubResourceType,
    setSelectedMCPServerName,
    setSelectedHubResourceType,
    skillCount: skills.length,
    t,
    templateCount: resourcesTemplates.length,
  });

  const retry = useCallback(async () => {
    if (refreshTemplates) {
      await refreshTemplates();
    }
    if (selectedHubTemplateId) {
      await refetchHubTemplateDetail();
      await refetchHubWorkspace();
    }
    await refetchSkills();
    if (selectedHubSkillName) {
      await refetchSkillTree();
    }
    await refetchMCPServers();
  }, [
    refetchHubTemplateDetail,
    refetchHubWorkspace,
    refetchMCPServers,
    refetchSkillTree,
    refetchSkills,
    refreshTemplates,
    selectedHubSkillName,
    selectedHubTemplateId,
  ]);

  return {
    templates: resourcesTemplates,
    skills,
    remoteSkills,
    remoteSkillsHasMore: Boolean(officialSkillsQuery.hasNextPage),
    remoteSkillsLoading:
      remoteSkillsEnabled && officialSkillsQuery.isFetching && !officialSkillsQuery.isFetchingNextPage,
    remoteSkillsLoadingMore: officialSkillsQuery.isFetchingNextPage,
    remoteSkillsEnabled,
    remoteSkillsSearch,
    remoteSkillsError,
    loadMoreRemoteSkills,
    refetchRemoteSkills,
    setRemoteSkillsEnabled,
    setRemoteSkillsSearch,
    loaded,
    listError,
    skillsError,
    mcpServers,
    mcpStateError,
    mcpMutationBusy,
    mcpMutationError,
    mcpCreateDialogOpen,
    openCreateMCPDialog,
    setMCPCreateDialogOpen,
    error:
      listError ||
      detailError ||
      workspaceTreeError ||
      skillsError ||
      skillTreeError ||
      (selectedHubResourceType === "mcp" ? mcpStateError : ""),
    selectedHubTemplateId,
    setSelectedHubTemplateId,
    selectedHubTemplate,
    selectedHubTemplateView,
    selectedHubSkillName,
    setSelectedHubSkillName,
    selectedHubSkill,
    selectedHubSkillView: selectedHubSkill,
    selectedHubSkillPath,
    setSelectedHubSkillPath,
    selectedMCPServerName,
    setSelectedMCPServerName,
    selectedMCPServer,
    selectedHubResourceType,
    setSelectedHubResourceType,
    hubTemplateDetail: hubTemplateDetailQuery.data ?? null,
    hubTemplateDetailLoading: hubTemplateDetailQuery.isFetching,
    hubTemplateDetailError: detailError,
    refetchHubTemplateDetail,
    selectedHubWorkspacePath,
    workspaceEntries,
    workspaceTreeLoading: hubWorkspaceQuery.isFetching,
    workspaceTreeError,
    loadingWorkspaceDirs,
    loadWorkspaceDirectory,
    setSelectedHubWorkspacePath,
    selectWorkspaceFile,
    hubWorkspaceFile: hubWorkspaceFileQuery.data ?? null,
    hubWorkspaceFileLoading: hubWorkspaceFileQuery.isFetching,
    hubWorkspaceFileError: workspaceFileError,
    refetchHubWorkspaceFile: hubWorkspaceFileQuery.refetch,
    mcpServersLoading: mcpServersFetching,
    mcpServersError: mcpStateError,
    refetchMCPServers,
    skillTree: skillTreeQuery.data ?? null,
    skillTreeLoading: skillTreeQuery.isFetching,
    skillTreeError,
    refetchSkillTree,
    skillFile: skillFileQuery.data ?? null,
    skillFileLoading: skillFileQuery.isFetching,
    skillFileError,
    refetchSkillFile: skillFileQuery.refetch,
    selectSkillFile,
    retry,
    detailPaneProps: {
      templates: resourcesTemplates,
      skills,
      mcpServers,
      selectedTemplate: selectedHubTemplateView,
      selectedTemplateId: selectedHubTemplateId,
      selectedSkill: selectedHubSkill,
      selectedSkillName: selectedHubSkillName,
      selectedSkillPath: selectedHubSkillPath,
      selectedMCPServer,
      selectedMCPServerName,
      mcpMutationBusy,
      mcpMutationError,
      mcpCreateDialogOpen,
      onMCPCreateDialogOpenChange: setMCPCreateDialogOpen,
      selectedResourceType: selectedHubResourceType,
      loaded,
      mcpStateError,
      mcpStateLoading: mcpServersFetching,
      error:
        listError ||
        detailError ||
        workspaceTreeError ||
        skillsError ||
        skillTreeError ||
        (selectedHubResourceType === "mcp" ? mcpStateError : ""),
      workspaceEntries,
      workspaceTreeLoading: hubWorkspaceQuery.isFetching,
      loadingWorkspaceDirs,
      selectedWorkspacePath: selectedHubWorkspacePath,
      workspaceFile: hubWorkspaceFileQuery.data ?? null,
      workspaceFiles:
        templateWorkspaceFilesState.templateID === selectedHubTemplateId ? templateWorkspaceFilesState.files : {},
      workspaceFileLoading: hubWorkspaceFileQuery.isFetching,
      workspaceFileError,
      skillTree: skillTreeQuery.data ?? null,
      skillTreeLoading: skillTreeQuery.isFetching,
      skillTreeError,
      skillFile: skillFileQuery.data ?? null,
      skillFileLoading: skillFileQuery.isFetching,
      skillFileError,
      onRetry: retry,
      onSelectTemplate: (item: HubTemplate | null | undefined) => {
        if (item?.id) {
          setSelectedHubResourceType("template");
          setSelectedHubTemplateId(item.id);
        }
      },
      onSelectSkill: (name: string | null | undefined) => {
        const value = String(name || "").trim();
        if (value) {
          setSelectedHubResourceType("skill");
          setSelectedHubSkillName(value);
        }
      },
      onSelectMCP: (name: string | null | undefined) => {
        const value = String(name || "").trim();
        if (value) {
          setSelectedHubResourceType("mcp");
          setSelectedMCPServerName(value);
        }
      },
      onCreateMCP: createMCPServer,
      onUpdateMCP: updateMCPServer,
      onDeleteMCP: deleteMCPServer,
      onSelectWorkspaceFile: selectWorkspaceFile,
      onUpdateTemplateInstructions: saveTemplateInstructions,
      onToggleWorkspaceDir: loadWorkspaceDirectory,
      onSelectSkillFile: selectSkillFile,
    },
  };
}
