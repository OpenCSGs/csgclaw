import { Progress } from "radix-ui";
import type { CSSProperties } from "react";
import type snapshot from "../figmaProgress.json";
import styles from "./ProgressFoundations.module.css";

type BarReference = (typeof snapshot.bars)[number];
type ProgressBarProps = { value: number; label: string; name?: string; reference?: BarReference };

export function ProgressBar({ value, label, name, reference }: ProgressBarProps) {
  const floating = label === "Top floating" || label === "Bottom floating";
  const fill = reference ? reference.fillPercent : value;
  const style = { "--progress-fill": `${fill}%` } as CSSProperties;
  const bubbleStyle: CSSProperties = reference
    ? reference.bubbleLeft != null
      ? { left: reference.bubbleLeft, right: "auto", transform: "translateX(-50%)" }
      : { right: reference.bubbleRight ?? 0 }
    : { left: `clamp(32px, ${value}%, calc(100% - 32px))`, transform: "translateX(-50%)" };
  const visual = (
    <>
      <div className={styles.track}>
        <div className={styles.fill} style={style}>
          {floating && reference && (
            <span className={styles.floating} style={bubbleStyle}>
              {value}%
            </span>
          )}
        </div>
        {floating && !reference && (
          <span className={styles.floating} style={bubbleStyle}>
            {value}%
          </span>
        )}
      </div>
      {(label === "Right" || label === "Bottom") && <span className={styles.percent}>{value}%</span>}
    </>
  );
  return name ? (
    <Progress.Root className={styles.bar} value={value} max={100} aria-label={name} data-label={label}>
      <div className={styles.visual} data-label={label} aria-hidden="true">
        {visual}
      </div>
    </Progress.Root>
  ) : (
    <div className={styles.visual} data-label={label} aria-hidden="true">
      {visual}
    </div>
  );
}
