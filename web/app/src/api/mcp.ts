import { del, get, post, put } from "@/api/client";
import type { JSONRecord } from "@/models/agents";
import type { MCPServerPayload, RemoteMCPServer } from "@/models/mcp";

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
