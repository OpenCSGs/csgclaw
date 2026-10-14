import styles from "./Switch.module.css";
import { Switch as RadixSwitch } from "radix-ui";
import { forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef } from "react";
import { classNames } from "@/shared/lib/classNames";

export type SwitchProps = ComponentPropsWithoutRef<typeof RadixSwitch.Root>;

export const Switch = forwardRef<ComponentRef<typeof RadixSwitch.Root>, SwitchProps>(function Switch(
  { className, ...props },
  ref,
) {
  return (
    <RadixSwitch.Root
      ref={ref}
      data-csg-control="switch"
      className={classNames(classNames(styles["csg-switch"], "csg-switch"), className)}
      {...props}
    >
      <RadixSwitch.Thumb className={classNames(styles["csg-switch-thumb"], "csg-switch-thumb")} />
    </RadixSwitch.Root>
  );
});
