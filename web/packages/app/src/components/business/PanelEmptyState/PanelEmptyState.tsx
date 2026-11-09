import { EmptyState } from "csgclaw-ui";
import type { EmptyStateProps } from "csgclaw-ui";
import type { ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";
import styles from "./PanelEmptyState.module.css";

export function PanelEmptyState({ className, mark, icon, ...props }: EmptyStateProps & { mark?: ReactNode }) {
  return (
    <EmptyState
      {...props}
      className={classNames("shell-empty-state", styles.panel, className)}
      icon={mark ? <span className={styles.mark}>{mark}</span> : icon}
    />
  );
}
