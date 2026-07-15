import { Popover as RadixPopover } from "radix-ui";
import { forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef } from "react";
import { classNames } from "@/shared/lib/classNames";

export type PopoverRootProps = ComponentPropsWithoutRef<typeof RadixPopover.Root>;

export function PopoverRoot(props: PopoverRootProps) {
  return <RadixPopover.Root {...props} />;
}

export type PopoverTriggerProps = ComponentPropsWithoutRef<typeof RadixPopover.Trigger>;

export const PopoverTrigger = forwardRef<ComponentRef<typeof RadixPopover.Trigger>, PopoverTriggerProps>(
  function PopoverTrigger(props, ref) {
    return <RadixPopover.Trigger ref={ref} {...props} />;
  },
);

export type PopoverCloseProps = ComponentPropsWithoutRef<typeof RadixPopover.Close>;

export const PopoverClose = forwardRef<ComponentRef<typeof RadixPopover.Close>, PopoverCloseProps>(
  function PopoverClose(props, ref) {
    return <RadixPopover.Close ref={ref} {...props} />;
  },
);

export type PopoverContentProps = ComponentPropsWithoutRef<typeof RadixPopover.Content> & {
  portalContainer?: ComponentPropsWithoutRef<typeof RadixPopover.Portal>["container"];
};

export const PopoverContent = forwardRef<ComponentRef<typeof RadixPopover.Content>, PopoverContentProps>(
  function PopoverContent(
    { align = "start", children, className, collisionPadding = 12, portalContainer, sideOffset = 8, ...props },
    ref,
  ) {
    return (
      <RadixPopover.Portal container={portalContainer}>
        <RadixPopover.Content
          ref={ref}
          align={align}
          className={classNames("csg-popover-content", className)}
          collisionPadding={collisionPadding}
          sideOffset={sideOffset}
          {...props}
        >
          {children}
        </RadixPopover.Content>
      </RadixPopover.Portal>
    );
  },
);
