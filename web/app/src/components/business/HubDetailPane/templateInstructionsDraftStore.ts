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
  return {
    inFlight: new Set<string>(),
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
  };
}

export const TemplateInstructionsDraftContext = createContext<ReturnType<
  typeof createTemplateInstructionsDraftStore
> | null>(null);
