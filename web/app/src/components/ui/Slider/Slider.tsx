import { Slider as RadixSlider } from "radix-ui";
import { useRef } from "react";
import { classNames } from "@/shared/lib/classNames";
import styles from "./Slider.module.css";

export type SliderProps = {
  value: number;
  min?: number;
  max?: number;
  step?: number;
  marks?: readonly number[];
  label: string;
  valueText?: string;
  describedBy?: string;
  disabled?: boolean;
  neutral?: boolean;
  onValueChange: (value: number) => void;
  onValueCommit?: (value: number) => void;
  className?: string;
};

export function Slider({
  value,
  min = 0,
  max = 100,
  step = 1,
  marks = [],
  label,
  valueText,
  describedBy,
  disabled,
  neutral,
  onValueChange,
  onValueCommit,
  className,
}: SliderProps) {
  const pointerInput = useRef(false);
  const pointerValue = useRef(value);
  return (
    <RadixSlider.Root
      className={classNames(styles.root, className)}
      data-csg-control="slider"
      data-neutral={neutral || undefined}
      value={[value]}
      min={min}
      max={max}
      step={step}
      disabled={disabled}
      onPointerDown={() => {
        pointerInput.current = true;
        pointerValue.current = value;
      }}
      onKeyDown={() => {
        pointerInput.current = false;
      }}
      onValueChange={([next]) => {
        pointerValue.current = next;
        onValueChange(next);
      }}
      onValueCommit={([next]) => {
        if (!pointerInput.current) onValueCommit?.(next);
      }}
      onPointerUp={() => {
        // Automatic and explicit selections may share a position. Commit even
        // when a click or drag finishes at the starting numeric value.
        if (!disabled && pointerInput.current) onValueCommit?.(pointerValue.current);
      }}
    >
      <RadixSlider.Track className={styles.track}>
        <RadixSlider.Range className={styles.range} />
      </RadixSlider.Track>
      <span className={styles.marks} aria-hidden="true">
        {marks
          .filter((mark) => mark >= min && mark <= max)
          .map((mark) => (
            <span
              key={mark}
              className={styles.mark}
              data-filled={!neutral && mark <= value ? "" : undefined}
              style={{ left: `${max === min ? 0 : ((mark - min) / (max - min)) * 100}%` }}
            />
          ))}
      </span>
      <RadixSlider.Thumb
        className={styles.thumb}
        tabIndex={disabled ? -1 : 0}
        aria-label={label}
        aria-disabled={disabled || undefined}
        aria-valuetext={valueText}
        aria-describedby={describedBy}
      />
    </RadixSlider.Root>
  );
}
