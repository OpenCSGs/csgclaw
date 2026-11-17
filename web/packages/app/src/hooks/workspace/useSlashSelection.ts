import { useEffect, useRef } from "react";
import type { SlashPickerCandidate } from "@/models/slashCommands";

// Keep the default at the top while skills replace the initial suggested-command list.
// Preserve selection away from the top when another batch is inserted before it.
export function useSlashSelection(
  candidates: SlashPickerCandidate[],
  index: number,
  setIndex: (index: number) => void,
  scope: string,
) {
  const previous = useRef({ candidates, index, scope });
  useEffect(() => {
    const old = previous.current;
    previous.current = { candidates, index, scope };
    if (index === 0 || old.scope !== scope || old.index !== index || old.candidates === candidates) return;
    const selected = old.candidates[old.index];
    if (!selected) return;
    const next = candidates.findIndex(
      (candidate) => candidate.name === selected.name && candidate.type === selected.type,
    );
    const nextIndex = next >= 0 ? next : Math.min(index, Math.max(0, candidates.length - 1));
    if (nextIndex !== index) setIndex(nextIndex);
  }, [candidates, index, setIndex, scope]);
}
