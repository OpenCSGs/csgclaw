import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage as apiErrorMessage } from "@/api/client";
import { deleteHubTemplateRequest, publishHubTemplateToCommunityRequest } from "@/api/hub";
import { deleteSkillRequest, installRemoteSkillRequest, uploadSkillArchive } from "@/api/skills";
import { isDeletableHubTemplate, isVisibleInHubTemplateList } from "@/models/hubWorkspace";
import type { HubTemplate } from "@/models/hubWorkspace";
import { isReadonlySkill } from "@/models/skillhub";
import type { SkillSummary } from "@/models/skillhub";
import { localizeAPIError, localizeTemplatePublishError } from "@/shared/i18n";
import { workspaceQueryKeys } from "./workspaceQueries";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";
import type { UseWorkspaceHubControllerArgs } from "./types";

type WorkspaceHubSelection = ReturnType<typeof useWorkspaceHubSelection>;
type DeleteHubTemplate = (template: HubTemplate | null | undefined) => Promise<boolean>;
type DeleteSkill = (skill: SkillSummary | null | undefined, options?: { navigate?: boolean }) => Promise<boolean>;
export type PublishHubTemplateResult = { status: "success"; template?: HubTemplate } | null;
export type InstallRemoteSkillOptions = {
  replace?: boolean;
};

export function workspaceHubMutationError(
  resourceType: "knowledge" | "mcp" | "skill" | "template",
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

export function nextWorkspaceSkillAfterDelete(
  skills: readonly SkillSummary[],
  deletedName: string,
): SkillSummary | null {
  const deletedIndex = skills.findIndex((item) => item.name === deletedName);
  if (deletedIndex < 0) {
    return skills[0] ?? null;
  }
  return skills[deletedIndex + 1] ?? skills[deletedIndex - 1] ?? null;
}

export type WorkspaceHubController = {
  hub: Omit<WorkspaceHubSelection, "detailPaneProps"> & {
    deleteBusy: boolean;
    deleteHubTemplate: DeleteHubTemplate;
    publishBusy: boolean;
    publishHubTemplate: (
      template: HubTemplate | null | undefined,
      includeMemory?: boolean,
    ) => Promise<PublishHubTemplateResult>;
    showPublishedTemplate: (template: HubTemplate) => void;
    setPublishError: (message: string) => void;
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
        includeMemory?: boolean,
      ) => Promise<PublishHubTemplateResult>;
      publishBusy: boolean;
      publishDisabled: boolean;
      publishedTemplate: HubTemplate | null;
      onDismissPublishedTemplate: () => void;
      publishError: string;
      onResetPublishError: () => void;
      onDeleteSkill: DeleteSkill;
      skillDeleteBusy: boolean;
    };
  };
  refreshHubTemplates: (published?: HubTemplate) => Promise<void>;
};

