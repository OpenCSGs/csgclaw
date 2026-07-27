import { del, get, request } from "@/api/client";
import { SKILL_SOURCE_OFFICIAL } from "@/models/skillhub";
import type { SkillFile, SkillSummary, SkillTree } from "@/models/skillhub";

const SKILLS_PATH = "api/v1/skills";
const REMOTE_SKILLS_PAGE_SIZE = 16;

export type RemoteSkillsPage = {
  hasMore: boolean;
  items: SkillSummary[];
  nextPage: number | null;
  page: number;
  per: number;
  total: number | null;
};

type RemoteSkillsResponse = {
  items?: unknown;
  next_page?: unknown;
  page?: unknown;
  per?: unknown;
  total?: unknown;
};

export function fetchSkills(): Promise<SkillSummary[]> {
  return get<SkillSummary[]>(SKILLS_PATH);
}

export async function fetchRemoteSkillsPage(page = 1, search = ""): Promise<RemoteSkillsPage> {
  const currentPage = Math.max(Math.trunc(page), 1);
  const params = new URLSearchParams({
    page: String(currentPage),
    per: String(REMOTE_SKILLS_PAGE_SIZE),
    search: String(search || "").trim(),
  });
  const payload = await get<RemoteSkillsResponse>(`${SKILLS_PATH}/remote?${params.toString()}`);
  const records = Array.isArray(payload?.items) ? payload.items : [];
  const responsePage = positiveNumberFromUnknown(payload?.page) || currentPage;
  const per = positiveNumberFromUnknown(payload?.per) || REMOTE_SKILLS_PAGE_SIZE;
  const nextPage = positiveNumberFromUnknown(payload?.next_page);
  return {
    hasMore: nextPage !== null,
    items: records.map(normalizeRemoteSkill).filter((item): item is SkillSummary => Boolean(item)),
    nextPage,
    page: responsePage,
    per,
    total: nullableNumberFromUnknown(payload?.total),
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

function normalizeRemoteSkill(record: unknown): SkillSummary | null {
  if (!record || typeof record !== "object") {
    return null;
  }
  const values = record as Record<string, unknown>;
  const name = stringFromUnknown(values.name);
  const remotePath = stringFromUnknown(values.remote_path);
  if (!name || !remotePath) {
    return null;
  }
  return {
    description: stringFromUnknown(values.description) || undefined,
    name,
    readonly: values.readonly !== false,
    remoteRef: stringFromUnknown(values.remote_ref) || undefined,
    remotePath,
    remoteURL: stringFromUnknown(values.remote_url) || undefined,
    source: stringFromUnknown(values.source) || SKILL_SOURCE_OFFICIAL,
  };
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
