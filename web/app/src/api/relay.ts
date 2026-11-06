import type { CloudChatResponse, CloudChatTurn } from "@/models/cloudChat";
import type { CloudAgent } from "@/models/cloudAgents";
import { del, get, post } from "@/api/client";

export type RelayPublicationStatus = {
  configured: boolean;
  endpoint: string;
  published: boolean;
  publishing?: boolean;
  pending?: boolean;
  removing?: boolean;
  can_unpublish?: boolean;
  agent?: { id: string; status: string; busy: boolean };
  error?: string;
};
const path = (id: string) => `api/v1/agents/${encodeURIComponent(id)}/relay`;
export const fetchRelayPublication = (id: string): Promise<RelayPublicationStatus> => get(path(id));
export const publishRelayAgent = (id: string): Promise<RelayPublicationStatus> => post(path(id), {});
export const unpublishRelayAgent = (id: string): Promise<RelayPublicationStatus> => del(path(id));

export type RelayCatalogEntry = {
  source?: "platform" | "external";
  creator_name?: string;
  model_id?: string;
  template_repo_path?: string;
  relay_url: string;
  connection_id: string;
  avatar?: string;
  agent: {
    id: string;
    name: string;
    description: string;
    owner_id: string;
    publisher_id: string;
    source_agent_id: string;
    visibility: "public" | "private";
    status: string;
    last_seen_at: string | null;
  };
};

export type RelayCatalogTarget = { agent_id: string; connection_id: string };

export async function fetchRelayCatalog(signal?: AbortSignal, target?: RelayCatalogTarget): Promise<CloudAgent[]> {
  const query = target
    ? `?${new URLSearchParams({ agent_id: target.agent_id, connection_id: target.connection_id })}`
    : "";
  const result = await get<{ data: RelayCatalogEntry[] }>(`api/v1/relay/agents${query}`, { signal });
  return result.data.map((entry) => ({
    id: entry.agent.id,
    relayURL: entry.relay_url,
    connectionID: entry.connection_id,
    name: entry.agent.name,
    description: entry.agent.description,
    owner: entry.agent.owner_id,
    creatorName: entry.creator_name?.trim(),
    modelID: entry.model_id?.trim(),
    templateRepoPath: entry.template_repo_path?.trim(),
    publisher: entry.source === "platform" ? "Agentichub" : "CSGClaw",
    sourceAgentID: entry.agent.source_agent_id,
    visibility: entry.agent.visibility,
    status: entry.agent.status === "online" ? "online" : "offline",
    lastSeenAt: entry.agent.last_seen_at,
    avatar: entry.avatar || "",
  }));
}

export type { CloudChatResponse, CloudChatTurn } from "@/models/cloudChat";
export async function fetchCloudAgentTemplate(agent: CloudAgent, signal: AbortSignal) {
  const query = new URLSearchParams({ connection_id: agent.connectionID });
  const result = await get<{ data: { repo_path: string } | null }>(
    `api/v1/relay/agents/${encodeURIComponent(agent.id)}/template?${query}`,
    { signal },
  );
  return result.data;
}

export function sendCloudAgentMessage(agent: CloudAgent, turn: CloudChatTurn): Promise<CloudChatResponse> {
  return post(`api/v1/relay/agents/${encodeURIComponent(agent.id)}/responses`, {
    connection_id: agent.connectionID,
    key: turn.key,
    input: turn.input,
    previous_response_id: turn.previousResponseID || undefined,
  });
}
export function fetchCloudAgentResponse(agent: CloudAgent, id: string): Promise<CloudChatResponse> {
  return get(
    `api/v1/relay/agents/${encodeURIComponent(agent.id)}/responses/${encodeURIComponent(id)}?${new URLSearchParams({ connection_id: agent.connectionID })}`,
  );
}
