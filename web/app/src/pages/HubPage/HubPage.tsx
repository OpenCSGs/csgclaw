import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { HubView } from "./components";

export function HubPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <HubView {...controller.hubViewProps} />;
}
