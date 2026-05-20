import { useEffect, useState } from "react";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";
import type { UseWorkspaceHubControllerArgs } from "./types";

export type WorkspaceHubController = {
  hub: ReturnType<typeof useWorkspaceHubSelection>;
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

  async function refreshHubTemplates(): Promise<void> {
    try {
      await refreshWorkspaceHubTemplates();
      setHubManualError("");
    } catch (_) {
      setHubManualError(t("hubLoadFailed"));
    }
  }

  useEffect(() => {
    if (hubTemplatesQuery.isSuccess) {
      setHubManualError("");
    }
  }, [hubTemplatesQuery.isSuccess, hubTemplatesQuery.dataUpdatedAt]);

  const hub = useWorkspaceHubSelection({
    templates: hubTemplates,
    templatesQuery: hubTemplatesQuery,
    loaded: hubLoaded,
    manualError: hubManualError,
    refreshTemplates: refreshHubTemplates,
    t,
  });

  return {
    hub,
    refreshHubTemplates,
  };
}
