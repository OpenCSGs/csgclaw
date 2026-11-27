import { isComposerFileReference } from "@/models/fileReferences";
import { isResourceSnapshot } from "@/models/agentResources";
import { MAX_ATTACHMENTS_PER_MESSAGE, type ComposerAttachmentDraft } from "@/models/attachments";
import type { ComposerSegment } from "@/models/composer";
import { COMPOSER_DRAFT_STORAGE_PREFIX } from "./keys";

type DraftSnapshot = { content: Record<string, unknown>; attachments: ComposerAttachmentDraft[] };
const emptyDraft: DraftSnapshot = { content: {}, attachments: [] };
const drafts = new Map<string, DraftSnapshot>();
const listeners = new Map<string, Set<() => void>>();
const pending = new Map<string, { timer: number; draft: object }>();

export function composerDraftKey(userID: string, roomID?: string): string {
  return userID
    ? COMPOSER_DRAFT_STORAGE_PREFIX + JSON.stringify(roomID === undefined ? [userID] : [userID, roomID])
    : "";
}

export function startTaskDraftKey(userID: string, recipient: string, connectionID = "", workspaceDir = "") {
  return userID ? COMPOSER_DRAFT_STORAGE_PREFIX + JSON.stringify([userID, recipient, connectionID, workspaceDir]) : "";
}

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object";
}

export function isStoredComposerSegment(value: unknown): value is ComposerSegment {
  if (!record(value)) return false;
  if (value.type === "mention") return typeof value.userId === "string" && typeof value.userName === "string";
  if (typeof value.text !== "string") return false;
  switch (value.type) {
    case "text":
    case "slash":
      return true;
    case "resource":
      return isResourceSnapshot(value.resource) && (value.agentID === undefined || typeof value.agentID === "string");
    case "file":
      return isComposerFileReference(value.reference);
    default:
      return false;
  }
}

function readComposerDraft(key: string): Record<string, unknown> {
  if (!key) return {};
  try {
    const draft: unknown = pending.get(key)?.draft ?? JSON.parse(window.localStorage.getItem(key) ?? "null");
    return record(draft) ? draft : {};
  } catch {
    return {};
  }
}

function attachmentMetadata({ id, kind, mediaType, name, sizeBytes }: ComposerAttachmentDraft) {
  return { id, kind, mediaType, name, sizeBytes };
}

export function readDraftAttachments(value: unknown): ComposerAttachmentDraft[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter(
      (item): item is ComposerAttachmentDraft =>
        record(item) &&
        typeof item.id === "string" &&
        typeof item.name === "string" &&
        typeof item.mediaType === "string" &&
        (item.kind === "file" || item.kind === "image") &&
        typeof item.sizeBytes === "number" &&
        Number.isFinite(item.sizeBytes) &&
        item.sizeBytes > 0,
    )
    .slice(0, MAX_ATTACHMENTS_PER_MESSAGE)
    .map(attachmentMetadata);
}

export function getComposerDraft(key: string): DraftSnapshot {
  if (!key) return emptyDraft;
  const cached = drafts.get(key);
  if (cached) return cached;
  const { attachments, ...content } = readComposerDraft(key);
  const draft = { content, attachments: readDraftAttachments(attachments) };
  drafts.set(key, draft);
  return draft;
}

export function subscribeToComposerDraft(key: string, listener: () => void) {
  const subscribers = listeners.get(key) ?? new Set();
  subscribers.add(listener);
  listeners.set(key, subscribers);
  return () => {
    subscribers.delete(listener);
    if (!subscribers.size) {
      listeners.delete(key);
      if (!drafts.get(key)?.attachments.some((attachment) => attachment.file)) drafts.delete(key);
    }
  };
}

function flush(key: string) {
  const item = pending.get(key);
  if (!item) return;
  window.clearTimeout(item.timer);
  pending.delete(key);
  if (!pending.size) window.removeEventListener("pagehide", flushAll);
  try {
    window.localStorage.setItem(key, JSON.stringify(item.draft));
  } catch {
    // Storage can be disabled or full. The current editor remains usable.
  }
}

function flushAll() {
  for (const key of pending.keys()) flush(key);
}

export function saveComposerDraft(
  key: string,
  content: Record<string, unknown> | null,
  attachments: ComposerAttachmentDraft[],
) {
  if (!key) return;
  drafts.set(key, content || attachments.length ? { content: content ?? {}, attachments } : emptyDraft);
  window.clearTimeout(pending.get(key)?.timer);
  if (!content && !attachments.length) {
    pending.delete(key);
    if (!pending.size) window.removeEventListener("pagehide", flushAll);
    try {
      window.localStorage.removeItem(key);
    } catch {
      // Storage can be disabled.
    }
  } else {
    const draft = { ...content, attachments: attachments.map(attachmentMetadata) };
    window.addEventListener("pagehide", flushAll);
    pending.set(key, { draft, timer: window.setTimeout(() => flush(key), 250) });
  }
  listeners.get(key)?.forEach((listener) => listener());
  if (!listeners.has(key) && !attachments.some((attachment) => attachment.file)) drafts.delete(key);
}
