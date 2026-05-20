import { useEffect, useState } from "react";
import { useWorkspaceHubSelection } from "./useWorkspaceHubSelection";

export function useWorkspaceHubController({
  hubLoaded,
  hubTemplates,
  hubTemplatesQuery,
  refreshWorkspaceHubTemplates,
  t,
}) {
  const [hubManualError, setHubManualError] = useState("");

  async function refreshHubTemplates() {
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
