import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { fetchHubWorkspace } from "@/api/hub";
import { hasSkillName, isOfficialSkill, isPersonalSkill } from "@/models/skillhub";
import { flattenWorkspaceDirectoryListings } from "@/models/workspace";
import type { WorkspaceDirectoryListings } from "@/models/workspace";
import { useWorkspaceUiStore } from "./workspaceUiStore";
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

  useEffect(() => {
    if (selectedHubResourceType === "skill" && !skills.length && resourcesTemplates.length) {
      setSelectedHubResourceType("template");
      return;
    }
    if (selectedHubResourceType === "template" && !resourcesTemplates.length && skills.length) {
      setSelectedHubResourceType("skill");
    }
  }, [resourcesTemplates.length, selectedHubResourceType, setSelectedHubResourceType, skills.length]);

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
  }, [
    refetchHubTemplateDetail,
    refetchHubWorkspace,
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
    error: listError || detailError || workspaceTreeError || skillsError || skillTreeError,
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
      selectedTemplate: selectedHubTemplateView,
      selectedTemplateId: selectedHubTemplateId,
      selectedSkill: selectedHubSkill,
      selectedSkillName: selectedHubSkillName,
      selectedSkillPath: selectedHubSkillPath,
      selectedResourceType: selectedHubResourceType,
      loaded,
      error: listError || detailError || workspaceTreeError || skillsError || skillTreeError,
      workspaceEntries,
      workspaceTreeLoading: hubWorkspaceQuery.isFetching,
      loadingWorkspaceDirs,
      selectedWorkspacePath: selectedHubWorkspacePath,
      workspaceFile: hubWorkspaceFileQuery.data ?? null,
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
      onSelectWorkspaceFile: selectWorkspaceFile,
      onToggleWorkspaceDir: loadWorkspaceDirectory,
      onSelectSkillFile: selectSkillFile,
    },
  };
}
