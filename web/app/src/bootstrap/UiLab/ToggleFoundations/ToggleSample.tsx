import { useLabTranslation } from "../LabLocale";
import { useId, useState } from "react";
import type { CSSProperties } from "react";
import { Switch } from "@/components/ui";
import snapshot from "../figmaToggles.json";
import styles from "./ToggleFoundations.module.css";

type ToggleSampleProps = {
  size: "sm" | "md";
  type: "Default" | "Slim";
  checked: boolean;
  text: boolean;
  supporting?: boolean;
  disabled?: boolean;
  label?: string;
  description?: string;
  referenceState?: string;
  onCheckedChange?: (checked: boolean) => void;
};

export function ToggleSample({
  size,
  type,
  checked,
  text,
  supporting = true,
  disabled = false,
  label = "Remember me",
  description = "Save my login details for next time.",
  referenceState,
  onCheckedChange,
}: ToggleSampleProps) {
  const { text: labText } = useLabTranslation();

  const id = useId();
  const [hover, setHover] = useState(false);
  const [focus, setFocus] = useState(false);
  const state = referenceState ?? (disabled ? "Disabled" : focus ? "Focus" : hover ? "Hover" : "Default");
  const entry = snapshot.entries.find(
    (item) =>
      item.Type === type &&
      item.Size === size &&
      item.Text === (text ? "True" : "False") &&
      item.Pressed === (checked ? "True" : "False") &&
      item.State === state,
  )!;
  const labelStyle = {
    "--toggle-label-family": `"${entry.labelFamily}", var(--font-family-text)`,
    "--toggle-label-weight": entry.labelWeight,
  } as CSSProperties;
  return (
    <div
      className={styles.sample}
      data-size={size}
      data-type={type}
      data-preview-state={state}
      data-text={text}
      style={labelStyle}
    >
      <Switch
        id={id}
        className={styles.switch}
        size={size}
        checked={checked}
        disabled={disabled || referenceState === "Disabled"}
        onCheckedChange={onCheckedChange}
        aria-label={text ? undefined : labText("切换示例")}
        aria-labelledby={text ? `${id}-label` : undefined}
        aria-describedby={text && supporting ? `${id}-description` : undefined}
        onPointerEnter={() => setHover(true)}
        onPointerLeave={() => setHover(false)}
        onFocus={(event) => setFocus(event.currentTarget.matches(":focus-visible"))}
        onKeyDown={(event) => {
          if (event.key === " " || event.key === "Enter") setFocus(true);
        }}
        onBlur={() => setFocus(false)}
      />
      {text && (
        <div className={styles.copy}>
          <label id={`${id}-label`} htmlFor={id} className={styles.title}>
            {label || "Remember me"}
          </label>
          {supporting && (
            <span id={`${id}-description`} className={styles.description}>
              {description || "Save my login details for next time."}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
