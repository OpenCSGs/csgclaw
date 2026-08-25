import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage as apiErrorMessage, type ApiError } from "@/api/client";
import { deleteHubTemplateRequest, publishHubTemplateToCommunityRequest } from "@/api/hub";
import { deleteSkillRequest, installRemoteSkillRequest, uploadSkillArchive } from "@/api/skills";
import {
  HubTemplateErrorCodes,
  hubTemplateErrorCode,
  isDeletableHubTemplate,
  isVisibleInHubTemplateList,
  upsertHubTemplateReviewState,
} from "@/models/hubWorkspace";
import type { HubTemplate } from "@/models/hubWorkspace";
import { isReadonlySkill } from "@/models/skillhub";
import type { SkillSummary } from "@/models/skillhub";
import { localizeAPIError } from "@/shared/i18n";
import { workspaceQueryKeys } from "./workspaceQueries";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";
import type { UseWorkspaceHubControllerArgs } from "./types";

type WorkspaceHubSelection = ReturnType<typeof useWorkspaceHubSelection>;
type DeleteHubTemplate = (template: HubTemplate | null | undefined) => Promise<boolean>;
type DeleteSkill = (skill: SkillSummary | null | undefined) => Promise<boolean>;
export type PublishHubTemplateResult = { status: "success" } | { status: "partial"; message: string } | null;
export type InstallRemoteSkillOptions = {
  replace?: boolean;
};

export function workspaceHubMutationError(
  resourceType: "mcp" | "skill" | "template",
  deleteError: string,
  publishError: string,
  skillDeleteError: string,
): string {
  if (resourceType === "template") {
    return deleteError || publishError;
  }
  if (resourceType === "skill") {
    return skillDeleteError;
  }
  return "";
}

export function visibleWorkspaceHubTemplates(
  templates: readonly HubTemplate[],
  deletingTemplateID: string,
): HubTemplate[] {
  return templates.filter((item) => item.id !== deletingTemplateID && isVisibleInHubTemplateList(item));
}

