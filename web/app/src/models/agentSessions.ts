export const AGENT_SESSION_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$/;

export type AgentSessionRole = "user" | "assistant";

export type AgentSessionMessage = {
  id: string;
  role: AgentSessionRole;
  content: string;
  createdAt: string;
};

export type AgentSessionResponse = {
  id: string;
  object: "response";
  created_at: number;
  completed_at: number;
  status: "completed";
  model: string;
  output: {
    id: string;
    type: "message";
    status: "completed";
    role: "assistant";
    content: {
      type: "output_text";
      text: string;
      annotations: unknown[];
    }[];
  }[];
  metadata: {
    agent_id: string;
    room_id: string;
    session_id: string;
  };
};

export function isValidAgentSessionID(value: unknown): value is string {
  return typeof value === "string" && AGENT_SESSION_ID_PATTERN.test(value.trim());
}

export function createAgentSessionID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `session-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

export function agentSessionRoomTitle(sessionId: string, agentName: string, agentId: string): string {
  return `Anonymous Session: ${sessionId} | Agent: ${agentName || agentId} (${agentId})`;
}

export function agentSessionResponseText(response: AgentSessionResponse): string {
  return response.output
    .flatMap((item) => item.content)
    .filter((item) => item.type === "output_text")
    .map((item) => item.text)
    .join("\n")
    .trim();
}
