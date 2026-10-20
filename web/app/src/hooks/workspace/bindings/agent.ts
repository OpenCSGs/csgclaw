import type { IMConversation } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";
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
  onNewTask: (item: AgentLike) => Promise<void>;
}) {
  return {
    agentViewProps: { ...props, activeRoom, onNewTask },
    detailPanelProps:
      detailAgentID && props.item?.id === detailAgentID
        ? {
            ...props,
            activeRoom,
            onNewTask,
            item: props.item,
            onClose,
          }
        : null,
  };
}
