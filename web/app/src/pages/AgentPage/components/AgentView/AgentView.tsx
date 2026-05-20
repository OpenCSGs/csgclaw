import { isNotificationBotAgent } from "@/models/agents";
import { AgentDetailPane } from "../AgentDetailPane";
import { NotificationBotDetailPane } from "../NotificationBotDetailPane";

export function AgentView(props) {
  if (isNotificationBotAgent(props.item)) {
    return <NotificationBotDetailPane {...props} />;
  }
  return <AgentDetailPane {...props} />;
}
