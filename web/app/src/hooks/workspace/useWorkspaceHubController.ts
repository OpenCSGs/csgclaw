import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { deleteHubTemplateRequest } from "@/api/hub";
import { deleteSkillRequest, installRemoteSkillRequest, uploadSkillArchive } from "@/api/skills";
import { isDeletableHubTemplate, isVisibleInHubTemplateList } from "@/models/hubWorkspace";
import type { HubTemplate } from "@/models/hubWorkspace";
import { isReadonlySkill } from "@/models/skillhub";
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
    remoteInstallBusy: string;
    remoteInstallError: string;
    installRemoteSkill: (skill: SkillSummary | null | undefined) => Promise<SkillSummary | null>;
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
  const [hubRemoteInstallBusy, setHubRemoteInstallBusy] = useState("");
  const [hubRemoteInstallError, setHubRemoteInstallError] = useState("");

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
      if (!name || isReadonlySkill(skill)) {
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
          return upsertSkillSummary(current, uploaded);
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

  const installRemoteSkill = useCallback(
    async (skill: SkillSummary | null | undefined): Promise<SkillSummary | null> => {
      const remotePath = String(skill?.remotePath || "").trim();
      if (!remotePath) {
        setHubRemoteInstallError(t("hubSkillRemoteInstallFailed"));
        return null;
      }
      setHubRemoteInstallBusy(remotePath);
      setHubRemoteInstallError("");
      try {
        const installed = await installRemoteSkillRequest(remotePath, skill?.remoteRef);
        queryClient.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) => {
          return upsertSkillSummary(current, installed);
        });
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.skills() });
        setSelectedHubResourceType("skill");
        setSelectedHubSkillName(installed.name);
        setSelectedHubSkillPath("");
        return installed;
      } catch (err) {
        setHubRemoteInstallError(errorMessage(err, t("hubSkillRemoteInstallFailed")));
        return null;
      } finally {
        setHubRemoteInstallBusy("");
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
      installRemoteSkill,
      remoteInstallBusy: hubRemoteInstallBusy,
      remoteInstallError: hubRemoteInstallError,
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

function upsertSkillSummary(current: readonly SkillSummary[] | null | undefined, skill: SkillSummary): SkillSummary[] {
  const items = Array.isArray(current) ? current : [];
  return [...items.filter((item) => item.name !== skill.name), skill].sort((left, right) =>
    left.name.localeCompare(right.name),
  );
}
