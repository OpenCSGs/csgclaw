import type { HubDetailPaneProps } from "@/components/business/HubDetailPane";

export function connectorViewProps(props: HubDetailPaneProps): HubDetailPaneProps {
  if (!props.hub) return props;
  return {
    ...props,
    hub: {
      ...props.hub,
      detailPaneProps: {
        ...props.hub.detailPaneProps,
        selectedResourceType: "mcp",
        selectedMCPServer: null,
        error: props.hub.detailPaneProps.mcpStateError || "",
      },
    },
  };
}
