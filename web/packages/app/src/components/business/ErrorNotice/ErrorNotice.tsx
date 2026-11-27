import type { ReactNode } from "react";
import { CircleAlert, X } from "lucide-react";
import { IconButton } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import styles from "./ErrorNotice.module.css";

export function ErrorNotice({
  action,
  children,
  className,
  dismissLabel,
  onDismiss,
}: {
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  dismissLabel: string;
  onDismiss?: () => void;
}) {
  return (
    <div className={classNames(styles.notice, action ? styles.centered : undefined, className)}>
      <CircleAlert className={styles.icon} size={16} aria-hidden="true" />
      <div className={action ? styles.withAction : undefined}>
        <div className={styles.content} role="alert">
          {children}
        </div>
        {action}
      </div>
      {onDismiss ? (
        <IconButton
          variant="tertiaryGray"
          size="sm"
          label={dismissLabel}
          icon={<X />}
          iconSize={16}
          onClick={onDismiss}
        />
      ) : null}
    </div>
  );
}
