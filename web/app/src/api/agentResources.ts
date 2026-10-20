import { get } from "@/api/client";
import type { AgentSkillSummary } from "@/api/agents";
import type { JSONRecord } from "@/models/agents";
import type { MCPServer } from "@/models/mcp";
import { mcpServerDescription } from "@/models/mcp";

export type ResourceBatch<T> = {
  items: T[];
  total: number;
  list_revision: string;
  next_cursor?: string;
  has_more: boolean;
};
export type ResourceRequest = { search?: string; enabled?: boolean; exclude?: readonly string[] };

export function fetchAgentSkillBatch(
  agentID: string,
  cursor: string,
  request: ResourceRequest = {},
  signal?: AbortSignal,
) {
  return get<ResourceBatch<AgentSkillSummary>>(batchPath(agentID, "skill-summaries", cursor, request), { signal });
}

export async function fetchAgentMCPBatch(
  agentID: string,
  cursor: string,
  signal?: AbortSignal,
): Promise<ResourceBatch<MCPServer>> {
  const result = await get<ResourceBatch<{ name: string; config: JSONRecord }>>(
    batchPath(agentID, "mcp-servers", cursor),
    { signal },
  );
  return {
    ...result,
    items: result.items.map((item) => ({ ...item, description: mcpServerDescription(item.config) })),
  };
}

function batchPath(agentID: string, resource: string, cursor: string, request: ResourceRequest = {}) {
  const params = new URLSearchParams({ pagination: "cursor", limit: "20" });
  if (cursor) params.set("cursor", cursor);
  if (request.search) params.set("search", request.search);
  if (request.enabled !== undefined) params.set("enabled", String(request.enabled));
  for (const name of request.exclude ?? []) params.append("exclude", name);
  return `api/v1/agents/${encodeURIComponent(agentID)}/${resource}?${params}`;
}

export function fetchConversationResourceBatch(
  agentID: string,
  type: import("@/models/conversationResources").ResourceType,
  cursor: string,
  request: ResourceRequest = {},
  signal?: AbortSignal,
) {
  return get<ResourceBatch<import("@/models/conversationResources").AgentResourceSummary>>(
    `${batchPath(agentID, "resources", cursor, request)}&type=${type}`,
    { signal },
  );
}
