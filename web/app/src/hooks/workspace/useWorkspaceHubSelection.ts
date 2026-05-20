import { useCallback, useEffect, useMemo } from "react";
import { errorMessage } from "@/api/client";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useWorkspaceHubTemplateQuery, useWorkspaceHubWorkspaceFileQuery } from "./workspaceQueries";
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
  const hubTemplates = useMemo(() => templates ?? [], [templates]);
  const selectedHubTemplateId = useWorkspaceUiStore((state) => state.selectedHubTemplateId);
  const setSelectedHubTemplateId = useWorkspaceUiStore((state) => state.setSelectedHubTemplateId);
  const selectedHubWorkspacePath = useWorkspaceUiStore((state) => state.selectedHubWorkspacePath);
  const setSelectedHubWorkspacePath = useWorkspaceUiStore((state) => state.setSelectedHubWorkspacePath);

  useEffect(() => {
    if (!hubTemplates.length) {
      setSelectedHubTemplateId("");
      setSelectedHubWorkspacePath("");
      return;
    }
    setSelectedHubTemplateId((current) =>
      hubTemplates.some((item) => item.id === current) ? current : hubTemplates[0].id,
    );
  }, [hubTemplates, setSelectedHubTemplateId, setSelectedHubWorkspacePath]);

  useEffect(() => {
    setSelectedHubWorkspacePath("");
  }, [selectedHubTemplateId, setSelectedHubWorkspacePath]);

  const hubTemplateDetailQuery = useWorkspaceHubTemplateQuery(selectedHubTemplateId);
  const hubWorkspaceFileQuery = useWorkspaceHubWorkspaceFileQuery(selectedHubTemplateId, selectedHubWorkspacePath);
  const refetchHubTemplateDetail = hubTemplateDetailQuery.refetch;

  const selectedHubTemplate = useMemo(
    () => hubTemplates.find((item) => item.id === selectedHubTemplateId) || hubTemplates[0] || null,
    [hubTemplates, selectedHubTemplateId],
  );

  const selectedHubTemplateView =
    hubTemplateDetailQuery.data?.id === selectedHubTemplateId ? hubTemplateDetailQuery.data : selectedHubTemplate;

  const selectWorkspaceFile = useCallback(
    (workspacePath: string) => {
      setSelectedHubWorkspacePath(workspacePath);
    },
    [setSelectedHubWorkspacePath],
  );

  const listError = manualError || (templatesQuery?.isError ? t("hubLoadFailed") : "");
  const detailError = hubTemplateDetailQuery.error
    ? errorMessage(hubTemplateDetailQuery.error, t("hubWorkspaceLoadFailed"))
    : "";
  const workspaceFileError = hubWorkspaceFileQuery.error
    ? errorMessage(hubWorkspaceFileQuery.error, t("hubWorkspaceFileLoadFailed"))
    : "";

  const retry = useCallback(async () => {
    if (refreshTemplates) {
      await refreshTemplates();
    }
    if (selectedHubTemplateId) {
      await refetchHubTemplateDetail();
    }
  }, [refetchHubTemplateDetail, refreshTemplates, selectedHubTemplateId]);

  return {
    templates: hubTemplates,
    loaded,
    listError,
    error: listError || detailError,
    selectedHubTemplateId,
    setSelectedHubTemplateId,
    selectedHubTemplate,
    selectedHubTemplateView,
    hubTemplateDetail: hubTemplateDetailQuery.data ?? null,
    hubTemplateDetailLoading: hubTemplateDetailQuery.isFetching,
    hubTemplateDetailError: detailError,
    refetchHubTemplateDetail,
    selectedHubWorkspacePath,
    setSelectedHubWorkspacePath,
    selectWorkspaceFile,
    hubWorkspaceFile: hubWorkspaceFileQuery.data ?? null,
    hubWorkspaceFileLoading: hubWorkspaceFileQuery.isFetching,
    hubWorkspaceFileError: workspaceFileError,
    refetchHubWorkspaceFile: hubWorkspaceFileQuery.refetch,
    retry,
    detailPaneProps: {
      templates: hubTemplates,
      selectedTemplate: selectedHubTemplateView,
      selectedTemplateId: selectedHubTemplateId,
      loaded,
      error: listError || detailError,
      detailLoading: hubTemplateDetailQuery.isFetching,
      selectedWorkspacePath: selectedHubWorkspacePath,
      workspaceFile: hubWorkspaceFileQuery.data ?? null,
      workspaceFileLoading: hubWorkspaceFileQuery.isFetching,
      workspaceFileError,
      onRetry: retry,
      onSelectTemplate: (item: HubTemplate | null | undefined) => {
        if (item?.id) {
          setSelectedHubTemplateId(item.id);
        }
      },
      onSelectWorkspaceFile: selectWorkspaceFile,
    },
  };
}
