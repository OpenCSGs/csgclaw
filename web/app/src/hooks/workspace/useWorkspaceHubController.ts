import { useCallback, useEffect, useState } from "react";
import { errorMessage } from "@/api/client";
import { deleteHubTemplateRequest } from "@/api/hub";
import { isDeletableHubTemplate } from "@/models/hubWorkspace";
import type { HubTemplate } from "@/models/hubWorkspace";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";
import type { UseWorkspaceHubControllerArgs } from "./types";

type WorkspaceHubSelection = ReturnType<typeof useWorkspaceHubSelection>;
type DeleteHubTemplate = (template: HubTemplate | null | undefined) => Promise<boolean>;

export type WorkspaceHubController = {
  hub: Omit<WorkspaceHubSelection, "detailPaneProps"> & {
    deleteBusy: boolean;
    deleteHubTemplate: DeleteHubTemplate;
    detailPaneProps: WorkspaceHubSelection["detailPaneProps"] & {
      deleteBusy: boolean;
      onDeleteTemplate: DeleteHubTemplate;
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
  const [hubManualError, setHubManualError] = useState("");
  const [hubDeleteBusy, setHubDeleteBusy] = useState(false);
  const [hubDeleteError, setHubDeleteError] = useState("");

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

  const hub = useWorkspaceHubSelection({
    templates: hubTemplates,
    templatesQuery: hubTemplatesQuery,
    loaded: hubLoaded,
    manualError: hubManualError || hubDeleteError,
    refreshTemplates: refreshHubTemplates,
    t,
  });
  const { setSelectedHubTemplateId } = hub;

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

  return {
    hub: {
      ...hub,
      deleteBusy: hubDeleteBusy,
      deleteHubTemplate,
      detailPaneProps: {
        ...hub.detailPaneProps,
        deleteBusy: hubDeleteBusy,
        onDeleteTemplate: deleteHubTemplate,
      },
    },
    refreshHubTemplates,
  };
}
