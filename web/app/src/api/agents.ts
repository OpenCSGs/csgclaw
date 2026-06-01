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

export type CreateManagerAgentOptions = {
  image?: string;
  runtime_kind?: RuntimeKind;
};

export type FetchAgentLogsOptions = {
  lines?: number;
};

export type AgentUpdatePayload = {
  agent_profile?: JSONRecord;
  description?: string;
  image?: string;
  name?: string;
  role?: string;
  runtime_kind?: RuntimeKind;
  runtime_options?: JSONRecord;
  from_template?: string;
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
  const bots = await get<AgentLike[]>("api/v1/channels/csgclaw/bots");
  return Array.isArray(bots) ? bots : [];
}

export function fetchAgent(agentID: string): Promise<AgentLike> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}`);
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

export function createBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  return post("api/v1/channels/csgclaw/bots", payload);
}

export function createNotificationBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  return post("api/v1/channels/csgclaw/bots", { ...payload, type: BOT_TYPE_NOTIFICATION });
}

export function patchNotificationBotRequest(botID: string, payload: CreateBotPayload): Promise<AgentLike> {
  return patch(`api/v1/channels/csgclaw/bots/${encodeURIComponent(botID)}`, payload);
}

export function deleteBotRequest(botID: string): Promise<void> {
  return del(`api/v1/channels/csgclaw/bots/${encodeURIComponent(botID)}`);
}

export function runAgentActionRequest(agentID: string, action: string): Promise<void> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/${action}`);
}
