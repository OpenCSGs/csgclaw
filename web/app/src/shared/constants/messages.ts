export const CSGCLAW_ACTION_CARD_TYPE = "csgclaw.action_card";
export const CSGCLAW_NOTIFY_CARD_TYPE = "csgclaw.notify_card";
export const ACTION_REBUILD_MANAGER = "rebuild-manager";
export const CSGCLAW_AGENT_ACTIVITY_TYPE = "com.opencsg.csgclaw.agent.activity";

export const AgentActivityMsgTypes = {
  action: "com.opencsg.csgclaw.agent.action",
  question: "com.opencsg.csgclaw.agent.question",
  tool: "com.opencsg.csgclaw.agent.tool",
} as const;

export const AgentActivityKinds = {
  message: "message",
  execCommand: "exec_command",
  other: "other",
} as const;
