import type { TaskConfiguration } from "@/models/taskConfiguration";
import { get, request } from "./client";
import type { IMConversation } from "@/models/conversations";

export type TaskConversationPage = { rooms: IMConversation[]; total: number; has_more: boolean };
export type ConversationMetadataPatch = {
  configuration?: TaskConfiguration;
  title?: string;
  pinned?: boolean;
  archived?: boolean;
};

export type TaskConversationPageFilters = {
  owner: string;
  directory: string;
  legacyPeers: readonly string[];
};

export function fetchTaskConversations(
  q: string,
  archived = false,
  offset = 0,
  filters?: TaskConversationPageFilters,
): Promise<TaskConversationPage> {
  const params = new URLSearchParams({ q, archived: String(archived), offset: String(offset), limit: "20" });
  if (filters?.owner) params.set("owner", filters.owner);
  if (filters?.directory) {
    params.set("directory", filters.directory);
    for (const peer of filters.legacyPeers) params.append("legacy_peer", peer);
  }
  return get(`/api/v1/conversations?${params}`);
}

type TaskConversationFilterGroup = {
  is_direct: boolean;
  peer?: string;
  cloud_user?: string;
  workspace_dir?: string;
  count: number;
};

export function fetchTaskConversationFilters(archived: boolean): Promise<TaskConversationFilterGroup[]> {
  return get(`/api/v1/conversations/filters?archived=${archived}`);
}

export function updateConversationMetadata(id: string, patch: ConversationMetadataPatch): Promise<IMConversation> {
  return request(`/api/v1/rooms/${encodeURIComponent(id)}`, { method: "PATCH", json: patch });
}
