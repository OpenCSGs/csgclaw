import type { SkillContinuation } from "@/models/slashCommands";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import type { SlashPickerCandidate } from "@/models/slashCommands";

export type SlashPickerNavigationInput = {
  continuation?: SkillContinuation;
  event: ReactKeyboardEvent<HTMLElement>;
  candidates: SlashPickerCandidate[];
  activeIndex: number;
  pickerOpen: boolean;
  onIndexChange: (index: number) => void;
  onApply: (value: string) => void;
  onDismiss: () => void;
  onPrepareNavigation?: () => void;
};

export function handleSlashPickerNavigation({
  continuation,
  event,
  candidates,
  activeIndex,
  pickerOpen,
  onIndexChange,
  onApply,
  onDismiss,
  onPrepareNavigation,
}: SlashPickerNavigationInput): boolean {
  if (!pickerOpen) {
    return false;
  }
  const controlNavigation = event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey;
  const navigationKey = event.key.toLowerCase();
  if ((event.key === "ArrowDown" || (controlNavigation && navigationKey === "n")) && candidates.length > 0) {
    event.preventDefault();
    onPrepareNavigation?.();
    if (
      continuation?.hasMore &&
      candidates[activeIndex]?.type === "skill" &&
      candidates[activeIndex + 1]?.type !== "skill"
    ) {
      if (!continuation.loading)
        void continuation.loadMore().then((loaded) => {
          if (loaded) onIndexChange(activeIndex + 1);
        });
      return true;
    }
    onIndexChange((activeIndex + 1) % candidates.length);
    return true;
  }
  if ((event.key === "ArrowUp" || (controlNavigation && navigationKey === "p")) && candidates.length > 0) {
    event.preventDefault();
    onPrepareNavigation?.();
    onIndexChange((activeIndex - 1 + candidates.length) % candidates.length);
    return true;
  }
  if (event.key === "Enter" && !event.shiftKey && candidates.length > 0) {
    event.preventDefault();
    onApply((candidates[activeIndex] ?? candidates[0])?.name ?? "");
    return true;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    onDismiss();
    return true;
  }
  return false;
}
