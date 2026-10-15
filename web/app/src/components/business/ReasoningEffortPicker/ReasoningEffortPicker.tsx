import { useId } from "react";
import { Check, RotateCcw, Sparkles } from "lucide-react";
import { Button, Slider } from "@/components/ui";
import { reasoningEffortLabel } from "@/components/business/ProfileControls";
import { reasoningSliderIndex } from "@/models/reasoning";
import type { TranslateFn } from "@/models/conversations";
import { REASONING_SLIDER_STEPS } from "@/shared/constants/agents";
import styles from "./ReasoningEffortPicker.module.css";

const marks = REASONING_SLIDER_STEPS.map((_, index) => index);
const hintMessageKeys = {
  none: "iaReasoningHintNone",
  minimal: "iaReasoningHintMinimal",
  low: "iaReasoningHintLow",
  medium: "iaReasoningHintMedium",
  high: "iaReasoningHintHigh",
  xhigh: "iaReasoningHintXHigh",
} as const;

export function ReasoningEffortPicker({
  value,
  agentEffort,
  disabled,
  onValueChange,
  onValueCommit,
  t,
}: {
  value: string;
  agentEffort?: string;
  disabled?: boolean;
  onValueChange: (value: string) => void;
  onValueCommit: (value: string) => void;
  t: TranslateFn;
}) {
  const descriptionId = useId();
  const effective = value || agentEffort || "auto";
  const index = reasoningSliderIndex(effective);
  const label = reasoningEffortLabel(t, effective);
  return (
    <div className={styles.root}>
      <div className={styles.body}>
        <div className={styles.heading}>
          <span className={styles.title}>{t("iaReasoningTitle")}</span>
          {!value ? <span className={styles.badge}>{t("iaReasoningFollowing")}</span> : null}
        </div>
        <div className={styles.value}>{label}</div>
        <p className={styles.description} id={descriptionId}>
          {t(index === null ? "iaReasoningHintAuto" : hintMessageKeys[REASONING_SLIDER_STEPS[index]])}
        </p>
        <Slider
          value={index ?? REASONING_SLIDER_STEPS.indexOf("medium")}
          max={REASONING_SLIDER_STEPS.length - 1}
          marks={marks}
          label={t("iaReasoningTitle")}
          valueText={label}
          describedBy={descriptionId}
          disabled={disabled}
          neutral={index === null}
          onValueChange={(next) => onValueChange(REASONING_SLIDER_STEPS[next])}
          onValueCommit={(next) => onValueCommit(REASONING_SLIDER_STEPS[next])}
        />
        <div className={styles.endpoints} aria-hidden="true">
          <span>{t("iaReasoningFaster")}</span>
          <span>{t("iaReasoningSmarter")}</span>
        </div>
      </div>
      <div className={styles.actions}>
        <Button
          variant="tertiaryGray"
          size="sm"
          active={value === "auto"}
          aria-pressed={value === "auto"}
          disabled={disabled || value === "auto"}
          onClick={() => onValueCommit("auto")}
        >
          <Sparkles size={14} aria-hidden="true" />
          {t("profileReasoningAuto")}
          {value === "auto" ? <Check size={14} aria-hidden="true" /> : null}
        </Button>
        <Button
          variant="tertiaryGray"
          size="sm"
          active={!value}
          aria-pressed={!value}
          disabled={disabled || !value}
          onClick={() => onValueCommit("")}
        >
          <RotateCcw size={14} aria-hidden="true" />
          {t("iaReasoningFollowAgent")}
          {!value ? <Check size={14} aria-hidden="true" /> : null}
        </Button>
      </div>
    </div>
  );
}
