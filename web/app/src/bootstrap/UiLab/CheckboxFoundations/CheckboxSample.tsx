import { useId } from "react";
import { Checkbox } from "@/components/ui";
import type { CheckboxProps } from "@/components/ui/Checkbox";
import checkSm from "./assets/dccc8.svg";
import checkMd from "./assets/89ed8.svg";
import checkDisabledSm from "./assets/6e59f.svg";
import checkDisabledMd from "./assets/33d50.svg";
import minusSm from "./assets/43134.svg";
import minusMd from "./assets/6ec2a.svg";
import minusDisabledSm from "./assets/02e67.svg";
import minusDisabledMd from "./assets/19eb7.svg";
import styles from "./CheckboxFoundations.module.css";

type CheckboxSampleProps = {
  size: "sm" | "md";
  checked: NonNullable<CheckboxProps["checked"]>;
  text?: boolean;
  supporting?: boolean;
  label?: string;
  description?: string;
  disabled?: boolean;
  referenceState?: string;
  onCheckedChange?: CheckboxProps["onCheckedChange"];
};

export function CheckboxSample({
  size,
  checked,
  text = true,
  supporting = true,
  label = "Remember me",
  description = "Save my login details for next time.",
  disabled = false,
  referenceState,
  onCheckedChange,
}: CheckboxSampleProps) {
  const id = useId();
  const unavailable = disabled || referenceState === "Disabled";
  const mixed = checked === "indeterminate";
  const icon = mixed
    ? unavailable
      ? size === "sm"
        ? minusDisabledSm
        : minusDisabledMd
      : size === "sm"
        ? minusSm
        : minusMd
    : unavailable
      ? size === "sm"
        ? checkDisabledSm
        : checkDisabledMd
      : size === "sm"
        ? checkSm
        : checkMd;
  return (
    <div className={styles.sample} data-size={size} data-text={text} data-preview-state={referenceState}>
      <Checkbox
        id={id}
        className={styles.checkbox}
        checked={checked}
        disabled={unavailable}
        onCheckedChange={onCheckedChange}
        aria-label={text ? undefined : label}
        aria-labelledby={text ? `${id}-label` : undefined}
        aria-describedby={text && supporting ? `${id}-description` : undefined}
        indicator={<img src={icon} alt="" />}
      />
      {text && (
        <div className={styles.copy}>
          <label id={`${id}-label`} htmlFor={id} className={styles.title}>
            {label}
          </label>
          {supporting && (
            <span id={`${id}-description`} className={styles.description}>
              {description}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
