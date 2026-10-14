import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { HubDetailPane } from "@/components/business/HubDetailPane";

export function HubPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready || !controller.hubViewProps) {
    return null;
  }

  return <HubDetailPane {...controller.hubViewProps} />;
}
