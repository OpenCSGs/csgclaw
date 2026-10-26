import { Select } from "@/components/ui";
import { normalizeReasoningEffort, reasoningSteps } from "@/models/reasoning";
import type { TranslateFn } from "@/models/conversations";
import { reasoningEffortLabel } from "./reasoningLabels";

export type ReasoningControlsProps = {
  onChange: (value: string) => void;
  t: TranslateFn;
  value: string;
  supportedEfforts?: readonly string[];
};

export function ReasoningControls({ onChange, t, value, supportedEfforts }: ReasoningControlsProps) {
  return (
    <div className="field reasoning-controls-field">
      <span>{t("profileReasoning")}</span>
      <Select
        value={normalizeReasoningEffort(value)}
        onValueChange={onChange}
        triggerProps={{ "aria-label": t("profileReasoningEffort") }}
        options={["auto", ...reasoningSteps(supportedEfforts)].map((option) => ({
          value: option,
          label: reasoningEffortLabel(t, option),
        }))}
      />
    </div>
  );
}
