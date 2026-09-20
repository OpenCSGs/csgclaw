import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { GlobalAppsPanel } from "@/components/business/Apps";

export function AppsPage() {
  const controller = useWorkspaceControllerContext();
  if (!controller.ready) return null;
  return <GlobalAppsPanel t={controller.t} />;
}
