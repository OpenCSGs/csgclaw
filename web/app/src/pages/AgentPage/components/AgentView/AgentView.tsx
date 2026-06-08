import { isNotificationBotAgent } from "@/models/agents";
import { AgentDetailPane } from "../AgentDetailPane";
import type { AgentDetailPaneProps } from "../AgentDetailPane";
import { NotificationParticipantDetailPane } from "../NotificationParticipantDetailPane";

export function AgentView(props: AgentDetailPaneProps) {
  if (isNotificationBotAgent(props.item)) {
    return <NotificationParticipantDetailPane {...props} />;
  }
  return <AgentDetailPane {...props} />;
}
