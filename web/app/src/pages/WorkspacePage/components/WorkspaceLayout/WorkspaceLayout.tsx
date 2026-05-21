import { useWorkspaceControllerContext } from "@/hooks/workspace";
import {
  AppLayout,
  AppLayoutLoading,
  AppLayoutMain,
  AppLayoutOverlays,
  AppLayoutShell,
  AppLayoutSidebar,
} from "@/components/ui";
import { WorkspaceMainPanel } from "../WorkspaceMainPanel";
import { WorkspaceOverlays } from "../WorkspaceOverlays";
import { WorkspaceSidebar } from "../WorkspaceSidebar";
import { WorkspaceTopBar } from "./WorkspaceTopBar";

export function WorkspaceLayout() {
  const controller = useWorkspaceControllerContext();

  return (
    <AppLayout ready={controller.ready} loadingFallback={<AppLayoutLoading>{controller.loadingText}</AppLayoutLoading>}>
      <AppLayoutShell className={controller.shellClassName}>
        {controller.ready ? <WorkspaceTopBar /> : null}
        <AppLayoutSidebar>
          <WorkspaceSidebar {...controller.sidebarProps} />
        </AppLayoutSidebar>
        <AppLayoutMain>
          <WorkspaceMainPanel />
        </AppLayoutMain>
      </AppLayoutShell>
      <AppLayoutOverlays>
        <WorkspaceOverlays />
      </AppLayoutOverlays>
    </AppLayout>
  );
}
