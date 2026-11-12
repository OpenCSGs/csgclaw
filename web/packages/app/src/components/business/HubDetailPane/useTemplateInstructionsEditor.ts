import { useContext, useEffect, useState, useSyncExternalStore } from "react";
import { extractManagedAgentInstructions, replaceManagedAgentInstructions } from "@/models/templateInstructions";
import {
  createTemplateInstructionsDraftStore,
  TemplateInstructionsDraftContext,
  type TemplateInstructionsDraft as Draft,
} from "./templateInstructionsDraftStore";

type Mode = "default" | "advanced";

function initialDraft(source: string, mode: Mode = "default"): Draft {
  return {
    source,
    baseline: source,
    document: source,
    custom: extractManagedAgentInstructions(source),
    mode,
    saving: false,
    failed: false,
    saved: false,
    conflict: false,
  };
}

function documentFor(draft: Draft) {
  return replaceManagedAgentInstructions(draft.document, draft.custom);
}

export function useTemplateInstructionsEditor({
  templateId,
  content,
  blocked,
  onSave,
}: {
  templateId: string;
  content: string | null;
  blocked: boolean;
  onSave?: (content: string) => boolean | Promise<boolean>;
}) {
  const sharedStore = useContext(TemplateInstructionsDraftContext);
  const [localStore] = useState(createTemplateInstructionsDraftStore);
  const store = sharedStore ?? localStore;
  const drafts = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const { setDrafts, inFlight } = store;
  const draft = drafts[templateId] ?? initialDraft(content ?? "");
  const dirty = documentFor(draft) !== draft.baseline;
  const editable = !blocked && content !== null && !draft.saving;
  const canSave = editable && dirty && !draft.conflict && Boolean(onSave);

  useEffect(() => {
    if (content === null) return;
    setDrafts((current) => {
      const previous = current[templateId];
      if (previous?.source === content) return current;
      if (previous?.baseline === content) {
        return { ...current, [templateId]: { ...previous, source: content } };
      }
      if (previous && (previous.saving || documentFor(previous) !== previous.baseline)) {
        return {
          ...current,
          [templateId]: {
            ...previous,
            source: content,
            conflict: previous.conflict || (!previous.saving && content !== previous.baseline),
          },
        };
      }
      return { ...current, [templateId]: initialDraft(content, previous?.mode) };
    });
  }, [templateId, content, setDrafts]);

  function update(change: (previous: Draft) => Draft) {
    setDrafts((current) => ({ ...current, [templateId]: change(current[templateId] ?? initialDraft(content ?? "")) }));
  }

  function change(value: string) {
    if (!editable) return;
    update((previous) => ({
      ...previous,
      failed: false,
      saved: false,
      ...(previous.mode === "advanced"
        ? { document: value, custom: extractManagedAgentInstructions(value) }
        : { custom: value }),
    }));
  }

  function setMode(mode: Mode) {
    if (draft.saving) return;
    update((previous) => ({ ...previous, mode }));
  }

  async function save() {
    if (!canSave || !onSave || inFlight.has(templateId)) return;
    const submitted = documentFor(draft);
    const request = Symbol();
    inFlight.set(templateId, request);
    update((previous) => ({ ...previous, saving: true, saved: false, failed: false }));
    let success = false;
    try {
      success = (await onSave(submitted)) === true;
    } catch {
      // Keep the draft available when the caller throws as well as when it returns false.
    } finally {
      if (inFlight.get(templateId) === request) {
        inFlight.delete(templateId);
        update((previous) =>
          success
            ? {
                ...initialDraft(submitted, previous.mode),
                source: previous.source,
                saved: true,
                conflict: previous.source !== draft.source && previous.source !== submitted,
              }
            : {
                ...previous,
                saving: false,
                failed: true,
                conflict: previous.conflict || previous.source !== draft.source,
              },
        );
      }
    }
  }

  return {
    value: draft.mode === "advanced" ? documentFor(draft) : draft.custom,
    mode: draft.mode,
    setMode,
    change,
    save,
    dirty,
    canSave,
    editable,
    saving: draft.saving,
    failed: draft.failed,
    saved: draft.saved,
    conflict: draft.conflict,
    reload: () => {
      if (!draft.saving && content !== null) update(() => initialDraft(content, draft.mode));
    },
  };
}
