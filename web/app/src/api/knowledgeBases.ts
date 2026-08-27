import { get } from "@/api/client";
import type { JSONRecord } from "@/models/agents";
import type {
  RemoteKnowledgeBase,
  RemoteKnowledgeBaseMCPConfig,
  RemoteKnowledgeBasePage,
} from "@/models/knowledgeBases";

const REMOTE_KNOWLEDGE_BASES_PATH = "/api/v1/knowledge-bases/remote";
const REMOTE_KNOWLEDGE_BASES_PAGE_SIZE = 50;

type RemoteKnowledgeBaseResponse = {
  items?: unknown;
  page?: unknown;
  per?: unknown;
  total?: unknown;
};

export async function fetchRemoteKnowledgeBases(search = ""): Promise<RemoteKnowledgeBasePage> {
  const params = new URLSearchParams({
    page: "1",
    per: String(REMOTE_KNOWLEDGE_BASES_PAGE_SIZE),
    search: String(search || "").trim(),
  });
  const payload = await get<RemoteKnowledgeBaseResponse>(`${REMOTE_KNOWLEDGE_BASES_PATH}?${params.toString()}`);
  const records = Array.isArray(payload?.items) ? payload.items : [];
  return {
    items: records.map(normalizeKnowledgeBase).filter((item): item is RemoteKnowledgeBase => Boolean(item)),
    page: numberFromUnknown(payload?.page, 1),
    per: numberFromUnknown(payload?.per, REMOTE_KNOWLEDGE_BASES_PAGE_SIZE),
    total: numberFromUnknown(payload?.total, records.length),
  };
}

export async function fetchRemoteKnowledgeBaseMCPConfig(id: string): Promise<RemoteKnowledgeBaseMCPConfig> {
  const payload = await get<{ config?: unknown; name?: unknown }>(
    `${REMOTE_KNOWLEDGE_BASES_PATH}/${encodeURIComponent(String(id || "").trim())}/mcp-config`,
  );
  const name = stringFromUnknown(payload?.name);
  if (!name || !isJSONRecord(payload?.config)) {
    throw new Error("AgenticHub knowledge base returned an invalid MCP configuration");
  }
  return { name, config: payload.config };
}

function normalizeKnowledgeBase(value: unknown): RemoteKnowledgeBase | null {
  if (!isJSONRecord(value)) {
    return null;
  }
  const id = stringFromUnknown(value.id);
  const name = stringFromUnknown(value.name);
  if (!id || !name) {
    return null;
  }
  return {
    availability: value.availability === "available" ? "available" : "unavailable",
    configuredMCPName: stringFromUnknown(value.configured_mcp_name) || undefined,
    contentID: stringFromUnknown(value.content_id),
    description: stringFromUnknown(value.description) || undefined,
    id,
    name,
    unavailableReason: stringFromUnknown(value.unavailable_reason) || undefined,
  };
}

function isJSONRecord(value: unknown): value is JSONRecord {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function stringFromUnknown(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  return typeof value === "string" ? value.trim() : "";
}

function numberFromUnknown(value: unknown, fallback: number): number {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) && number >= 0 ? Math.trunc(number) : fallback;
}
