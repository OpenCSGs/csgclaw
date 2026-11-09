import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { TeamDetailPane } from "@/components/business/TeamDetailPane";

export function TeamPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <TeamDetailPane {...controller.teamViewProps} />;
}
