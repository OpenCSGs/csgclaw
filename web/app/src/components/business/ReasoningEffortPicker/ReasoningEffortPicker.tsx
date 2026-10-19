import { useId } from "react";
import { Check, Info, RotateCcw, Sparkles } from "lucide-react";
import { Button, Slider, Tooltip } from "@/components/ui";
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
  scopeNote,
  onValueChange,
  onValueCommit,
  t,
}: {
  value: string;
  agentEffort?: string;
  disabled?: boolean;
  scopeNote?: string;
  onValueChange: (value: string) => void;
  onValueCommit: (value: string) => void;
  t: TranslateFn;
}) {
  const descriptionId = useId();
  const effective = value || agentEffort || "auto";
  const index = reasoningSliderIndex(effective);
  const label = reasoningEffortLabel(t, effective);
  const custom = Boolean(value && value !== "auto");
  const mode = !value ? "follow" : custom ? "custom" : "auto";
  return (
    <div className={styles.root}>
      <div className={styles.body}>
        <div className={styles.heading}>
          <div className={styles.headingLabel}>
            <span className={styles.title}>{t("iaReasoningTitle")}</span>
            <Tooltip
              content={scopeNote ?? t("taskConfigurationSessionOnly")}
              contentProps={{ side: "top", sideOffset: 6 }}
            >
              <button type="button" className={styles.info} aria-label={t("iaReasoningInfo")}>
                <Info size={14} aria-hidden="true" />
              </button>
            </Tooltip>
          </div>
          <span className={styles.badge}>
            {t(
              mode === "follow"
                ? "iaReasoningFollowing"
                : mode === "auto"
                  ? "profileReasoningAuto"
                  : "iaReasoningCustom",
            )}
          </span>
        </div>
        {custom ? <div className={styles.value}>{label}</div> : null}
        <p className={styles.description} id={descriptionId}>
          {t(
            mode === "follow"
              ? "iaReasoningHintFollowAgent"
              : mode === "auto"
                ? "iaReasoningHintAuto"
                : index === null
                  ? "iaReasoningHintAuto"
                  : hintMessageKeys[REASONING_SLIDER_STEPS[index]],
          )}
        </p>
        <Slider
          value={index ?? REASONING_SLIDER_STEPS.indexOf("medium")}
          max={REASONING_SLIDER_STEPS.length - 1}
          marks={marks}
          label={t("iaReasoningTitle")}
          valueText={label}
          describedBy={descriptionId}
          disabled={disabled}
          neutral={!custom}
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
          active={mode === "auto"}
          aria-pressed={mode === "auto"}
          disabled={disabled || mode === "auto"}
          onClick={() => onValueCommit("auto")}
        >
          <Sparkles size={14} aria-hidden="true" />
          {t("profileReasoningAuto")}
          {mode === "auto" ? <Check size={14} aria-hidden="true" /> : null}
        </Button>
        <Button
          variant="tertiaryGray"
          size="sm"
          active={mode === "follow"}
          aria-pressed={mode === "follow"}
          disabled={disabled || mode === "follow"}
          onClick={() => onValueCommit("")}
        >
          <RotateCcw size={14} aria-hidden="true" />
          {t("iaReasoningFollowAgent")}
          {mode === "follow" ? <Check size={14} aria-hidden="true" /> : null}
        </Button>
      </div>
    </div>
  );
}
