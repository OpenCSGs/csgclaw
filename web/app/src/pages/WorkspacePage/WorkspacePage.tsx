import { WorkspaceControllerProvider, useWorkspaceController } from "@/hooks/workspace";
import { WorkspaceLayout } from "./components/WorkspaceLayout";

export function WorkspacePage() {
  const controller = useWorkspaceController();

  return (
    <WorkspaceControllerProvider controller={controller}>
      <WorkspaceLayout />
    </WorkspaceControllerProvider>
  );
}
