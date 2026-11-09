import { get, post, resolveRequestPath } from "./client";
import type { WorkspaceListing, FileOpenTarget } from "@/models/workspace";
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

export function roomFilePage(files: RoomFile[], query = "", from = 0): RoomFiles {
  const filtered = files.filter((file) => file.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  return { items: filtered.slice(from, from + 50), total: filtered.length, from, limit: 50 };
}

export function attachmentCapability(file: RoomFile): string {
  try {
    return new URL(file.download_url ?? "", window.location.href).searchParams.get("token") ?? "";
  } catch {
    return "";
  }
}

export function fetchRoomFiles(roomID: string, query = "", from = 0, signal?: AbortSignal): Promise<RoomFiles> {
  const params = new URLSearchParams({ query, from: String(from), limit: "50" });
  return get(`api/v1/rooms/${encodeURIComponent(roomID)}/attachments?${params}`, { signal });
}

export function fetchWorkspaceDirectory(
  agentID: string,
  path = "",
  signal?: AbortSignal,
  roomID = "",
): Promise<WorkspaceListing> {
  const params = new URLSearchParams({ path, depth: "1" });
  if (roomID) params.set("room_id", roomID);
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/workspace?${params}`, { signal });
}

export function fileContentURL(roomID: string, ref: ComposerFileReference, token = ""): string {
  return resolveRequestPath(
    ref.source === "workspace"
      ? `api/v1/agents/${encodeURIComponent(ref.agent_id)}/workspace/content?${new URLSearchParams({ path: ref.path, room_id: roomID })}`
      : `api/v1/rooms/${encodeURIComponent(roomID)}/attachments/${encodeURIComponent(ref.attachment_id)}${token ? `?${new URLSearchParams({ token })}` : ""}`,
  );
}

export function filePreviewItem(
  roomID: string,
  reference: ComposerFileReference,
  sizeBytes = 0,
  mediaType = "",
  token = "",
): AttachmentPreviewItem {
  const url = fileContentURL(roomID, reference, token);
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
): Promise<WorkspaceLocation> {
  return get(
    reference.source === "workspace"
      ? `api/v1/agents/${encodeURIComponent(reference.agent_id)}/workspace/location?${new URLSearchParams({ room_id: roomID, path: reference.path })}`
      : `api/v1/rooms/${encodeURIComponent(roomID)}/attachments/${encodeURIComponent(reference.attachment_id)}/location${token ? `?${new URLSearchParams({ token })}` : ""}`,
    { signal },
  );
}

export function openFileLocation(
  roomID: string,
  reference: ComposerFileReference,
  target: string,
  token = "",
): Promise<void> {
  return post(
    reference.source === "workspace"
      ? `api/v1/agents/${encodeURIComponent(reference.agent_id)}/workspace/open?${new URLSearchParams({ room_id: roomID })}`
      : `api/v1/rooms/${encodeURIComponent(roomID)}/attachments/${encodeURIComponent(reference.attachment_id)}/open${token ? `?${new URLSearchParams({ token })}` : ""}`,
    { ...(reference.source === "workspace" ? { path: reference.path } : {}), target },
  );
}
