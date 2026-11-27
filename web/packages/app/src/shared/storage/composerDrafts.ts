import { isComposerFileReference } from "@/models/fileReferences";
import { isResourceSnapshot } from "@/models/agentResources";
import { MAX_ATTACHMENTS_PER_MESSAGE, type ComposerAttachmentDraft } from "@/models/attachments";
import type { ComposerSegment } from "@/models/composer";
import { COMPOSER_DRAFT_STORAGE_PREFIX } from "./keys";

type DraftSnapshot = { content: Record<string, unknown>; attachments: ComposerAttachmentDraft[] };
type CachedDraft = {
  snapshot: DraftSnapshot;
  storedValue: string | null | undefined;
};
type PendingWrite = {
  timer?: number;
  value: object | null;
  state: "pending" | "failed" | "acknowledged" | "discarding";
};
const emptyDraft: DraftSnapshot = { content: {}, attachments: [] };
const drafts = new Map<string, CachedDraft>();
const listeners = new Map<string, Set<() => void>>();
const pending = new Map<string, PendingWrite>();

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

function readStoredValue(key: string): string | null | undefined {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return undefined;
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

function parseStoredDraft(storedValue: string | null | undefined): DraftSnapshot {
  if (!storedValue) return emptyDraft;
  try {
    const draft: unknown = JSON.parse(storedValue);
    if (!record(draft)) return emptyDraft;
    const { attachments, ...content } = draft;
    return { content, attachments: readDraftAttachments(attachments) };
  } catch {
    return emptyDraft;
  }
}

function getCachedDraft(key: string): CachedDraft {
  const cached = drafts.get(key);
  if (cached) return cached;
  const storedValue = readStoredValue(key);
  const draft = { snapshot: parseStoredDraft(storedValue), storedValue };
  drafts.set(key, draft);
  return draft;
}

export function getComposerDraft(key: string): DraftSnapshot {
  return key ? getCachedDraft(key).snapshot : emptyDraft;
}

export function subscribeToComposerDraft(key: string, listener: () => void) {
  const subscribers = listeners.get(key) ?? new Set();
  subscribers.add(listener);
  listeners.set(key, subscribers);
  return () => {
    subscribers.delete(listener);
    if (!subscribers.size) {
      listeners.delete(key);
      evictUnusedDraft(key);
    }
  };
}

function evictUnusedDraft(key: string) {
  if (
    !listeners.has(key) &&
    !pending.has(key) &&
    !drafts.get(key)?.snapshot.attachments.some((attachment) => attachment.file)
  ) {
    drafts.delete(key);
  }
}

export function composerDraftSaveFailed(key: string): boolean {
  return pending.get(key)?.state === "failed";
}

export function acknowledgeComposerDraftSaveFailure(key: string) {
  const item = pending.get(key);
  if (item?.state !== "failed") return;
  window.clearTimeout(item.timer);
  item.timer = undefined;
  item.value = null;
  item.state = "acknowledged";
  listeners.get(key)?.forEach((listener) => listener());
}

export function discardAcknowledgedComposerDraft(key: string) {
  if (pending.get(key)?.state === "acknowledged") flushComposerDraft(key);
}

export function flushComposerDraft(key: string) {
  const item = pending.get(key);
  if (!item) return;
  window.clearTimeout(item.timer);
  item.timer = undefined;
  const wasFailed = item.state === "failed";
  const discard = item.state === "acknowledged";
  const draft = getCachedDraft(key);
  if (discard) {
    draft.snapshot = emptyDraft;
    item.state = "discarding";
  }
  try {
    let storedValue = item.value === null ? null : JSON.stringify(item.value);
    const currentValue = discard ? window.localStorage.getItem(key) : draft.storedValue;
    if (discard && currentValue !== draft.storedValue) {
      // Dismissing the failed draft only discards this tab's edits.
      storedValue = currentValue ?? null;
      draft.snapshot = parseStoredDraft(storedValue);
    } else if (storedValue === null) {
      window.localStorage.removeItem(key);
    } else {
      window.localStorage.setItem(key, storedValue);
    }
    draft.storedValue = storedValue;
    pending.delete(key);
    if (!pending.size) window.removeEventListener("pagehide", flushAll);
    evictUnusedDraft(key);
  } catch {
    // Keep a discarded draft empty even if the browser refuses to remove the old value.
    if (item.state !== "discarding") item.state = "failed";
  }
  if (discard || wasFailed !== composerDraftSaveFailed(key)) listeners.get(key)?.forEach((listener) => listener());
}

function flushAll() {
  for (const [key, item] of pending) {
    // A timer represents a fresh edit, even while an earlier failure is still shown.
    if ((item.state === "failed" && item.timer === undefined) || item.state === "discarding") continue;
    flushComposerDraft(key);
  }
}

export function saveComposerDraft(
  key: string,
  content: Record<string, unknown> | null,
  attachments: ComposerAttachmentDraft[],
) {
  if (!key) return;
  getCachedDraft(key).snapshot = content || attachments.length ? { content: content ?? {}, attachments } : emptyDraft;
  const previous = pending.get(key);
  window.clearTimeout(previous?.timer);
  const value = content || attachments.length ? { ...content, attachments: attachments.map(attachmentMetadata) } : null;
  const item: PendingWrite = { value, state: previous?.state === "failed" ? "failed" : "pending" };
  pending.set(key, item);
  window.addEventListener("pagehide", flushAll);
  if (value === null) {
    flushComposerDraft(key);
  } else {
    item.timer = window.setTimeout(() => flushComposerDraft(key), 250);
  }
  listeners.get(key)?.forEach((listener) => listener());
}
