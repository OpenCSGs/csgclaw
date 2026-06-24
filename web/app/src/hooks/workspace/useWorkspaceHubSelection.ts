import { useCallback, useEffect, useMemo } from "react";
import { errorMessage } from "@/api/client";
import { hasSkillName } from "@/models/skillhub";
import { useWorkspaceUiStore } from "./workspaceUiStore";
import {
  useWorkspaceHubTemplateQuery,
  useWorkspaceHubWorkspaceFileQuery,
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
  const hubTemplates = useMemo(() => templates ?? [], [templates]);
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
  const skillsQuery = useWorkspaceSkillsQuery();
  const skills = useMemo(() => skillsQuery.data ?? [], [skillsQuery.data]);

  useEffect(() => {
    if (!hubTemplates.length) {
      setSelectedHubTemplateId("");
      setSelectedHubWorkspacePath("");
      return;
    }
    setSelectedHubTemplateId((current) =>
      hubTemplates.some((item) => item.id === current) ? current : (hubTemplates[0]?.id ?? ""),
    );
  }, [hubTemplates, setSelectedHubTemplateId, setSelectedHubWorkspacePath]);

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
    if (selectedHubResourceType === "skill" && !skills.length && hubTemplates.length) {
      setSelectedHubResourceType("template");
      return;
    }
    if (selectedHubResourceType === "template" && !hubTemplates.length && skills.length) {
      setSelectedHubResourceType("skill");
    }
  }, [hubTemplates.length, selectedHubResourceType, setSelectedHubResourceType, skills.length]);

  const hubTemplateDetailQuery = useWorkspaceHubTemplateQuery(selectedHubTemplateId);
  const hubWorkspaceFileQuery = useWorkspaceHubWorkspaceFileQuery(selectedHubTemplateId, selectedHubWorkspacePath);
  const skillTreeQuery = useWorkspaceSkillTreeQuery(selectedHubSkillName);
  const skillFileQuery = useWorkspaceSkillFileQuery(selectedHubSkillPath);
  const refetchHubTemplateDetail = hubTemplateDetailQuery.refetch;
  const refetchSkills = skillsQuery.refetch;
  const refetchSkillTree = skillTreeQuery.refetch;

  const selectedHubTemplate = useMemo(
    () => hubTemplates.find((item) => item.id === selectedHubTemplateId) || hubTemplates[0] || null,
    [hubTemplates, selectedHubTemplateId],
  );
  const selectedHubSkill = useMemo(
    () => skills.find((item) => item.name === selectedHubSkillName) || skills[0] || null,
    [selectedHubSkillName, skills],
  );

  const selectedHubTemplateView =
    hubTemplateDetailQuery.data?.id === selectedHubTemplateId ? hubTemplateDetailQuery.data : selectedHubTemplate;

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

  const listError = manualError || (templatesQuery?.isError ? t("hubLoadFailed") : "");
  const detailError = hubTemplateDetailQuery.error
    ? errorMessage(hubTemplateDetailQuery.error, t("hubWorkspaceLoadFailed"))
    : "";
  const workspaceFileError = hubWorkspaceFileQuery.error
    ? errorMessage(hubWorkspaceFileQuery.error, t("hubWorkspaceFileLoadFailed"))
    : "";
  const skillsError = skillsQuery.error ? errorMessage(skillsQuery.error, t("hubSkillsLoadFailed")) : "";
  const skillTreeError = skillTreeQuery.error ? errorMessage(skillTreeQuery.error, t("hubSkillFilesLoadFailed")) : "";
  const skillFileError = skillFileQuery.error ? errorMessage(skillFileQuery.error, t("hubSkillFileLoadFailed")) : "";

  const retry = useCallback(async () => {
    if (refreshTemplates) {
      await refreshTemplates();
    }
    if (selectedHubTemplateId) {
      await refetchHubTemplateDetail();
    }
    await refetchSkills();
    if (selectedHubSkillName) {
      await refetchSkillTree();
    }
  }, [
    refetchHubTemplateDetail,
    refetchSkillTree,
    refetchSkills,
    refreshTemplates,
    selectedHubSkillName,
    selectedHubTemplateId,
  ]);

  return {
    templates: hubTemplates,
    skills,
    loaded,
    listError,
    skillsError,
    error: listError || detailError || skillsError || skillTreeError,
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
      templates: hubTemplates,
      skills,
      selectedTemplate: selectedHubTemplateView,
      selectedTemplateId: selectedHubTemplateId,
      selectedSkill: selectedHubSkill,
      selectedSkillName: selectedHubSkillName,
      selectedSkillPath: selectedHubSkillPath,
      selectedResourceType: selectedHubResourceType,
      loaded,
      error: listError || detailError || skillsError || skillTreeError,
      detailLoading: hubTemplateDetailQuery.isFetching,
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
      onSelectSkillFile: selectSkillFile,
    },
  };
}
