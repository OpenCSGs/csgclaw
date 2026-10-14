import { HubDetailPane } from "@/components/business/HubDetailPane";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { connectorViewProps } from "./connectorViewProps";

export function AppsPage() {
  const controller = useWorkspaceControllerContext();
  if (!controller.ready || !controller.hubViewProps) return null;
  return <HubDetailPane {...connectorViewProps(controller.hubViewProps)} />;
}
