import { useId } from "react";
import { Check, RotateCcw, Sparkles } from "lucide-react";
import { Button, Slider } from "@/components/ui";
import { reasoningEffortLabel } from "@/components/business/ProfileControls";
import { normalizeReasoningEffort, reasoningSliderIndex, reasoningSteps } from "@/models/reasoning";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ReasoningEffortPicker.module.css";

const hintMessageKeys = {
  none: "iaReasoningHintNone",
  minimal: "iaReasoningHintMinimal",
  low: "iaReasoningHintLow",
  medium: "iaReasoningHintMedium",
  high: "iaReasoningHintHigh",
  xhigh: "iaReasoningHintXHigh",
  max: "iaReasoningHintMax",
} as const;

export function ReasoningEffortPicker({
  compact = false,
  value,
  agentEffort,
  supportedEfforts,
  disabled,
  onValueChange,
  onValueCommit,
  t,
}: {
  compact?: boolean;
  value: string;
  agentEffort?: string;
  supportedEfforts?: readonly string[];
  disabled?: boolean;
  onValueChange: (value: string) => void;
  onValueCommit: (value: string) => void;
  t: TranslateFn;
}) {
  const descriptionId = useId();
  const steps = reasoningSteps(supportedEfforts);
  const marks = steps.map((_, index) => index);
  const effective = normalizeReasoningEffort(value || agentEffort || "auto");
  const index = reasoningSliderIndex(effective, steps);
  const label = reasoningEffortLabel(t, effective);
  return (
    <div className={`${styles.root} ${compact ? styles.compact : ""}`}>
      <div className={styles.body}>
        <div className={styles.heading}>
          <span className={styles.title}>{t("iaReasoningTitle")}</span>
          {compact ? <strong className={styles.compactValue}>{label}</strong> : null}
          {!value ? <span className={styles.badge}>{t("iaReasoningFollowing")}</span> : null}
        </div>
        {!compact ? <div className={styles.value}>{label}</div> : null}
        <p hidden={compact} className={styles.description} id={descriptionId}>
          {t(index === null ? "iaReasoningHintAuto" : hintMessageKeys[steps[index]])}
        </p>
        {steps.length > 0 ? (
          <div className={compact ? styles.sliderRow : undefined}>
            {compact ? <span>{t("iaReasoningFaster")}</span> : null}
            <Slider
              className={compact ? styles.compactSlider : undefined}
              value={index ?? steps.indexOf("medium")}
              max={steps.length - 1}
              marks={marks}
              label={t("iaReasoningTitle")}
              valueText={label}
              describedBy={descriptionId}
              disabled={disabled}
              neutral={index === null}
              onValueChange={(next) => onValueChange(steps[next])}
              onValueCommit={(next) => onValueCommit(steps[next])}
            />
            {compact ? <span>{t("iaReasoningSmarter")}</span> : null}
          </div>
        ) : null}
        {!compact && steps.length > 0 ? (
          <div className={styles.endpoints} aria-hidden="true">
            <span>{t("iaReasoningFaster")}</span>
            <span>{t("iaReasoningSmarter")}</span>
          </div>
        ) : null}
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
