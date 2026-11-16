import { get, post, resolveRequestPath } from "./client";
import { compareFiles, type WorkspaceListing, type FileOpenTarget, type FileSort } from "@/models/workspace";
import type { AttachmentPreviewItem } from "@/models/attachments";
import type { ComposerFileReference } from "@/models/fileReferences";
import type { IMMessage } from "@/models/conversations";

export type RoomFile = {
  id: string;
  name: string;
  media_type: string;
  size_bytes: number;
  message_id: string;
  created_at?: string;
  download_url?: string;
};
export type RoomFiles = { items: RoomFile[]; total: number; from: number; limit: number };

export function fetchRoomFileMessages(roomID: string, signal?: AbortSignal): Promise<IMMessage[]> {
  return get(`api/v1/messages?${new URLSearchParams({ room_id: roomID, include_thread_replies: "true" })}`, { signal });
}

export function transcriptRoomFiles(messages: readonly IMMessage[]): RoomFile[] {
  const seen = new Set<string>();
  return messages
    .flatMap((message) =>
      (message.attachments ?? []).flatMap((file) => {
        if (seen.has(file.id)) return [];
        seen.add(file.id);
        return [
          {
            id: file.id,
            name: file.name,
            media_type: file.media_type,
            size_bytes: file.size_bytes,
            message_id: message.id ?? "",
            created_at: file.created_at,
            download_url: file.download_url,
          },
        ];
      }),
    )
    .sort((a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? "") || b.id.localeCompare(a.id));
}

export function roomFilePage(files: RoomFile[], query = "", from = 0, sort: FileSort = "updated-desc"): RoomFiles {
  const filtered = files
    .filter((file) => file.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()))
    .sort((a, b) => compareFiles({ name: a.name, date: a.created_at }, { name: b.name, date: b.created_at }, sort));
  return { items: filtered.slice(from, from + 50), total: filtered.length, from, limit: 50 };
}

export function attachmentCapability(file: RoomFile): string {
  try {
    return new URL(file.download_url ?? "", window.location.href).searchParams.get("token") ?? "";
  } catch {
    return "";
  }
}

export function fetchRoomFiles(
  roomID: string,
  query = "",
  from = 0,
  signal?: AbortSignal,
  sort?: FileSort,
): Promise<RoomFiles> {
  const params = new URLSearchParams({ query, from: String(from), limit: "50" });
  if (sort && sort !== "updated-desc") params.set("sort", sort);
  return get(`api/v1/rooms/${encodeURIComponent(roomID)}/attachments?${params}`, { signal });
}

// A separate endpoint prevents older servers from silently ignoring the draft directory.
function workspacePath(agentID: string, roomID: string, workspaceDir: string): string {
  const endpoint = !roomID && workspaceDir ? "draft-workspace" : "workspace";
  return `api/v1/agents/${encodeURIComponent(agentID)}/${endpoint}`;
}

export function fetchWorkspaceDirectory(
  agentID: string,
  path = "",
  signal?: AbortSignal,
  roomID = "",
  workspaceDir = "",
): Promise<WorkspaceListing> {
  const params = new URLSearchParams({ path, depth: "1" });
  if (roomID) params.set("room_id", roomID);
  else if (workspaceDir) params.set("dir", workspaceDir);
  return get(`${workspacePath(agentID, roomID, workspaceDir)}?${params}`, { signal });
}

function workspaceFileParams(roomID: string, workspaceDir: string, fields: Record<string, string> = {}) {
  const params = new URLSearchParams({ ...fields, room_id: roomID });
  if (!roomID && workspaceDir) params.set("dir", workspaceDir);
  return params;
}

export function fileContentURL(roomID: string, ref: ComposerFileReference, token = "", workspaceDir = ""): string {
  return resolveRequestPath(
    ref.source === "workspace"
      ? `${workspacePath(ref.agent_id, roomID, workspaceDir)}/content?${workspaceFileParams(roomID, workspaceDir, { path: ref.path })}`
      : `api/v1/rooms/${encodeURIComponent(roomID)}/attachments/${encodeURIComponent(ref.attachment_id)}${token ? `?${new URLSearchParams({ token })}` : ""}`,
  );
}

export function filePreviewItem(
  roomID: string,
  reference: ComposerFileReference,
  sizeBytes = 0,
  mediaType = "",
  token = "",
  workspaceDir = "",
): AttachmentPreviewItem {
  const url = fileContentURL(roomID, reference, token, workspaceDir);
  return {
    id:
      reference.source === "workspace" ? `workspace:${reference.agent_id}:${reference.path}` : reference.attachment_id,
    name: reference.name,
    mediaType,
    sizeBytes,
    reference,
    downloadURL: url,
    previewURL: url,
  };
}

export type WorkspaceLocation = { absolute_path: string; directory: boolean; open_targets: FileOpenTarget[] };

export function fetchFileLocation(
  roomID: string,
  reference: ComposerFileReference,
  token = "",
  signal?: AbortSignal,
  workspaceDir = "",
): Promise<WorkspaceLocation> {
  return get(
    reference.source === "workspace"
      ? `${workspacePath(reference.agent_id, roomID, workspaceDir)}/location?${workspaceFileParams(roomID, workspaceDir, { path: reference.path })}`
      : `api/v1/rooms/${encodeURIComponent(roomID)}/attachments/${encodeURIComponent(reference.attachment_id)}/location${token ? `?${new URLSearchParams({ token })}` : ""}`,
    { signal },
  );
}

export function openFileLocation(
  roomID: string,
  reference: ComposerFileReference,
  target: string,
  token = "",
  workspaceDir = "",
): Promise<void> {
  return post(
    reference.source === "workspace"
      ? `${workspacePath(reference.agent_id, roomID, workspaceDir)}/open?${workspaceFileParams(roomID, workspaceDir)}`
      : `api/v1/rooms/${encodeURIComponent(roomID)}/attachments/${encodeURIComponent(reference.attachment_id)}/open${token ? `?${new URLSearchParams({ token })}` : ""}`,
    { ...(reference.source === "workspace" ? { path: reference.path } : {}), target },
  );
}
