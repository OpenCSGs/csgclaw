import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ComputerView } from "./components";
import { useAgentRuntimes } from "./useAgentRuntimes";

export function ComputerPage() {
  const controller = useWorkspaceControllerContext();
  const agentRuntimes = useAgentRuntimes(controller.t);

  if (!controller.ready) {
    return null;
  }

  return (
    <ComputerView
      {...controller.computerViewProps}
      runtimeSectionProps={{
        busyRuntimeName: agentRuntimes.busyRuntimeName,
        error: agentRuntimes.error,
        installError: agentRuntimes.installError,
        loading: agentRuntimes.loading,
        onInstall: agentRuntimes.installRuntime,
        onRetryLoad: agentRuntimes.refresh,
        refreshing: agentRuntimes.refreshing,
        runtimes: agentRuntimes.runtimes,
        t: controller.t,
      }}
    />
  );
}
