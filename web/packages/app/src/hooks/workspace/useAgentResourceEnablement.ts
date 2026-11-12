import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useGlobalNotice } from "@/components/ui";
import { setAgentResourceEnabled, type AgentResourceKind } from "@/api/agents";
import { localizeAPIError } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import { workspaceQueryKeys } from "./workspaceQueries";

export function useAgentResourceEnablement(agentID: string, t: TranslateFn, onChanged: (id: string) => Promise<void>) {
  const queryClient = useQueryClient();
  const { showNotice } = useGlobalNotice();
  const inFlight = useRef(false);
  const lastRequest = useRef<{ agentID: string; kind: AgentResourceKind; name: string; enabled: boolean } | null>(null);
  const [state, setState] = useState({ agentID, busy: "", error: "" });
  async function setEnabled(kind: AgentResourceKind, name: string, enabled: boolean): Promise<void> {
    if (!agentID || inFlight.current) return;
    inFlight.current = true;
    lastRequest.current = { agentID, kind, name, enabled };
    setState({ agentID, busy: `${kind}:${name}`, error: "" });
    let failure = "";
    let actualEnabled: boolean | undefined;
    try {
      const result = await setAgentResourceEnabled(agentID, kind, name, enabled);
      actualEnabled = result.enabled;
    } catch (error) {
      failure = localizeAPIError(error, t, t("agentResourceApplyFailed"));
    } finally {
      const refreshed = await Promise.allSettled([
        queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(agentID) }, { throwOnError: true }),
        queryClient.invalidateQueries(
          { queryKey: workspaceQueryKeys.agentMCPServers(agentID) },
          { throwOnError: true },
        ),
        onChanged(agentID),
      ]);
      if (!failure && refreshed.some((result) => result.status === "rejected")) {
        failure = t("agentResourceRefreshFailed");
      }
      if (!failure && actualEnabled !== undefined) {
        showNotice({
          message: t(actualEnabled ? "resourceEnabledNotice" : "resourceDisabledNotice", { name }),
          closeLabel: t("close"),
        });
      }
      inFlight.current = false;
      setState({ agentID, busy: "", error: failure });
    }
  }
  return {
    busy: state.busy,
    error: state.agentID === agentID ? state.error : "",
    setEnabled,
    retry: async () => {
      const request = lastRequest.current;
      if (request?.agentID === agentID) await setEnabled(request.kind, request.name, request.enabled);
    },
  };
}
