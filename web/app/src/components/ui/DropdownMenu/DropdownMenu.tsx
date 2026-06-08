import { DropdownMenu as RadixDropdownMenu } from "radix-ui";
import { forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef } from "react";
import { classNames } from "@/shared/lib/classNames";

export type DropdownMenuRootProps = ComponentPropsWithoutRef<typeof RadixDropdownMenu.Root>;

export function DropdownMenuRoot(props: DropdownMenuRootProps) {
  return <RadixDropdownMenu.Root {...props} />;
}

export type DropdownMenuTriggerProps = ComponentPropsWithoutRef<typeof RadixDropdownMenu.Trigger>;

export const DropdownMenuTrigger = forwardRef<ComponentRef<typeof RadixDropdownMenu.Trigger>, DropdownMenuTriggerProps>(
  function DropdownMenuTrigger(props, ref) {
    return <RadixDropdownMenu.Trigger ref={ref} {...props} />;
  },
);

export type DropdownMenuContentProps = ComponentPropsWithoutRef<typeof RadixDropdownMenu.Content> & {
  portalContainer?: ComponentPropsWithoutRef<typeof RadixDropdownMenu.Portal>["container"];
};

export const DropdownMenuContent = forwardRef<ComponentRef<typeof RadixDropdownMenu.Content>, DropdownMenuContentProps>(
  function DropdownMenuContent({ align = "end", children, className, portalContainer, sideOffset = 8, ...props }, ref) {
    return (
      <RadixDropdownMenu.Portal container={portalContainer}>
        <RadixDropdownMenu.Content
          ref={ref}
          align={align}
          className={classNames("csg-dropdown-menu-content", className)}
          sideOffset={sideOffset}
          {...props}
        >
          {children}
        </RadixDropdownMenu.Content>
      </RadixDropdownMenu.Portal>
    );
  },
);

export type DropdownMenuItemProps = ComponentPropsWithoutRef<typeof RadixDropdownMenu.Item> & {
  danger?: boolean;
};

export const DropdownMenuItem = forwardRef<ComponentRef<typeof RadixDropdownMenu.Item>, DropdownMenuItemProps>(
  function DropdownMenuItem({ className, danger = false, ...props }, ref) {
    return (
      <RadixDropdownMenu.Item
        ref={ref}
        className={classNames("csg-dropdown-menu-item", danger && "danger", className)}
        {...props}
      />
    );
  },
);

export type DropdownMenuSeparatorProps = ComponentPropsWithoutRef<typeof RadixDropdownMenu.Separator>;

export const DropdownMenuSeparator = forwardRef<
  ComponentRef<typeof RadixDropdownMenu.Separator>,
  DropdownMenuSeparatorProps
>(function DropdownMenuSeparator({ className, ...props }, ref) {
  return (
    <RadixDropdownMenu.Separator
      ref={ref}
      className={classNames("csg-dropdown-menu-separator", className)}
      {...props}
    />
  );
});
