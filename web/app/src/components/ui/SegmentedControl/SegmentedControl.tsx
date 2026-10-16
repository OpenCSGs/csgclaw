import styles from "./SegmentedControl.module.css";
import { ToggleGroup } from "radix-ui";
import type { ReactNode } from "react";
import { Tooltip } from "@/components/ui/Tooltip";
import { classNames } from "@/shared/lib/classNames";

export type SegmentedControlOption<Value extends string> = {
  value: Value;
  label: ReactNode;
  accessibleName?: string;
  disabled?: boolean;
};

export function SegmentedControl<Value extends string>({
  value,
  onChange,
  options,
  label,
  className,
  size = "md",
}: {
  value: Value;
  onChange: (value: Value) => void;
  options: readonly SegmentedControlOption<Value>[];
  label: string;
  className?: string;
  size?: "sm" | "md";
}) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      className={classNames(styles["csg-segmented-control"], className)}
      aria-label={label}
      data-size={size}
      onValueChange={(next) => {
        const option = options.find((item) => item.value === next && !item.disabled);
        if (option) onChange(option.value);
      }}
    >
      {options.map((option) => (
        <Tooltip key={option.value} content={option.accessibleName}>
          <ToggleGroup.Item
            data-csg-control="segmented-control"
            value={option.value}
            disabled={option.disabled}
            className={styles["csg-segmented-control-item"]}
            aria-label={option.accessibleName}
          >
            {option.label}
          </ToggleGroup.Item>
        </Tooltip>
      ))}
    </ToggleGroup.Root>
  );
}
