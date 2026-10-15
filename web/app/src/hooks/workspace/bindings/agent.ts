import type { IMConversation } from "@/models/conversations";
import type { AgentDetailSidePanelProps } from "../types";
import type { useAgentController } from "../useAgentController";

export function bindAgentViews({
  props,
  activeRoom,
  detailAgentID,
  onClose,
}: {
  props: ReturnType<typeof useAgentController>["agentViewProps"];
  activeRoom: IMConversation | null;
  detailAgentID: string;
  onClose: AgentDetailSidePanelProps["onClose"];
}) {
  return {
    agentViewProps: { ...props, activeRoom },
    detailPanelProps:
      detailAgentID && props.item?.id === detailAgentID
        ? {
            ...props,
            activeRoom,
            item: props.item,
            onClose,
          }
        : null,
  };
}
