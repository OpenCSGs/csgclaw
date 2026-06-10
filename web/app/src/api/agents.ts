import { del, get, patch, post, put, requestText } from "@/api/client";
import { BOT_TYPE_NOTIFICATION, MANAGER_AGENT_ID } from "@/shared/constants/agents";
import type { AgentLike, AgentProfileLike, AgentProfileModelsResponse, JSONRecord, RuntimeKind } from "@/models/agents";
import type { WorkspaceFile, WorkspaceListing } from "@/models/workspace";

export type AgentProfileModelRequest = {
  agent_id?: string;
  api_key?: string;
  base_url?: string;
  headers?: JSONRecord;
  provider?: string;
};

export type FetchAgentsOptions = {
  silent?: boolean;
};

export type FetchAgentOptions = {
  cacheBust?: boolean;
};

export type CreateManagerAgentOptions = {
  image?: string;
  runtime_kind?: RuntimeKind;
};

export type FetchAgentLogsOptions = {
  lines?: number;
};

export type DeleteBotOptions = {
  deleteAgent?: boolean;
};

export type AgentUpdatePayload = {
  agent_profile?: JSONRecord;
  avatar?: string;
  description?: string;
  image?: string;
  name?: string;
  role?: string;
  runtime_kind?: RuntimeKind;
  runtime_options?: JSONRecord;
  from_template?: string;
};

export type ParticipantLike = {
  agent_id?: string | null;
  channel?: string | null;
  channel_app_ref?: string | null;
  channel_user_kind?: string | null;
  channel_user_ref?: string | null;
  id?: string | null;
  lifecycle_status?: string | null;
  mentionable?: boolean | null;
  metadata?: JSONRecord | null;
  name?: string | null;
  type?: string | null;
};

function modelPayload(draft: AgentProfileModelRequest): AgentProfileModelRequest {
  return {
    agent_id: draft.agent_id,
    provider: draft.provider,
    base_url: draft.base_url,
    api_key: draft.api_key,
    headers: draft.headers,
  };
}

export function fetchManagerProfile(): Promise<AgentProfileLike> {
  return get(`api/v1/agents/${MANAGER_AGENT_ID}/profile`);
}

export function saveManagerProfileRequest(profile: JSONRecord): Promise<AgentProfileLike> {
  return put(`api/v1/agents/${MANAGER_AGENT_ID}/profile`, profile);
}

export async function fetchAgents(options: FetchAgentsOptions = {}): Promise<AgentLike[]> {
  void options;
  const [agents, notifications] = await Promise.all([
    get<AgentLike[]>("api/v1/agents?include_participants=true"),
    get<ParticipantLike[]>("api/v1/channels/csgclaw/participants?type=notification"),
  ]);
  return [
    ...(Array.isArray(agents) ? agents : []),
    ...(Array.isArray(notifications) ? notifications.map(participantToAgentLike) : []),
  ];
}

export function fetchAgent(agentID: string, options: FetchAgentOptions = {}): Promise<AgentLike> {
  const params = new URLSearchParams({ include_participants: "true" });
  if (options.cacheBust) {
    params.set("_", String(Date.now()));
  }
  return get(`api/v1/agents/${encodeURIComponent(agentID)}?${params.toString()}`);
}

export function fetchAgentLogsRequest(agentID: string, options: FetchAgentLogsOptions = {}): Promise<string> {
  const params = new URLSearchParams();
  const lines = Number(options.lines ?? 400);
  if (Number.isFinite(lines) && lines > 0) {
    params.set("lines", String(Math.floor(lines)));
  }
  return requestText(`api/v1/agents/${encodeURIComponent(agentID)}/logs?${params.toString()}`);
}

