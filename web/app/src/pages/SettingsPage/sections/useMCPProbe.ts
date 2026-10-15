import { useCallback, useRef, useState } from "react";
import { localizeAPIError } from "@/shared/i18n";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import type { MCPServer, MCPProbeResult } from "@/models/mcp";
export function useMCPProbe() {
  const controller = useWorkspaceControllerContext();
  const detail = controller.hubViewProps?.hub?.detailPaneProps;
  const probeMCP = detail?.onProbeMCP;
  const [probe, setProbe] = useState<MCPProbeResult | null>(null);
  const [probing, setProbing] = useState(false);
  const [localError, setLocalError] = useState("");
  const requestID = useRef(0);
  const run = async (server: MCPServer | null | undefined) => {
    if (!server || probing) {
      return;
    }
    const currentRequest = ++requestID.current;
    setProbing(true);
    setProbe(null);
    setLocalError("");
    const failed: MCPProbeResult = {
      connected: false,
      durationMs: 0,
      tools: [],
      toolsSupported: false,
      truncated: false,
    };
    try {
      const result = await probeMCP?.({ name: server.name, config: server.config });
      if (requestID.current === currentRequest) setProbe(result ?? failed);
    } catch (error) {
      if (requestID.current === currentRequest) {
        setProbe(failed);
        setLocalError(localizeAPIError(error, controller.t, controller.t("resourcesMCPTestFailed")));
      }
    } finally {
      if (requestID.current === currentRequest) setProbing(false);
    }
  };
  const reset = useCallback(() => {
    requestID.current += 1;
    setProbe(null);
    setProbing(false);
    setLocalError("");
  }, []);
  const error =
    probe?.connected === false ? localError || detail?.mcpProbeError || controller.t("resourcesMCPTestFailed") : "";
  return { probe, probing, error, run, reset };
}
