import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Dispatch, SetStateAction } from "react";
import { isNotifierRuntimeDraft, modelRequestKey } from "@/models/agents";
import type { AgentDraft } from "@/models/agents";
import { useWorkspaceAgentProfileModelsQuery, workspaceQueryKeys } from "./workspaceQueries";

export type UseProfileModelOptionsArgs = {
  draft: AgentDraft | null;
  enabled?: boolean;
  onDraftChange?: Dispatch<SetStateAction<AgentDraft | null>>;
};

export function useProfileModelOptions({ draft, enabled = true, onDraftChange }: UseProfileModelOptionsArgs) {
  const queryClient = useQueryClient();
  const [requestDraft, setRequestDraft] = useState<AgentDraft | null>(null);
  const draftRequestKey = modelRequestKey(draft);
  const requestKey = modelRequestKey(requestDraft);
  const shouldLoad = Boolean(enabled && draft?.provider && !isNotifierRuntimeDraft(draft));

  useEffect(() => {
    if (!shouldLoad) {
      setRequestDraft(null);
      return undefined;
    }
    const timer = window.setTimeout(
      () => {
        setRequestDraft(draft);
      },
      draft.provider === "api" ? 420 : 0,
    );
    return () => window.clearTimeout(timer);
  }, [shouldLoad, draftRequestKey]);

  const query = useWorkspaceAgentProfileModelsQuery(requestDraft, {
    enabled: Boolean(requestDraft),
  });
  const models = useMemo(() => query.data?.models ?? [], [query.data]);

  useEffect(() => {
    if (!models.length || !draft || draft.model_id || !onDraftChange) {
      return;
    }
    if (draftRequestKey !== requestKey) {
      return;
    }
    onDraftChange((current) => {
      if (!current || modelRequestKey(current) !== requestKey || current.model_id) {
        return current;
      }
      return { ...current, model_id: models[0] };
    });
  }, [draft, draftRequestKey, models, onDraftChange, requestKey]);

  const resetModels = useCallback(() => {
    const key = requestKey || draftRequestKey;
    setRequestDraft(null);
    if (key) {
      queryClient.removeQueries({
        queryKey: workspaceQueryKeys.agentProfileModels(key),
        exact: true,
      });
    }
  }, [draftRequestKey, queryClient, requestKey]);

  return {
    models,
    modelBusy: Boolean(requestDraft) && query.isFetching,
    modelError: query.error,
    resetModels,
  };
}