export function useWorkspaceHubController({
  activePane,
  hubLoaded,
  hubTemplates,
  hubTemplatesQuery,
  onSkillDeleted,
  onTemplateDeleteStarted,
  openCSGAuthGuard,
  refreshWorkspaceHubTemplates,
  t,
}: UseWorkspaceHubControllerArgs): WorkspaceHubController {
  const {
    authenticated: openCSGAuthenticated,
    handleAuthenticationError: handleOpenCSGAuthenticationError,
    requireAuthentication: requireOpenCSGAuthentication,
  } = openCSGAuthGuard;
  const errorMessage = useCallback(
    (error: unknown, fallback = "") => localizeAPIError(error, t, apiErrorMessage(error, fallback) || fallback),
    [t],
  );
  const queryClient = useQueryClient();
  const [resourcesManualError, setResourcesManualError] = useState("");
  const [resourcesDeleteBusy, setResourcesDeleteBusy] = useState(false);
  const [resourcesDeleteError, setResourcesDeleteError] = useState("");
  const [deletingTemplateID, setDeletingTemplateID] = useState("");
  const [publishedTemplate, setPublishedTemplate] = useState<HubTemplate | null>(null);
  const [resourcesPublishBusy, setResourcesPublishBusy] = useState(false);
  const [publishFailure, setPublishFailure] = useState<{ templateID: string; message: string } | null>(null);
  const publishRequestVersion = useRef(0);
  const resetPublishError = useCallback(() => {
    publishRequestVersion.current += 1;
    setPublishFailure(null);
  }, []);
  const [skillDeleteBusy, setSkillDeleteBusy] = useState(false);
  const [skillDeleteError, setSkillDeleteError] = useState("");
  const [resourcesUploadBusy, setResourcesUploadBusy] = useState(false);
  const [resourcesUploadError, setResourcesUploadError] = useState("");
  const [resourcesRemoteInstallBusy, setResourcesRemoteInstallBusy] = useState("");
  const [resourcesRemoteInstallError, setResourcesRemoteInstallError] = useState("");

  const refreshHubTemplates = useCallback(
    async (published?: HubTemplate): Promise<void> => {
      try {
        await refreshWorkspaceHubTemplates();
        setResourcesManualError("");
      } catch (_) {
        setResourcesManualError(t("resourcesLoadFailed"));
      }
      // Community listings may lag behind a successful publication.
      // Keep refreshed metadata when present, otherwise include the returned template.
      if (published?.id) {
        queryClient.setQueryData<HubTemplate[]>(workspaceQueryKeys.hubTemplates(), (current) =>
          current?.some((item) => item.id === published.id) ? current : [...(current ?? []), published],
        );
      }
    },
    [refreshWorkspaceHubTemplates, queryClient, t],
  );

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
    activePane,
    templates: visibleHubTemplates,
    templatesQuery: hubTemplatesQuery,
    loaded: hubLoaded,
    manualError: resourcesManualError,
    openCSGAuthGuard,
    refreshTemplates: refreshHubTemplates,
    t,
  });
  const {
    skills: workspaceSkills,
    setSelectedHubResourceType,
    setSelectedHubSkillName,
    setSelectedHubSkillPath,
    setSelectedHubTemplateId,
  } = hub;
  useEffect(() => {
    resetPublishError();
  }, [hub.selectedHubTemplateId, resetPublishError]);
  const resourcesPublishError =
    publishFailure && publishFailure.templateID === hub.selectedHubTemplateId ? publishFailure.message : "";

  const mutationError = workspaceHubMutationError(
    hub.selectedHubResourceType,
    resourcesDeleteError,
    resourcesPublishError,
    skillDeleteError,
  );

  useEffect(() => {
    if (hub.selectedHubResourceType !== "template") {
      setResourcesDeleteError("");
      resetPublishError();
    }
    if (hub.selectedHubResourceType !== "skill") {
      setSkillDeleteError("");
    }
  }, [hub.selectedHubResourceType, resetPublishError]);

  const deleteHubTemplate = useCallback(
    async (template: HubTemplate | null | undefined): Promise<boolean> => {
      if (!template?.id || !isDeletableHubTemplate(template)) {
        return false;
      }
      const label = template.name || template.id;
      if (!window.confirm(`${t("resourcesDeleteConfirm")} ${label}?\n\n${t("resourcesDeleteLocalOnlyNotice")}`)) {
        return false;
      }
      setResourcesDeleteBusy(true);
      setResourcesDeleteError("");
      setDeletingTemplateID(template.id);
      setSelectedHubTemplateId("");
      onTemplateDeleteStarted?.(template.id);
      try {
        await Promise.all([
          queryClient.cancelQueries({ queryKey: workspaceQueryKeys.hubTemplate(template.id) }),
          queryClient.cancelQueries({ queryKey: workspaceQueryKeys.hubWorkspaceScope(template.id) }),
          queryClient.cancelQueries({ queryKey: workspaceQueryKeys.hubWorkspaceFileScope(template.id) }),
        ]);
        await deleteHubTemplateRequest(template.id);
        queryClient.setQueryData<HubTemplate[]>(workspaceQueryKeys.hubTemplates(), (templates) =>
          templates?.filter((item) => item.id !== template.id),
        );
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
    [errorMessage, onTemplateDeleteStarted, queryClient, refreshHubTemplates, setSelectedHubTemplateId, t],
  );

  const deleteSkill = useCallback(
    async (skill: SkillSummary | null | undefined, options: { navigate?: boolean } = {}): Promise<boolean> => {
      const name = String(skill?.name || "").trim();
      if (!name || isReadonlySkill(skill)) {
        return false;
      }
      setSkillDeleteBusy(true);
      setSkillDeleteError("");
      try {
        await deleteSkillRequest(name);
        const nextSkill = nextWorkspaceSkillAfterDelete(workspaceSkills, name);
        setSelectedHubSkillName(nextSkill?.name ?? "");
        setSelectedHubSkillPath(nextSkill ? `${nextSkill.name}/SKILL.md` : "");
        if (options.navigate !== false) onSkillDeleted?.(nextSkill);
        queryClient.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) =>
          (Array.isArray(current) ? current : []).filter((item) => item.name !== name),
        );
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.skills() });
        queryClient.removeQueries({ queryKey: workspaceQueryKeys.skillTree(name), type: "inactive" });
        return true;
      } catch (err) {
        setSkillDeleteError(errorMessage(err, t("resourcesSkillDeleteFailed")));
        return false;
      } finally {
        setSkillDeleteBusy(false);
      }
    },
    [errorMessage, onSkillDeleted, queryClient, setSelectedHubSkillName, setSelectedHubSkillPath, t, workspaceSkills],
  );

  const publishHubTemplate = useCallback(
    async (template: HubTemplate | null | undefined, includeMemory = false): Promise<PublishHubTemplateResult> => {
      if (!template?.id || !isDeletableHubTemplate(template) || !requireOpenCSGAuthentication()) {
        return null;
      }
      setResourcesPublishBusy(true);
      const requestVersion = ++publishRequestVersion.current;
      setPublishFailure(null);
      try {
        const published = await publishHubTemplateToCommunityRequest(template.id, includeMemory);
        await refreshHubTemplates(published);
        setPublishedTemplate(published);
        return { status: "success", template: published };
      } catch (err) {
        if (handleOpenCSGAuthenticationError(err)) {
          return null;
        }
        const message = localizeTemplatePublishError(err, t, t("resourcesPublishCommunityFailed"));
        if (requestVersion === publishRequestVersion.current) {
          setPublishFailure({ templateID: template.id, message });
        }
        return null;
      } finally {
        setResourcesPublishBusy(false);
      }
    },
    [handleOpenCSGAuthenticationError, requireOpenCSGAuthentication, refreshHubTemplates, t],
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
      if (!requireOpenCSGAuthentication()) {
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
        if (!handleOpenCSGAuthenticationError(err)) {
          setResourcesRemoteInstallError(errorMessage(err, t("resourcesSkillRemoteInstallFailed")));
        }
        return null;
      } finally {
        setResourcesRemoteInstallBusy("");
      }
    },
    [
      errorMessage,
      handleOpenCSGAuthenticationError,
      queryClient,
      requireOpenCSGAuthentication,
      setSelectedHubResourceType,
      setSelectedHubSkillName,
      setSelectedHubSkillPath,
      t,
    ],
  );

  return {
    hub: {
      ...hub,
      error: hub.error || mutationError,
      deleteBusy: resourcesDeleteBusy,
      deleteHubTemplate,
      publishBusy: resourcesPublishBusy,
      publishHubTemplate,
      showPublishedTemplate: setPublishedTemplate,
      setPublishError: (message) => setPublishFailure({ templateID: hub.selectedHubTemplateId, message }),
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
        publishedTemplate,
        onDismissPublishedTemplate: () => setPublishedTemplate(null),
        publishError: resourcesPublishError,
        onResetPublishError: resetPublishError,
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
