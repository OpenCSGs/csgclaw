import styles from "./Checkbox.module.css";
import { Checkbox as RadixCheckbox } from "radix-ui";
import { Check } from "lucide-react";
import { forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef } from "react";
import { classNames } from "@/shared/lib/classNames";

export type CheckboxProps = ComponentPropsWithoutRef<typeof RadixCheckbox.Root>;

export const Checkbox = forwardRef<ComponentRef<typeof RadixCheckbox.Root>, CheckboxProps>(function Checkbox(
  { className, ...props },
  ref,
) {
  return (
    <RadixCheckbox.Root
      ref={ref}
      data-csg-control="checkbox"
      className={classNames(classNames(styles["csg-checkbox"], "csg-checkbox"), className)}
      {...props}
    >
      <RadixCheckbox.Indicator className={styles["csg-checkbox-indicator"]}>
        <Check aria-hidden="true" size={12} strokeWidth={3} />
      </RadixCheckbox.Indicator>
    </RadixCheckbox.Root>
  );
});
