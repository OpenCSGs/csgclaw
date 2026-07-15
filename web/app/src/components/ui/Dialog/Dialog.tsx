import { Dialog as RadixDialog } from "radix-ui";
import { X } from "lucide-react";
import { forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef, HTMLAttributes } from "react";
import { Button } from "@/components/ui/Button";
import { Tooltip } from "@/components/ui/Tooltip";
import { classNames } from "@/shared/lib/classNames";

export type DialogRootProps = ComponentPropsWithoutRef<typeof RadixDialog.Root>;

export function DialogRoot(props: DialogRootProps) {
  return <RadixDialog.Root {...props} />;
}

export type DialogTriggerProps = ComponentPropsWithoutRef<typeof RadixDialog.Trigger>;

export const DialogTrigger = forwardRef<ComponentRef<typeof RadixDialog.Trigger>, DialogTriggerProps>(
  function DialogTrigger(props, ref) {
    return <RadixDialog.Trigger ref={ref} {...props} />;
  },
);

export type DialogPortalProps = ComponentPropsWithoutRef<typeof RadixDialog.Portal>;

export function DialogPortal(props: DialogPortalProps) {
  return <RadixDialog.Portal {...props} />;
}

export type DialogOverlayProps = ComponentPropsWithoutRef<typeof RadixDialog.Overlay>;

export const DialogOverlay = forwardRef<ComponentRef<typeof RadixDialog.Overlay>, DialogOverlayProps>(
  function DialogOverlay({ className, ...props }, ref) {
    return <RadixDialog.Overlay ref={ref} className={classNames("csg-dialog-overlay", className)} {...props} />;
  },
);

export type DialogContentProps = ComponentPropsWithoutRef<typeof RadixDialog.Content> & {
  overlayClassName?: string;
  portalContainer?: DialogPortalProps["container"];
};

export const DialogContent = forwardRef<ComponentRef<typeof RadixDialog.Content>, DialogContentProps>(
  function DialogContent({ children, className, overlayClassName, portalContainer, ...props }, ref) {
    return (
      <DialogPortal container={portalContainer}>
        <DialogOverlay className={overlayClassName} />
        <RadixDialog.Content ref={ref} className={classNames("csg-dialog-content", className)} {...props}>
          {children}
        </RadixDialog.Content>
      </DialogPortal>
    );
  },
);

export type DialogHeaderProps = HTMLAttributes<HTMLDivElement>;

export const DialogHeader = forwardRef<HTMLDivElement, DialogHeaderProps>(function DialogHeader(
  { className, ...props },
  ref,
) {
  return <div ref={ref} className={classNames("csg-dialog-header", className)} {...props} />;
});

export type DialogBodyProps = HTMLAttributes<HTMLDivElement>;

export const DialogBody = forwardRef<HTMLDivElement, DialogBodyProps>(function DialogBody(
  { className, ...props },
  ref,
) {
  return <div ref={ref} className={classNames("csg-dialog-body", className)} {...props} />;
});

export type DialogFooterProps = HTMLAttributes<HTMLDivElement>;

export const DialogFooter = forwardRef<HTMLDivElement, DialogFooterProps>(function DialogFooter(
  { className, ...props },
  ref,
) {
  return <div ref={ref} className={classNames("csg-dialog-footer", className)} {...props} />;
});

export type DialogTitleProps = ComponentPropsWithoutRef<typeof RadixDialog.Title>;

export const DialogTitle = forwardRef<ComponentRef<typeof RadixDialog.Title>, DialogTitleProps>(function DialogTitle(
  { className, ...props },
  ref,
) {
  return <RadixDialog.Title ref={ref} className={classNames("csg-dialog-title", className)} {...props} />;
});

export type DialogDescriptionProps = ComponentPropsWithoutRef<typeof RadixDialog.Description>;

export const DialogDescription = forwardRef<ComponentRef<typeof RadixDialog.Description>, DialogDescriptionProps>(
  function DialogDescription({ className, ...props }, ref) {
    return <RadixDialog.Description ref={ref} className={classNames("csg-dialog-description", className)} {...props} />;
  },
);

export type DialogCloseProps = ComponentPropsWithoutRef<typeof RadixDialog.Close>;

export const DialogClose = forwardRef<ComponentRef<typeof RadixDialog.Close>, DialogCloseProps>(
  function DialogClose(props, ref) {
    return <RadixDialog.Close ref={ref} {...props} />;
  },
);

export type DialogCloseButtonProps = Omit<ComponentPropsWithoutRef<typeof Button>, "aria-label" | "children"> & {
  label: string;
};

export const DialogCloseButton = forwardRef<HTMLButtonElement, DialogCloseButtonProps>(function DialogCloseButton(
  { className, label, title, ...props },
  ref,
) {
  const button = (
    <DialogClose asChild>
      <Button ref={ref} className={classNames("csg-dialog-close", className)} aria-label={label} {...props}>
        <span className="icon-button-mark" aria-hidden="true">
          <X size={18} strokeWidth={2} />
        </span>
      </Button>
    </DialogClose>
  );

  return <Tooltip content={title ?? label}>{button}</Tooltip>;
});
