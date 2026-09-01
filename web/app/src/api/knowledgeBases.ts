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

export async function fetchRemoteKnowledgeBases(search = "", page = 1): Promise<RemoteKnowledgeBasePage> {
  const currentPage = Math.max(Math.trunc(page), 1);
  const params = new URLSearchParams({
    page: String(currentPage),
    per: String(REMOTE_KNOWLEDGE_BASES_PAGE_SIZE),
    search: String(search || "").trim(),
  });
  const payload = await get<RemoteKnowledgeBaseResponse>(`${REMOTE_KNOWLEDGE_BASES_PATH}?${params.toString()}`);
  const records = Array.isArray(payload?.items) ? payload.items : [];
  const responsePage = Math.max(numberFromUnknown(payload?.page, currentPage), 1);
  const per = Math.max(numberFromUnknown(payload?.per, REMOTE_KNOWLEDGE_BASES_PAGE_SIZE), 1);
  const total = numberFromUnknown(payload?.total, records.length);
  return {
    items: records.map(normalizeKnowledgeBase).filter((item): item is RemoteKnowledgeBase => Boolean(item)),
    nextPage: responsePage * per < total ? responsePage + 1 : undefined,
    page: responsePage,
    per,
    total,
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
    csgHubResponse: isJSONRecord(value.csghub_response) ? value.csghub_response : undefined,
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
