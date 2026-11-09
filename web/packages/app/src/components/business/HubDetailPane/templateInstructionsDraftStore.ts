import { createContext } from "react";

export type TemplateInstructionsDraft = {
  source: string;
  baseline: string;
  document: string;
  custom: string;
  mode: "default" | "advanced";
  saving: boolean;
  failed: boolean;
  saved: boolean;
  conflict: boolean;
};

export function createTemplateInstructionsDraftStore() {
  let drafts: Record<string, TemplateInstructionsDraft> = {};
  const listeners = new Set<() => void>();
  const inFlight = new Map<string, symbol>();
  return {
    inFlight,
    getSnapshot: () => drafts,
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    setDrafts: (update: (current: typeof drafts) => typeof drafts) => {
      const next = update(drafts);
      if (next === drafts) return;
      drafts = next;
      listeners.forEach((listener) => listener());
    },
    deleteDraft: (templateId: string) => {
      // Invalidate pending saves as well: their completion must not restore a
      // deleted draft or settle a new save after the same ID is recreated.
      inFlight.delete(templateId);
      if (!Object.hasOwn(drafts, templateId)) return;
      const next = { ...drafts };
      delete next[templateId];
      drafts = next;
      listeners.forEach((listener) => listener());
    },
  };
}

export const TemplateInstructionsDraftContext = createContext<ReturnType<
  typeof createTemplateInstructionsDraftStore
> | null>(null);
