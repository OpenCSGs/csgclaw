import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { deleteHubTemplateRequest } from "@/api/hub";
import { deleteSkillRequest, uploadSkillArchive } from "@/api/skills";
import { isDeletableHubTemplate, isVisibleInHubTemplateList } from "@/models/hubWorkspace";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { SkillSummary } from "@/models/skillhub";
import { workspaceQueryKeys } from "./workspaceQueries";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";
import type { UseWorkspaceHubControllerArgs } from "./types";

type WorkspaceHubSelection = ReturnType<typeof useWorkspaceHubSelection>;
type DeleteHubTemplate = (template: HubTemplate | null | undefined) => Promise<boolean>;
type DeleteSkill = (skill: SkillSummary | null | undefined) => Promise<boolean>;

export type WorkspaceHubController = {
  hub: Omit<WorkspaceHubSelection, "detailPaneProps"> & {
    deleteBusy: boolean;
    deleteHubTemplate: DeleteHubTemplate;
    deleteSkill: DeleteSkill;
    skillDeleteBusy: boolean;
    uploadBusy: boolean;
    uploadError: string;
    uploadSkill: (file: File) => Promise<SkillSummary | null>;
    detailPaneProps: WorkspaceHubSelection["detailPaneProps"] & {
      deleteBusy: boolean;
      onDeleteTemplate: DeleteHubTemplate;
      onDeleteSkill: DeleteSkill;
      skillDeleteBusy: boolean;
    };
  };
  refreshHubTemplates: () => Promise<void>;
};

export function useWorkspaceHubController({
  hubLoaded,
  hubTemplates,
  hubTemplatesQuery,
  refreshWorkspaceHubTemplates,
  t,
}: UseWorkspaceHubControllerArgs): WorkspaceHubController {
  const queryClient = useQueryClient();
  const [hubManualError, setHubManualError] = useState("");
  const [hubDeleteBusy, setHubDeleteBusy] = useState(false);
  const [hubDeleteError, setHubDeleteError] = useState("");
  const [skillDeleteBusy, setSkillDeleteBusy] = useState(false);
  const [skillDeleteError, setSkillDeleteError] = useState("");
  const [hubUploadBusy, setHubUploadBusy] = useState(false);
  const [hubUploadError, setHubUploadError] = useState("");

  const refreshHubTemplates = useCallback(async (): Promise<void> => {
    try {
      await refreshWorkspaceHubTemplates();
      setHubManualError("");
    } catch (_) {
      setHubManualError(t("hubLoadFailed"));
    }
  }, [refreshWorkspaceHubTemplates, t]);

  useEffect(() => {
    if (hubTemplatesQuery.isSuccess) {
      setHubManualError("");
    }
  }, [hubTemplatesQuery.isSuccess, hubTemplatesQuery.dataUpdatedAt]);

  const visibleHubTemplates = useMemo(
    () => hubTemplates.filter((item) => isVisibleInHubTemplateList(item)),
    [hubTemplates],
  );

  const hub = useWorkspaceHubSelection({
    templates: visibleHubTemplates,
    templatesQuery: hubTemplatesQuery,
    loaded: hubLoaded,
    manualError: hubManualError || hubDeleteError || skillDeleteError,
    refreshTemplates: refreshHubTemplates,
    t,
  });
  const { setSelectedHubResourceType, setSelectedHubSkillName, setSelectedHubSkillPath, setSelectedHubTemplateId } =
    hub;

  const deleteHubTemplate = useCallback(
    async (template: HubTemplate | null | undefined): Promise<boolean> => {
      if (!template?.id || !isDeletableHubTemplate(template)) {
        return false;
      }
      const label = template.name || template.id;
      if (!window.confirm(`${t("hubDeleteConfirm")} ${label}?`)) {
        return false;
      }
      setHubDeleteBusy(true);
      setHubDeleteError("");
      try {
        await deleteHubTemplateRequest(template.id);
        setSelectedHubTemplateId("");
        await refreshHubTemplates();
        return true;
      } catch (err) {
        setHubDeleteError(errorMessage(err, t("hubDeleteFailed")));
        return false;
      } finally {
        setHubDeleteBusy(false);
      }
    },
    [refreshHubTemplates, setSelectedHubTemplateId, t],
  );

  const deleteSkill = useCallback(
    async (skill: SkillSummary | null | undefined): Promise<boolean> => {
      const name = String(skill?.name || "").trim();
      if (!name || skill?.readonly || skill?.source === "system") {
        return false;
      }
      setSkillDeleteBusy(true);
      setSkillDeleteError("");
      try {
        await deleteSkillRequest(name);
        queryClient.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) =>
          (Array.isArray(current) ? current : []).filter((item) => item.name !== name),
        );
        queryClient.removeQueries({ queryKey: workspaceQueryKeys.skillTree(name) });
        setSelectedHubSkillName("");
        setSelectedHubSkillPath("");
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.skills() });
        return true;
      } catch (err) {
        setSkillDeleteError(errorMessage(err, t("hubSkillDeleteFailed")));
        return false;
      } finally {
        setSkillDeleteBusy(false);
      }
    },
    [queryClient, setSelectedHubSkillName, setSelectedHubSkillPath, t],
  );

  const uploadSkill = useCallback(
    async (file: File): Promise<SkillSummary | null> => {
      setHubUploadBusy(true);
      setHubUploadError("");
      try {
        const uploaded = await uploadSkillArchive(file);
        queryClient.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) => {
          const items = Array.isArray(current) ? current : [];
          if (items.some((item) => item.name === uploaded.name)) {
            return items;
          }
          return [...items, uploaded].sort((left, right) => left.name.localeCompare(right.name));
        });
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.skills() });
        setSelectedHubResourceType("skill");
        setSelectedHubSkillName(uploaded.name);
        setSelectedHubSkillPath("");
        return uploaded;
      } catch (err) {
        setHubUploadError(errorMessage(err, t("hubSkillUploadFailed")));
        return null;
      } finally {
        setHubUploadBusy(false);
      }
    },
    [queryClient, setSelectedHubResourceType, setSelectedHubSkillName, setSelectedHubSkillPath, t],
  );

  return {
    hub: {
      ...hub,
      deleteBusy: hubDeleteBusy,
      deleteHubTemplate,
      deleteSkill,
      skillDeleteBusy,
      uploadBusy: hubUploadBusy,
      uploadError: hubUploadError,
      uploadSkill,
      detailPaneProps: {
        ...hub.detailPaneProps,
        deleteBusy: hubDeleteBusy,
        onDeleteTemplate: deleteHubTemplate,
        onDeleteSkill: deleteSkill,
        skillDeleteBusy,
      },
    },
    refreshHubTemplates,
  };
}
