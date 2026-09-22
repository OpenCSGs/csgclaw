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
        error: agentRuntimes.error,
        installError: agentRuntimes.installError,
        installProgress: agentRuntimes.installProgress,
        installingRuntime: agentRuntimes.installingRuntime,
        loading: agentRuntimes.loading,
        onInstallRuntime: agentRuntimes.install,
        onRetryLoad: agentRuntimes.refresh,
        refreshing: agentRuntimes.refreshing,
        runtimes: agentRuntimes.runtimes,
        t: controller.t,
      }}
    />
  );
}
