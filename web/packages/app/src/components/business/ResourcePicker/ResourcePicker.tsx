import type { ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";
import {
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui";
import styles from "./ResourcePicker.module.css";

export function ResourcePicker({
  title,
  description,
  closeLabel,
  busy,
  fixedHeight = false,
  portalContainer,
  children,
  actions,
  footer,
}: {
  title: string;
  description: string;
  closeLabel: string;
  busy?: boolean;
  fixedHeight?: boolean;
  portalContainer?: HTMLElement | null;
  children: ReactNode;
  actions: ReactNode;
  footer: ReactNode;
}) {
  return (
    <DialogContent
      className={classNames(styles.dialog, fixedHeight && styles.fixedHeight)}
      portalContainer={portalContainer}
    >
      <DialogHeader className={styles.header}>
        <DialogTitle>{title}</DialogTitle>
        <DialogCloseButton label={closeLabel} disabled={busy} size="sm" variant="tertiaryGray" />
        <DialogDescription className={styles.description} size="md">
          {description}
        </DialogDescription>
      </DialogHeader>
      <DialogBody className={styles.body}>{children}</DialogBody>
      <ResourcePickerActions>{actions}</ResourcePickerActions>
      <DialogFooter className={styles.footer}>{footer}</DialogFooter>
    </DialogContent>
  );
}

export function ResourcePickerList({ children }: { children: ReactNode }) {
  return <div className={styles.list}>{children}</div>;
}

export function ResourcePickerRow({ children, selectable = false }: { children: ReactNode; selectable?: boolean }) {
  const Element = selectable ? "label" : "div";
  return <Element className={styles.row}>{children}</Element>;
}

export function ResourcePickerCopy({
  name,
  titleBadge,
  description,
  metadata,
  onOpen,
}: {
  name: string;
  titleBadge?: ReactNode;
  description?: string;
  metadata?: string;
  onOpen?: (trigger: HTMLButtonElement) => void;
}) {
  const copy = (
    <>
      <span className={styles.copyHeading}>
        <strong className={styles.name} title={name}>
          {name}
        </strong>
        {titleBadge}
        {metadata ? (
          <span className={styles.metadata} title={metadata}>
            {metadata}
          </span>
        ) : null}
      </span>
      <span className={styles.copyDescription}>{description}</span>
    </>
  );
  return onOpen ? (
    <button
      type="button"
      className={classNames(styles.copy, metadata && styles.withMetadata)}
      aria-label={name}
      onClick={(event) => onOpen(event.currentTarget)}
    >
      {copy}
    </button>
  ) : (
    <span className={classNames(styles.copy, metadata && styles.withMetadata)}>{copy}</span>
  );
}

export function ResourcePickerActions({ children }: { children: ReactNode }) {
  return <div className={styles.links}>{children}</div>;
}
