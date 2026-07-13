import { del, get, post, put } from "@/api/client";
import type { JSONRecord } from "@/models/agents";
import type { MCPServerPayload } from "@/models/mcp";

const MCP_SERVERS_PATH = "/api/v1/mcp-servers";

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

function mcpServerPath(name: string): string {
  return `${MCP_SERVERS_PATH}/${encodeURIComponent(String(name || "").trim())}`;
}
