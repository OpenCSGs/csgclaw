import { Select } from "@/components/ui";
import { REASONING_OPTIONS } from "@/shared/constants/agents";
import { normalizeReasoningEffort } from "@/models/reasoning";
import type { TranslateFn } from "@/models/conversations";
import { reasoningOptionMessageKeys } from "./reasoningLabels";

export type ReasoningControlsProps = {
  onChange: (value: string) => void;
  t: TranslateFn;
  value: string;
};

export function ReasoningControls({ onChange, t, value }: ReasoningControlsProps) {
  return (
    <div className="field reasoning-controls-field">
      <span>{t("profileReasoning")}</span>
      <Select
        value={normalizeReasoningEffort(value)}
        onValueChange={onChange}
        triggerProps={{ "aria-label": t("profileReasoningEffort") }}
        options={REASONING_OPTIONS.map((option) => ({
          value: option,
          label: t(reasoningOptionMessageKeys[option]),
        }))}
      />
    </div>
  );
}
