import { useState } from "react";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import type { MCPServer, MCPProbeResult } from "@/models/mcp";
export function useMCPProbe() {
  const controller = useWorkspaceControllerContext();
  const probeMCP = controller.hubViewProps?.hub?.detailPaneProps?.onProbeMCP;
  const [probe, setProbe] = useState<MCPProbeResult | null>(null);
  const [probing, setProbing] = useState(false);
  const run = async (server: MCPServer | null | undefined) => {
    if (!server || probing) {
      return;
    }
    setProbing(true);
    try {
      setProbe((await probeMCP?.({ name: server.name, config: server.config })) ?? null);
    } finally {
      setProbing(false);
    }
  };
  const reset = () => {
    setProbe(null);
    setProbing(false);
  };
  return { probe, probing, run, reset };
}
