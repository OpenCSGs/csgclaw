import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { HubView } from "./components";

export function HubPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready || !controller.hubViewProps) {
    return null;
  }

  const props = controller.hubViewProps;
  const connectorRoute = controller.activePane?.type === "apps";
  return (
    <HubView
      {...props}
      hub={
        connectorRoute && props.hub
          ? {
              ...props.hub,
              detailPaneProps: {
                ...props.hub.detailPaneProps,
                selectedResourceType: "mcp",
                selectedMCPServer: null,
                error: props.hub.detailPaneProps.mcpStateError || "",
              },
            }
          : props.hub
      }
    />
  );
}
