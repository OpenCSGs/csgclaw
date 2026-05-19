import { useCallback, useEffect, useMemo } from "react";
import { errorMessage } from "@/api/client";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import { useWorkspaceHubTemplateQuery, useWorkspaceHubWorkspaceFileQuery } from "./workspaceQueries";

export function useWorkspaceHubSelection({ templates, templatesQuery, loaded, manualError, refreshTemplates, t }) {
  const hubTemplates = templates ?? [];
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

  const selectedHubTemplate = useMemo(
    () => hubTemplates.find((item) => item.id === selectedHubTemplateId) || hubTemplates[0] || null,
    [hubTemplates, selectedHubTemplateId],
  );

  const selectedHubTemplateView =
    hubTemplateDetailQuery.data?.id === selectedHubTemplateId ? hubTemplateDetailQuery.data : selectedHubTemplate;

  const selectWorkspaceFile = useCallback(
    (workspacePath) => {
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
      await hubTemplateDetailQuery.refetch();
    }
  }, [hubTemplateDetailQuery.refetch, refreshTemplates, selectedHubTemplateId]);

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
    refetchHubTemplateDetail: hubTemplateDetailQuery.refetch,
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
      onSelectTemplate: (item) => {
        if (item?.id) {
          setSelectedHubTemplateId(item.id);
        }
      },
      onSelectWorkspaceFile: selectWorkspaceFile,
    },
  };
}
