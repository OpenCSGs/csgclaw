import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import {
  acknowledgeComposerDraftSaveFailure,
  composerDraftSaveFailed,
  discardAcknowledgedComposerDraft,
  flushComposerDraft,
  getComposerDraft,
  saveComposerDraft,
  subscribeToComposerDraft,
} from "@/shared/storage/composerDrafts";
import type { ComposerAttachmentDraft } from "@/models/attachments";

type ComposerDraft<Content> = { content: Content; attachments: ComposerAttachmentDraft[] };

export function usePersistedComposerDraft<Content>(
  key: string,
  parseContent: (stored: Record<string, unknown>) => Content,
  serializeContent: (content: Content) => Record<string, unknown> | null,
  active = true,
) {
  const subscribe = useCallback((listener: () => void) => subscribeToComposerDraft(key, listener), [key]);
  const getSnapshot = useCallback(() => getComposerDraft(key), [key]);
  const snapshot = useSyncExternalStore(subscribe, getSnapshot);
  const getSaveFailed = useCallback(() => composerDraftSaveFailed(key), [key]);
  const saveFailed = useSyncExternalStore(subscribe, getSaveFailed);
  const retrySave = useCallback(() => flushComposerDraft(key), [key]);
  const dismissSaveError = useCallback(() => acknowledgeComposerDraftSaveFailure(key), [key]);
  useEffect(() => {
    if (active) return () => discardAcknowledgedComposerDraft(key);
  }, [active, key]);
  const draft = useMemo(
    () => ({ content: parseContent(snapshot.content), attachments: snapshot.attachments }),
    [snapshot, parseContent],
  );

  function update(key: string, change: (draft: ComposerDraft<Content>) => ComposerDraft<Content>) {
    if (!key) return;
    const snapshot = getComposerDraft(key);
    const current = { content: parseContent(snapshot.content), attachments: snapshot.attachments };
    const next = change(current);
    if (next === current) return;
    saveComposerDraft(key, serializeContent(next.content), next.attachments);
  }

  return {
    draft,
    update,
    snapshot,
    retrySave: saveFailed ? retrySave : undefined,
    dismissSaveError: saveFailed ? dismissSaveError : undefined,
  };
}
