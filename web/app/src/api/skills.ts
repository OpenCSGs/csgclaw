import { del, get, request } from "@/api/client";
import { fetchServerConfig } from "@/api/config";
import { normalizeConfigSettings } from "@/models/configSettings";
import { SKILL_SOURCE_OFFICIAL } from "@/models/skillhub";
import type { SkillFile, SkillSummary, SkillTree } from "@/models/skillhub";

const SKILLS_PATH = "api/v1/skills";
const AGENTICHUB_OFFICIAL_SKILLS_PATH = "api/v1/skills";
const AGENTICHUB_SKILLS_PAGE_SIZE = 16;

export type AgenticHubSkillsPage = {
  hasMore: boolean;
  items: SkillSummary[];
  nextPage: number | null;
  page: number;
  per: number;
  total: number | null;
};

type AgenticHubSkillsResponse = {
  data?: unknown;
  total?: unknown;
};

export function fetchSkills(): Promise<SkillSummary[]> {
  return get<SkillSummary[]>(SKILLS_PATH);
}

export async function fetchAgenticHubOfficialSkillsPage(page = 1, search = ""): Promise<AgenticHubSkillsPage> {
  const currentPage = Math.max(Math.trunc(page), 1);
  const endpoint = await agenticHubOfficialSkillsPath(currentPage, search);
  const payload = await get<AgenticHubSkillsResponse>(endpoint, {
    credentials: "omit",
  });
  const records = Array.isArray(payload?.data) ? payload.data : [];
  const items = records.map(normalizeAgenticHubOfficialSkill).filter((item): item is SkillSummary => Boolean(item));
  const total = nullableNumberFromUnknown(payload?.total);
  const hasMore =
    total === null ? records.length >= AGENTICHUB_SKILLS_PAGE_SIZE : currentPage * AGENTICHUB_SKILLS_PAGE_SIZE < total;
  return {
    hasMore,
    items,
    nextPage: hasMore ? currentPage + 1 : null,
    page: currentPage,
    per: AGENTICHUB_SKILLS_PAGE_SIZE,
    total,
  };
}

export function fetchSkillTree(path = ""): Promise<SkillTree> {
  const params = new URLSearchParams();
  if (path.trim()) {
    params.set("path", path.trim());
  }
  const query = params.toString();
  return get<SkillTree>(`${SKILLS_PATH}/tree${query ? `?${query}` : ""}`);
}

export function fetchSkillFile(path: string): Promise<SkillFile> {
  const params = new URLSearchParams({ path });
  return get<SkillFile>(`${SKILLS_PATH}/file?${params.toString()}`);
}

export function deleteSkillRequest(name: string): Promise<void> {
  return del<void>(`${SKILLS_PATH}/${encodeURIComponent(String(name || "").trim())}`);
}

export function uploadSkillArchive(file: File): Promise<SkillSummary> {
  const formData = new FormData();
  formData.set("file", file);
  return request<SkillSummary>("api/v1/skills:upload", {
    body: formData,
    method: "POST",
  });
}

export function installRemoteSkillRequest(remotePath: string, ref = ""): Promise<SkillSummary> {
  const payload: { ref?: string; remote_path: string } = { remote_path: String(remotePath || "").trim() };
  const normalizedRef = String(ref || "").trim();
  if (normalizedRef) {
    payload.ref = normalizedRef;
  }
  return request<SkillSummary>("api/v1/skills:install", {
    json: payload,
    method: "POST",
  });
}

async function agenticHubOfficialSkillsPath(page: number, search: string): Promise<string> {
  const baseURL = await officialHubBaseURL();
  const params = new URLSearchParams({
    page: String(Math.max(Math.trunc(page), 1)),
    per: String(AGENTICHUB_SKILLS_PAGE_SIZE),
    search: String(search || "").trim(),
    sort: "trending",
    source: "",
  });
  const endpoint = new URL(AGENTICHUB_OFFICIAL_SKILLS_PATH, `${baseURL}/`);
  endpoint.search = params.toString();
  return endpoint.toString();
}

async function officialHubBaseURL(): Promise<string> {
  const settings = normalizeConfigSettings(await fetchServerConfig());
  const baseURL = normalizeOfficialHubBaseURL(settings?.hub_official_url || "");
  if (!baseURL) {
    throw new Error("Official Hub URL is not configured");
  }
  return baseURL;
}

function normalizeOfficialHubBaseURL(value: string): string {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  try {
    const url = new URL(raw);
    url.search = "";
    url.hash = "";
    url.pathname = url.pathname.replace(/\/+$/, "");
    return url.toString().replace(/\/+$/, "");
  } catch (_) {
    return "";
  }
}

function normalizeAgenticHubOfficialSkill(record: unknown): SkillSummary | null {
  return normalizeAgenticHubSkill(record, SKILL_SOURCE_OFFICIAL);
}

function normalizeAgenticHubSkill(record: unknown, source: string): SkillSummary | null {
  if (!record || typeof record !== "object") {
    return null;
  }
  const values = record as Record<string, unknown>;
  const name = stringFromUnknown(values.name) || stringFromUnknown(values.nickname) || skillNameFromPath(values.path);
  const remotePath = stringFromUnknown(values.path);
  if (!name) {
    return null;
  }
  if (source === SKILL_SOURCE_OFFICIAL && !remotePath) {
    return null;
  }
  return {
    description: stringFromUnknown(values.description),
    name,
    readonly: true,
    remoteRef: stringFromUnknown(values.default_branch) || stringFromUnknown(values.defaultBranch) || undefined,
    remotePath: remotePath || undefined,
    source,
  };
}

function skillNameFromPath(value: unknown): string {
  const path = stringFromUnknown(value);
  if (!path) {
    return "";
  }
  const parts = path
    .split("/")
    .map((part) => part.trim())
    .filter(Boolean);
  return parts.at(-1) || "";
}

function stringFromUnknown(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function nullableNumberFromUnknown(value: unknown): number | null {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) && number >= 0 ? number : null;
}
