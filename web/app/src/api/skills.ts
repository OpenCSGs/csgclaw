import { del, get, request } from "@/api/client";
import { fetchAuthStatus } from "@/api/auth";
import { fetchServerConfig } from "@/api/config";
import { normalizeAuthStatus } from "@/models/auth";
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
  const baseURL = await officialHubBaseURL();
  const endpoint = agenticHubOfficialSkillsPath(baseURL, currentPage, search);
  const payload = await get<AgenticHubSkillsResponse>(endpoint, {
    credentials: "omit",
  });
  const records = Array.isArray(payload?.data) ? payload.data : [];
  const items = records
    .map((record) => normalizeAgenticHubOfficialSkill(record, baseURL))
    .filter((item): item is SkillSummary => Boolean(item));
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

export function installRemoteSkillRequest(remotePath: string, ref = "", replace = false): Promise<SkillSummary> {
  const payload: { ref?: string; remote_path: string; replace?: boolean } = {
    remote_path: String(remotePath || "").trim(),
  };
  const normalizedRef = String(ref || "").trim();
  if (normalizedRef) {
    payload.ref = normalizedRef;
  }
  if (replace) {
    payload.replace = true;
  }
  return request<SkillSummary>("api/v1/skills:install", {
    json: payload,
    method: "POST",
  });
}

function agenticHubOfficialSkillsPath(baseURL: string, page: number, search: string): string {
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
  const configBaseURL = await officialHubBaseURLFromServerConfig();
  if (configBaseURL) {
    return configBaseURL;
  }
  const authBaseURL = await officialHubBaseURLFromAuthStatus();
  if (authBaseURL) {
    return authBaseURL;
  }
  return "https://hub.opencsg.com";
}

async function officialHubBaseURLFromServerConfig(): Promise<string> {
  try {
    const settings = normalizeConfigSettings(await fetchServerConfig());
    return normalizeOfficialHubBaseURL(settings?.hub_official_url_effective || "");
  } catch (_) {
    return "";
  }
}

async function officialHubBaseURLFromAuthStatus(): Promise<string> {
  try {
    const status = normalizeAuthStatus(await fetchAuthStatus());
    if (!status.authenticated) {
      return "";
    }
    if (status.opencsg_base_url === "https://opencsg-stg.com") {
      return status.opencsg_base_url;
    }
    return normalizeOfficialHubBaseURL(status.base_url || status.opencsg_base_url);
  } catch (_) {
    return "";
  }
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

function normalizeAgenticHubOfficialSkill(record: unknown, baseURL: string): SkillSummary | null {
  return normalizeAgenticHubSkill(record, SKILL_SOURCE_OFFICIAL, baseURL);
}

function normalizeAgenticHubSkill(record: unknown, source: string, baseURL = ""): SkillSummary | null {
  if (!record || typeof record !== "object") {
    return null;
  }
  const values = record as Record<string, unknown>;
  const remotePath = stringFromUnknown(values.path);
  const name =
    usefulSkillTitle(
      stringFromUnknown(values.name),
      stringFromUnknown(values.nickname),
      stringFromUnknown(values.displayName),
      stringFromUnknown(values.display_name),
      stringFromUnknown(values.title),
    ) || skillNameFromPath(remotePath);
  if (!name) {
    return null;
  }
  if (source === SKILL_SOURCE_OFFICIAL && !remotePath) {
    return null;
  }
  return {
    description: stringFromUnknown(values.description) || stringFromUnknown(values.summary),
    name,
    readonly: true,
    remoteRef: stringFromUnknown(values.default_branch) || stringFromUnknown(values.defaultBranch) || undefined,
    remotePath: remotePath || undefined,
    remoteURL: remotePath ? agenticHubSkillWebURL(baseURL, remotePath) || undefined : undefined,
    source,
  };
}

function stringFromUnknown(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function usefulSkillTitle(...values: string[]): string {
  return values.find((value) => value && !isGenericSkillTitle(value)) || "";
}

function isGenericSkillTitle(value: string): boolean {
  return value.trim().toLowerCase() === "skill";
}

function agenticHubSkillWebURL(baseURL: string, remotePath: string): string {
  if (!baseURL || !remotePath) {
    return "";
  }
  try {
    const url = new URL(`${baseURL}/`);
    const pathParts = url.pathname
      .split("/")
      .map((part) => part.trim())
      .filter(Boolean);
    const remotePathParts = remotePath
      .split("/")
      .map((part) => part.trim())
      .filter(Boolean);
    url.pathname = `/${[...pathParts, "skills", ...remotePathParts].join("/")}`;
    url.search = "";
    url.hash = "";
    return url.toString();
  } catch (_) {
    return "";
  }
}

function skillNameFromPath(value: unknown): string {
  const path = stringFromUnknown(value);
  if (!path) {
    return "";
  }
  return (
    path
      .split("/")
      .map((part) => part.trim())
      .filter(Boolean)
      .at(-1) || ""
  );
}

function nullableNumberFromUnknown(value: unknown): number | null {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) && number >= 0 ? number : null;
}
