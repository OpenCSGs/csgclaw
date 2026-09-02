import { del, get, post, put } from "@/api/client";
import type { AgentMCPServersView } from "@/api/agents";
import type { JSONRecord } from "@/models/agents";
import { mcpProbeResultFromResponse } from "@/models/mcp";
import type { MCPProbeResult, MCPServerPayload, MCPServerSourceStatus, RemoteMCPServer } from "@/models/mcp";

const MCP_SERVERS_PATH = "/api/v1/mcp-servers";
const REMOTE_MCP_SERVERS_PAGE_SIZE = 12;

export type RemoteMCPServersPage = {
  hasMore: boolean;
  items: RemoteMCPServer[];
  nextPage: number | null;
  page: number;
  per: number;
  total: number | null;
};

type RemoteMCPServersResponse = {
  items?: unknown;
  next_page?: unknown;
  page?: unknown;
  per?: unknown;
  total?: unknown;
};

type RemoteMCPServerInstallResponse = {
  name?: unknown;
};

type MCPServerSourceStatusResponse = {
  agent_update_available?: unknown;
  auth_type?: unknown;
  configured_endpoint_url?: unknown;
  content_id?: unknown;
  global_server_name?: unknown;
  kind?: unknown;
  latest_endpoint_url?: unknown;
  resource_id?: unknown;
  source_available?: unknown;
  source_description?: unknown;
  source_name?: unknown;
  update_available?: unknown;
};

type MCPServerSourceSyncResponse = {
  source?: unknown;
  state?: unknown;
};

type AgentMCPServerSourceSyncResponse = {
  agent?: unknown;
  source?: unknown;
};

export function fetchMCPServers(): Promise<JSONRecord> {
  return get<JSONRecord>(MCP_SERVERS_PATH);
}

export function createMCPServerRequest(payload: MCPServerPayload): Promise<JSONRecord> {
  return post<JSONRecord>(MCP_SERVERS_PATH, payload);
}

export function updateMCPServerRequest(name: string, payload: MCPServerPayload): Promise<JSONRecord> {
  return put<JSONRecord>(mcpServerPath(name), payload);
}

export function deleteMCPServerRequest(name: string): Promise<JSONRecord> {
  return del<JSONRecord>(mcpServerPath(name));
}

export async function fetchMCPServerSourceStatus(name: string): Promise<MCPServerSourceStatus> {
  const payload = await get<MCPServerSourceStatusResponse>(`${mcpServerPath(name)}/source`);
  return normalizeMCPServerSourceStatus(payload);
}

export async function syncMCPServerSource(name: string): Promise<{ source: MCPServerSourceStatus; state: JSONRecord }> {
  const payload = await post<MCPServerSourceSyncResponse>(`${mcpServerPath(name)}/source:sync`);
  if (!isJSONRecord(payload?.state)) {
    throw new Error("MCP source sync returned an invalid state");
  }
  return {
    source: normalizeMCPServerSourceStatus(payload.source),
    state: payload.state,
  };
}

export async function fetchAgentMCPServerSourceStatus(agentID: string, name: string): Promise<MCPServerSourceStatus> {
  const payload = await get<MCPServerSourceStatusResponse>(`${agentMCPServerPath(agentID, name)}/source`);
  return normalizeMCPServerSourceStatus(payload);
}

export async function syncAgentMCPServerSource(
  agentID: string,
  name: string,
): Promise<{ agent: AgentMCPServersView; source: MCPServerSourceStatus }> {
  const payload = await post<AgentMCPServerSourceSyncResponse>(`${agentMCPServerPath(agentID, name)}/source:sync`);
  if (!isJSONRecord(payload?.agent)) {
    throw new Error("Agent MCP source sync returned an invalid state");
  }
  return {
    agent: payload.agent as AgentMCPServersView,
    source: normalizeMCPServerSourceStatus(payload.source),
  };
}

export async function probeMCPServerRequest(payload: MCPServerPayload): Promise<MCPProbeResult> {
  const response = await post<unknown>(`${MCP_SERVERS_PATH}:probe`, payload);
  const result = mcpProbeResultFromResponse(response);
  if (!result) {
    throw new Error("MCP server probe returned an invalid response");
  }
  return result;
}

