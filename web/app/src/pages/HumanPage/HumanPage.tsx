import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { HumanDetailPane } from "./components";

export function HumanPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <HumanDetailPane {...controller.humanViewProps} />;
}
