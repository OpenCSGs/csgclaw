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

export function fetchTaskConversations(q: string, archived = false, offset = 0): Promise<TaskConversationPage> {
  const params = new URLSearchParams({ q, archived: String(archived), offset: String(offset), limit: "20" });
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