export async function installRemoteMCPServerRequest(id: string): Promise<string> {
  const normalizedID = String(id || "").trim();
  if (!normalizedID) {
    throw new Error("remote MCP server id is required");
  }
  const payload = await post<RemoteMCPServerInstallResponse>(
    `${MCP_SERVERS_PATH}/remote/${encodeURIComponent(normalizedID)}/install`,
  );
  const name = stringFromUnknown(payload?.name);
  if (!name) {
    throw new Error("remote MCP server installation returned no name");
  }
  return name;
}

export async function fetchRemoteMCPServersPage(page = 1, search = ""): Promise<RemoteMCPServersPage> {
  const currentPage = Math.max(Math.trunc(page), 1);
  const params = new URLSearchParams({
    page: String(currentPage),
    per: String(REMOTE_MCP_SERVERS_PAGE_SIZE),
    search: String(search || "").trim(),
  });
  const payload = await get<RemoteMCPServersResponse>(`${MCP_SERVERS_PATH}/remote?${params.toString()}`);
  const records = Array.isArray(payload?.items) ? payload.items : [];
  const responsePage = positiveNumberFromUnknown(payload?.page) || currentPage;
  const per = positiveNumberFromUnknown(payload?.per) || REMOTE_MCP_SERVERS_PAGE_SIZE;
  const nextPage = positiveNumberFromUnknown(payload?.next_page);
  return {
    hasMore: nextPage !== null,
    items: records.map(normalizeRemoteMCPServer).filter((item): item is RemoteMCPServer => Boolean(item)),
    nextPage,
    page: responsePage,
    per,
    total: nullableNumberFromUnknown(payload?.total),
  };
}

function mcpServerPath(name: string): string {
  return `${MCP_SERVERS_PATH}/${encodeURIComponent(String(name || "").trim())}`;
}

function agentMCPServerPath(agentID: string, name: string): string {
  return `/api/v1/agents/${encodeURIComponent(String(agentID || "").trim())}/mcp-servers/${encodeURIComponent(
    String(name || "").trim(),
  )}`;
}

function normalizeRemoteMCPServer(record: unknown): RemoteMCPServer | null {
  if (!isJSONRecord(record)) {
    return null;
  }
  const id = stringFromUnknown(record.id);
  const name = stringFromUnknown(record.name);
  const url = stringFromUnknown(record.url);
  if (!id || !name) {
    return null;
  }
  return {
    description: stringFromUnknown(record.description) || undefined,
    id,
    name,
    protocol: stringFromUnknown(record.protocol) || undefined,
    url,
  };
}

export function normalizeMCPServerSourceStatus(record: unknown): MCPServerSourceStatus {
  if (!isJSONRecord(record)) {
    throw new Error("MCP source status returned an invalid response");
  }
  const resourceID = stringFromUnknown(record.resource_id);
  const contentID = stringFromUnknown(record.content_id);
  const kind = stringFromUnknown(record.kind);
  if (!resourceID || !contentID || !kind) {
    throw new Error("MCP source status returned incomplete source identity");
  }
  return {
    agentUpdateAvailable: record.agent_update_available === true,
    authType: stringFromUnknown(record.auth_type),
    configuredEndpointURL: stringFromUnknown(record.configured_endpoint_url),
    contentID,
    globalServerName: stringFromUnknown(record.global_server_name) || undefined,
    kind,
    latestEndpointURL: stringFromUnknown(record.latest_endpoint_url),
    resourceID,
    sourceAvailable: record.source_available !== false,
    sourceDescription: stringFromUnknown(record.source_description) || undefined,
    sourceName: stringFromUnknown(record.source_name) || undefined,
    updateAvailable: record.update_available === true,
  };
}

function isJSONRecord(value: unknown): value is JSONRecord {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function stringFromUnknown(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function positiveNumberFromUnknown(value: unknown): number | null {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) && number >= 1 ? Math.trunc(number) : null;
}

function nullableNumberFromUnknown(value: unknown): number | null {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) && number >= 0 ? number : null;
}
