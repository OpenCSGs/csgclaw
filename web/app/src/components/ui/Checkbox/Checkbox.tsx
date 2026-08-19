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
    <RadixCheckbox.Root ref={ref} className={classNames("csg-checkbox", className)} {...props}>
      <RadixCheckbox.Indicator className="csg-checkbox-indicator">
        <Check aria-hidden="true" size={12} strokeWidth={3} />
      </RadixCheckbox.Indicator>
    </RadixCheckbox.Root>
  );
});
