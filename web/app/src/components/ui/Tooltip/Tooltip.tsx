import { Tooltip as RadixTooltip } from "radix-ui";
import type { ComponentPropsWithoutRef, ComponentRef, ReactNode } from "react";
import { forwardRef } from "react";
import { classNames } from "@/shared/lib/classNames";

export type TooltipRootProps = ComponentPropsWithoutRef<typeof RadixTooltip.Root>;

export function TooltipRoot({ delayDuration = 250, ...props }: TooltipRootProps) {
  return (
    <RadixTooltip.Provider delayDuration={delayDuration}>
      <RadixTooltip.Root {...props} />
    </RadixTooltip.Provider>
  );
}

export type TooltipTriggerProps = ComponentPropsWithoutRef<typeof RadixTooltip.Trigger>;

export const TooltipTrigger = forwardRef<ComponentRef<typeof RadixTooltip.Trigger>, TooltipTriggerProps>(
  function TooltipTrigger(props, ref) {
    return <RadixTooltip.Trigger ref={ref} {...props} />;
  },
);

export type TooltipContentProps = ComponentPropsWithoutRef<typeof RadixTooltip.Content> & {
  portalContainer?: ComponentPropsWithoutRef<typeof RadixTooltip.Portal>["container"];
};

export const TooltipContent = forwardRef<ComponentRef<typeof RadixTooltip.Content>, TooltipContentProps>(
  function TooltipContent(
    {
      align = "center",
      children,
      className,
      collisionPadding = 12,
      portalContainer,
      side = "right",
      sideOffset = 10,
      ...props
    },
    ref,
  ) {
    return (
      <RadixTooltip.Portal container={portalContainer}>
        <RadixTooltip.Content
          ref={ref}
          align={align}
          className={classNames("csg-tooltip-content", className)}
          collisionPadding={collisionPadding}
          side={side}
          sideOffset={sideOffset}
          {...props}
        >
          {children}
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    );
  },
);

export type TooltipProps = Omit<TooltipRootProps, "content"> & {
  children: ReactNode;
  content: ReactNode;
  contentProps?: Omit<TooltipContentProps, "children">;
};

export function Tooltip({ children, content, contentProps, ...props }: TooltipProps) {
  if (!content) {
    return children;
  }

  return (
    <TooltipRoot {...props}>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent {...contentProps}>{content}</TooltipContent>
    </TooltipRoot>
  );
}
