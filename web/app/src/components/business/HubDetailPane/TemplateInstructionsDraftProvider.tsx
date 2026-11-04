import { useEffect, useState, type ReactNode } from "react";
import { replaceManagedAgentInstructions } from "@/models/templateInstructions";
import {
  createTemplateInstructionsDraftStore,
  TemplateInstructionsDraftContext,
} from "./templateInstructionsDraftStore";

// The application owns this in-memory session. Drafts survive route unmounts,
// but instructions are never written to browser storage.
export function TemplateInstructionsDraftProvider({ children }: { children: ReactNode }) {
  const [store] = useState(createTemplateInstructionsDraftStore);
  useEffect(() => {
    function handleBeforeUnload(event: BeforeUnloadEvent) {
      const pending = Object.values(store.getSnapshot()).some(
        (draft) => draft.saving || replaceManagedAgentInstructions(draft.document, draft.custom) !== draft.baseline,
      );
      if (!pending) return;
      event.preventDefault();
      event.returnValue = "";
    }
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [store]);
  return (
    <TemplateInstructionsDraftContext.Provider value={store}>{children}</TemplateInstructionsDraftContext.Provider>
  );
}
