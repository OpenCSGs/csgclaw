import type { IMConversation } from "@/models/conversations";
import type { AgentDetailSidePanelProps } from "../types";
import type { useAgentController } from "../useAgentController";

export function bindAgentViews({
  props,
  activeRoom,
  detailAgentID,
  onClose,
  onNewTask,
}: {
  props: ReturnType<typeof useAgentController>["agentViewProps"];
  activeRoom: IMConversation | null;
  detailAgentID: string;
  onClose: AgentDetailSidePanelProps["onClose"];
  onNewTask?: AgentDetailSidePanelProps["onNewTask"];
}) {
  return {
    agentViewProps: { ...props, activeRoom, onNewTask },
    detailPanelProps:
      detailAgentID && props.item?.id === detailAgentID
        ? {
            ...props,
            activeRoom,
            item: props.item,
            onClose,
            onNewTask,
          }
        : null,
  };
}
