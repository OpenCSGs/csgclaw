/*! Derived from CSGClaw. Modified for standalone csgclaw-ui distribution. */
import styles from "./Dialog.module.css";
import { Dialog as RadixDialog } from "radix-ui";
import { X } from "lucide-react";
import { forwardRef, useEffect } from "react";
import type { ComponentPropsWithoutRef, ComponentRef, HTMLAttributes } from "react";
import { Button } from "../Button/index.js";
import { Tooltip } from "../Tooltip/index.js";
import { classNames, legacyClassName } from "../../lib/classNames.js";

const dialogFocusableSelector = [
  "button:not([disabled])",
  "[href]",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
].join(",");

const overlaySelector = `.${styles["csg-dialog-overlay"]}`;
const contentSelector = `.${styles["csg-dialog-content"]}`;
const layerSelector = `${overlaySelector}, ${contentSelector}`;

let dialogLayerObserver: MutationObserver | undefined;
let dialogLayerObserverUsers = 0;

function syncDialogLayers() {
  // Radix appends each portal in open order, so the DOM order represents the modal stack.
  const overlays = document.querySelectorAll<HTMLElement>(overlaySelector);
  const contents = document.querySelectorAll<HTMLElement>(contentSelector);
  overlays.forEach((overlay, index) => {
    overlay.style.zIndex = `calc(var(--csgclaw-ui-z-modal) + ${index * 2})`;
  });
  contents.forEach((content, index) => {
    content.style.zIndex = `calc(var(--csgclaw-ui-z-modal) + ${index * 2 + 1})`;
  });
}

function isDialogLayerNode(node: Node) {
  return node instanceof HTMLElement && (node.matches(layerSelector) || Boolean(node.querySelector(layerSelector)));
}

function observeDialogLayers() {
  if (typeof document === "undefined" || !document.body) return () => undefined;
  dialogLayerObserverUsers += 1;
  if (!dialogLayerObserver) {
    syncDialogLayers();
    dialogLayerObserver = new MutationObserver((records) => {
      if (records.some((record) => [...record.addedNodes, ...record.removedNodes].some(isDialogLayerNode))) {
        syncDialogLayers();
      }
    });
    dialogLayerObserver.observe(document.body, { childList: true, subtree: true });
  }
  return () => {
    dialogLayerObserverUsers -= 1;
    if (dialogLayerObserverUsers === 0) {
      dialogLayerObserver?.disconnect();
      dialogLayerObserver = undefined;
    }
  };
}

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
  function DialogOverlay({ className, style, ...props }, ref) {
    return (
      <RadixDialog.Overlay
        ref={ref}
        className={classNames(
          classNames(styles["csg-dialog-overlay"], legacyClassName("csg-dialog-overlay")),
          className,
        )}
        style={style}
        {...props}
      />
    );
  },
);

export type DialogContentProps = ComponentPropsWithoutRef<typeof RadixDialog.Content> & {
  overlayClassName?: string;
  portalContainer?: DialogPortalProps["container"];
  scrollBody?: boolean;
};

export const DialogContent = forwardRef<ComponentRef<typeof RadixDialog.Content>, DialogContentProps>(
  function DialogContent(
    {
      children,
      className,
      onOpenAutoFocus,
      overlayClassName,
      portalContainer,
      scrollBody,
      style,
      tabIndex = -1,
      ...props
    },
    ref,
  ) {
    useEffect(() => observeDialogLayers(), []);
    return (
      <DialogPortal container={portalContainer}>
        <DialogOverlay className={overlayClassName} />
        <RadixDialog.Content
          ref={ref}
          className={classNames(
            classNames(styles["csg-dialog-content"], legacyClassName("csg-dialog-content")),
            className,
          )}
          tabIndex={tabIndex}
          data-scroll-body={scrollBody ? "" : undefined}
          onOpenAutoFocus={(event) => {
            onOpenAutoFocus?.(event);
            if (event.defaultPrevented) {
              return;
            }
            const content = event.currentTarget as HTMLElement | null;
            const firstFocusable = content?.querySelector<HTMLElement>(dialogFocusableSelector);
            if (!firstFocusable?.hasAttribute("data-csg-tooltip-trigger")) {
              return;
            }
            event.preventDefault();
            content?.focus();
          }}
          {...props}
          style={style}
        >
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
  return <div ref={ref} className={classNames(styles["csg-dialog-header"], className)} {...props} />;
});

export type DialogBodyProps = HTMLAttributes<HTMLDivElement>;

export const DialogBody = forwardRef<HTMLDivElement, DialogBodyProps>(function DialogBody(
  { className, ...props },
  ref,
) {
  return (
    <div
      ref={ref}
      className={classNames(classNames(styles["csg-dialog-body"], legacyClassName("csg-dialog-body")), className)}
      {...props}
    />
  );
});

export type DialogFooterProps = HTMLAttributes<HTMLDivElement>;

export const DialogFooter = forwardRef<HTMLDivElement, DialogFooterProps>(function DialogFooter(
  { className, ...props },
  ref,
) {
  return (
    <div
      ref={ref}
      className={classNames(classNames(styles["csg-dialog-footer"], legacyClassName("csg-dialog-footer")), className)}
      {...props}
    />
  );
});

export type DialogTitleProps = ComponentPropsWithoutRef<typeof RadixDialog.Title>;

export const DialogTitle = forwardRef<ComponentRef<typeof RadixDialog.Title>, DialogTitleProps>(function DialogTitle(
  { className, ...props },
  ref,
) {
  return (
    <RadixDialog.Title
      ref={ref}
      className={classNames(classNames(styles["csg-dialog-title"], legacyClassName("csg-dialog-title")), className)}
      {...props}
    />
  );
});

export type DialogDescriptionProps = ComponentPropsWithoutRef<typeof RadixDialog.Description> & {
  size?: "sm" | "md";
};

export const DialogDescription = forwardRef<ComponentRef<typeof RadixDialog.Description>, DialogDescriptionProps>(
  function DialogDescription({ className, size = "sm", ...props }, ref) {
    return (
      <RadixDialog.Description
        ref={ref}
        data-size={size}
        className={classNames(
          classNames(styles["csg-dialog-description"], legacyClassName("csg-dialog-description")),
          className,
        )}
        {...props}
      />
    );
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
  iconSize?: number;
};

export const DialogCloseButton = forwardRef<HTMLButtonElement, DialogCloseButtonProps>(function DialogCloseButton(
  { className, label, title, iconSize, ...props },
  ref,
) {
  const button = (
    <DialogClose asChild>
      <Button
        ref={ref}
        className={classNames(classNames(styles["csg-dialog-close"], legacyClassName("csg-dialog-close")), className)}
        aria-label={label}
        {...props}
      >
        <span
          className={classNames(styles["icon-button-mark"], legacyClassName("icon-button-mark"))}
          aria-hidden="true"
          style={iconSize === undefined ? undefined : { width: iconSize, height: iconSize }}
        >
          <X size={18} strokeWidth={2} />
        </span>
      </Button>
    </DialogClose>
  );

  return <Tooltip content={title ?? label}>{button}</Tooltip>;
});