export function fetchAgentWorkspace(agentID: string, workspacePath = ""): Promise<WorkspaceListing> {
  const params = new URLSearchParams();
  if (workspacePath.trim()) {
    params.set("path", workspacePath.trim());
  }
  const query = params.toString();
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/workspace${query ? `?${query}` : ""}`);
}

export function fetchAgentWorkspaceFile(agentID: string, workspacePath: string): Promise<WorkspaceFile> {
  const params = new URLSearchParams({ path: workspacePath });
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/workspace/file?${params.toString()}`);
}

export function createManagerAgentRequest(options: CreateManagerAgentOptions = {}): Promise<AgentLike> {
  const payload: { id: string; image?: string; replace: boolean; runtime_kind?: RuntimeKind } = {
    id: MANAGER_AGENT_ID, // Legacy contract: id: "u-manager",
    replace: true,
  };
  if (options.runtime_kind) {
    payload.runtime_kind = options.runtime_kind;
  }
  if (Object.prototype.hasOwnProperty.call(options, "image")) {
    payload.image = options.image;
  }
  return post("api/v1/agents", payload);
}

export function fetchAgentProfileDefaults(): Promise<AgentProfileLike> {
  return get("api/v1/agent-profile-defaults");
}

export function fetchAgentProfile(agentID: string): Promise<AgentProfileLike> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/profile`);
}

export function fetchAgentProfileModels(draft: AgentProfileModelRequest): Promise<AgentProfileModelsResponse> {
  return post("api/v1/agent-profiles/models", modelPayload(draft));
}

export function updateAgentRequest(agentID: string, payload: AgentUpdatePayload): Promise<AgentLike> {
  return patch(`api/v1/agents/${encodeURIComponent(agentID)}`, payload);
}

export type CreateBotPayload = AgentUpdatePayload & {
  type?: string;
};

export async function createBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  const participant = await post<ParticipantLike>("api/v1/channels/csgclaw/participants", {
    name: payload.name,
    type: "agent",
    agent_binding: {
      mode: "create",
      agent: {
        name: payload.name,
        role: payload.role,
        description: payload.description,
        image: payload.image,
        runtime_kind: payload.runtime_kind,
        from_template: payload.from_template,
        runtime_options: payload.runtime_options,
        agent_profile: payload.agent_profile,
      },
    },
  });
  return participant.agent_id ? fetchAgent(participant.agent_id) : participantToAgentLike(participant);
}

export async function createNotificationBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  const participant = await post<ParticipantLike>("api/v1/channels/csgclaw/participants", {
    name: payload.name,
    type: "notification",
    metadata: payload.runtime_options ?? {},
  });
  return participantToAgentLike(participant);
}

export function patchNotificationBotRequest(botID: string, payload: CreateBotPayload): Promise<AgentLike> {
  return patch<ParticipantLike>(`api/v1/channels/csgclaw/participants/${encodeURIComponent(botID)}`, {
    name: payload.name,
    metadata: payload.runtime_options ?? {},
  }).then(participantToAgentLike);
}

export function deleteBotRequest(botID: string, options: DeleteBotOptions = {}): Promise<void> {
  const params = new URLSearchParams();
  if (options.deleteAgent) {
    params.set("delete_agent", "if_unreferenced");
  }
  const query = params.toString();
  return del(`api/v1/channels/csgclaw/participants/${encodeURIComponent(botID)}${query ? `?${query}` : ""}`);
}

export function runAgentActionRequest(agentID: string, action: string): Promise<AgentLike> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/${action}`);
}

function participantToAgentLike(participant: ParticipantLike): AgentLike {
  const metadata = participant.metadata ?? {};
  return {
    id: participant.id,
    name: participant.name,
    type: participant.type === "notification" ? BOT_TYPE_NOTIFICATION : participant.type,
    bot_type: participant.type === "notification" ? BOT_TYPE_NOTIFICATION : participant.type,
    available: participant.lifecycle_status === "active",
    handle: participant.channel_user_ref,
    runtime_options: metadata,
    notification_profile: metadata,
    notifier_profile: metadata,
    status: participant.lifecycle_status,
  };
}