export type WorkspaceHubController = {
  hub: Omit<WorkspaceHubSelection, "detailPaneProps"> & {
    deleteBusy: boolean;
    deleteHubTemplate: DeleteHubTemplate;
    publishBusy: boolean;
    publishHubTemplate: (
      template: HubTemplate | null | undefined,
      deploy?: boolean,
    ) => Promise<PublishHubTemplateResult>;
    deleteSkill: DeleteSkill;
    skillDeleteBusy: boolean;
    remoteInstallBusy: string;
    remoteInstallError: string;
    installRemoteSkill: (
      skill: SkillSummary | null | undefined,
      options?: InstallRemoteSkillOptions,
    ) => Promise<SkillSummary | null>;
    uploadBusy: boolean;
    uploadError: string;
    uploadSkill: (file: File) => Promise<SkillSummary | null>;
    detailPaneProps: WorkspaceHubSelection["detailPaneProps"] & {
      deleteBusy: boolean;
      onDeleteTemplate: DeleteHubTemplate;
      onPublishTemplate: (
        template: HubTemplate | null | undefined,
        deploy?: boolean,
      ) => Promise<PublishHubTemplateResult>;
      publishBusy: boolean;
      publishDisabled: boolean;
      publishError: string;
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
  openCSGAuthenticated = false,
  refreshWorkspaceHubTemplates,
  t,
}: UseWorkspaceHubControllerArgs): WorkspaceHubController {
  const errorMessage = useCallback(
    (error: unknown, fallback = "") => localizeAPIError(error, t, apiErrorMessage(error, fallback) || fallback),
    [t],
  );
  const queryClient = useQueryClient();
  const [resourcesManualError, setResourcesManualError] = useState("");
  const [resourcesDeleteBusy, setResourcesDeleteBusy] = useState(false);
  const [resourcesDeleteError, setResourcesDeleteError] = useState("");
  const [deletingTemplateID, setDeletingTemplateID] = useState("");
  const [resourcesPublishBusy, setResourcesPublishBusy] = useState(false);
  const [resourcesPublishError, setResourcesPublishError] = useState("");
  const [skillDeleteBusy, setSkillDeleteBusy] = useState(false);
  const [skillDeleteError, setSkillDeleteError] = useState("");
  const [resourcesUploadBusy, setResourcesUploadBusy] = useState(false);
  const [resourcesUploadError, setResourcesUploadError] = useState("");
  const [resourcesRemoteInstallBusy, setResourcesRemoteInstallBusy] = useState("");
  const [resourcesRemoteInstallError, setResourcesRemoteInstallError] = useState("");

  const refreshHubTemplates = useCallback(async (): Promise<void> => {
    try {
      await refreshWorkspaceHubTemplates();
      setResourcesManualError("");
    } catch (_) {
      setResourcesManualError(t("resourcesLoadFailed"));
    }
  }, [refreshWorkspaceHubTemplates, t]);

  useEffect(() => {
    if (hubTemplatesQuery.isSuccess) {
      setResourcesManualError("");
    }
  }, [hubTemplatesQuery.isSuccess, hubTemplatesQuery.dataUpdatedAt]);

  const visibleHubTemplates = useMemo(
    () => visibleWorkspaceHubTemplates(hubTemplates, deletingTemplateID),
    [deletingTemplateID, hubTemplates],
  );

  const hub = useWorkspaceHubSelection({
    templates: visibleHubTemplates,
    templatesQuery: hubTemplatesQuery,
    loaded: hubLoaded,
    manualError: resourcesManualError,
    refreshTemplates: refreshHubTemplates,
    t,
  });
  const { setSelectedHubResourceType, setSelectedHubSkillName, setSelectedHubSkillPath, setSelectedHubTemplateId } =
    hub;
  const mutationError = workspaceHubMutationError(
    hub.selectedHubResourceType,
    resourcesDeleteError,
    resourcesPublishError,
    skillDeleteError,
  );

  useEffect(() => {
    if (hub.selectedHubResourceType !== "template") {
      setResourcesDeleteError("");
      setResourcesPublishError("");
    }
    if (hub.selectedHubResourceType !== "skill") {
      setSkillDeleteError("");
    }
  }, [hub.selectedHubResourceType]);

  const deleteHubTemplate = useCallback(
    async (template: HubTemplate | null | undefined): Promise<boolean> => {
      if (!template?.id || !isDeletableHubTemplate(template)) {
        return false;
      }
      const label = template.name || template.id;
      if (!window.confirm(`${t("resourcesDeleteConfirm")} ${label}?`)) {
        return false;
      }
      setResourcesDeleteBusy(true);
      setResourcesDeleteError("");
      setDeletingTemplateID(template.id);
      setSelectedHubTemplateId("");
      try {
        await Promise.all([
          queryClient.cancelQueries({ queryKey: workspaceQueryKeys.hubTemplate(template.id) }),
          queryClient.cancelQueries({ queryKey: workspaceQueryKeys.hubWorkspaceScope(template.id) }),
          queryClient.cancelQueries({ queryKey: workspaceQueryKeys.hubWorkspaceFileScope(template.id) }),
        ]);
        await deleteHubTemplateRequest(template.id);
        queryClient.removeQueries({ queryKey: workspaceQueryKeys.hubTemplate(template.id) });
        queryClient.removeQueries({ queryKey: workspaceQueryKeys.hubWorkspaceScope(template.id) });
        queryClient.removeQueries({ queryKey: workspaceQueryKeys.hubWorkspaceFileScope(template.id) });
        await refreshHubTemplates();
        return true;
      } catch (err) {
        setResourcesDeleteError(errorMessage(err, t("resourcesDeleteFailed")));
        return false;
      } finally {
        setDeletingTemplateID("");
        setResourcesDeleteBusy(false);
      }
    },
    [errorMessage, queryClient, refreshHubTemplates, setSelectedHubTemplateId, t],
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
        setSkillDeleteError(errorMessage(err, t("resourcesSkillDeleteFailed")));
        return false;
      } finally {
        setSkillDeleteBusy(false);
      }
    },
    [errorMessage, queryClient, setSelectedHubSkillName, setSelectedHubSkillPath, t],
  );

  const publishHubTemplate = useCallback(
    async (template: HubTemplate | null | undefined, deploy = false): Promise<PublishHubTemplateResult> => {
      if (!template?.id || !isDeletableHubTemplate(template) || !openCSGAuthenticated) {
        return null;
      }
      setResourcesPublishBusy(true);
      setResourcesPublishError("");
      try {
        const published = await publishHubTemplateToCommunityRequest(template.id, deploy);
        await refreshHubTemplates();
        if (published.id) {
          setSelectedHubTemplateId(published.id);
        }
        return { status: "success" };
      } catch (err) {
        const errorCode = hubTemplateErrorCode(err);
        const deploySensitiveCheckFailed = errorCode === HubTemplateErrorCodes.reviewFailed;
        const deployReviewPending = errorCode === HubTemplateErrorCodes.reviewPending;
        const message = errorMessage(err, t("resourcesPublishCommunityFailed"));
        if (deploy) {
          await refreshHubTemplates();
          const apiError = err as ApiError;
          const publishedTemplateID = String(apiError.publishedTemplateId ?? "").trim();
          if (publishedTemplateID) {
            setSelectedHubResourceType("template");
            if (deploySensitiveCheckFailed || deployReviewPending) {
              queryClient.setQueryData<HubTemplate[]>(workspaceQueryKeys.hubTemplates(), (templates) =>
                upsertHubTemplateReviewState(
                  templates,
                  publishedTemplateID,
                  deployReviewPending ? "Pending" : "Fail",
                  deployReviewPending ? "" : message,
                ),
              );
            }
            setSelectedHubTemplateId(publishedTemplateID);
            setResourcesPublishError("");
            return { status: "partial", message };
          }
        }
        setResourcesPublishError(message);
        return null;
      } finally {
        setResourcesPublishBusy(false);
      }
    },
    [
      errorMessage,
      openCSGAuthenticated,
      queryClient,
      refreshHubTemplates,
      setSelectedHubResourceType,
      setSelectedHubTemplateId,
      t,
    ],
  );

  const uploadSkill = useCallback(
    async (file: File): Promise<SkillSummary | null> => {
      setResourcesUploadBusy(true);
      setResourcesUploadError("");
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
        setResourcesUploadError(errorMessage(err, t("resourcesSkillUploadFailed")));
        return null;
      } finally {
        setResourcesUploadBusy(false);
      }
    },
    [errorMessage, queryClient, setSelectedHubResourceType, setSelectedHubSkillName, setSelectedHubSkillPath, t],
  );

  const installRemoteSkill = useCallback(
    async (
      skill: SkillSummary | null | undefined,
      options: InstallRemoteSkillOptions = {},
    ): Promise<SkillSummary | null> => {
      const remotePath = String(skill?.remotePath || "").trim();
      if (!remotePath) {
        setResourcesRemoteInstallError(t("resourcesSkillRemoteInstallFailed"));
        return null;
      }
      setResourcesRemoteInstallBusy(remotePath);
      setResourcesRemoteInstallError("");
      try {
        const installed = await installRemoteSkillRequest(remotePath, skill?.remoteRef, Boolean(options.replace));
        queryClient.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) => {
          return upsertSkillSummary(current, installed);
        });
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.skills() });
        setSelectedHubResourceType("skill");
        setSelectedHubSkillName(installed.name);
        setSelectedHubSkillPath("");
        return installed;
      } catch (err) {
        setResourcesRemoteInstallError(errorMessage(err, t("resourcesSkillRemoteInstallFailed")));
        return null;
      } finally {
        setResourcesRemoteInstallBusy("");
      }
    },
    [errorMessage, queryClient, setSelectedHubResourceType, setSelectedHubSkillName, setSelectedHubSkillPath, t],
  );

  return {
    hub: {
      ...hub,
      error: hub.error || mutationError,
      deleteBusy: resourcesDeleteBusy,
      deleteHubTemplate,
      publishBusy: resourcesPublishBusy,
      publishHubTemplate,
      deleteSkill,
      installRemoteSkill,
      remoteInstallBusy: resourcesRemoteInstallBusy,
      remoteInstallError: resourcesRemoteInstallError,
      skillDeleteBusy,
      uploadBusy: resourcesUploadBusy,
      uploadError: resourcesUploadError,
      uploadSkill,
      detailPaneProps: {
        ...hub.detailPaneProps,
        error: hub.detailPaneProps.error || mutationError,
        deleteBusy: resourcesDeleteBusy,
        onDeleteTemplate: deleteHubTemplate,
        onPublishTemplate: publishHubTemplate,
        publishBusy: resourcesPublishBusy,
        publishDisabled: !openCSGAuthenticated,
        publishError: resourcesPublishError,
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
