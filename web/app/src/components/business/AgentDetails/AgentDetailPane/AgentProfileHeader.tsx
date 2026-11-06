import type { ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";
import styles from "./AgentProfileHeader.module.css";

export function AgentProfileHeader({
  avatar,
  title,
  description,
  actions,
  className,
  containerClassName,
  actionsLabel,
}: {
  avatar: ReactNode;
  title: ReactNode;
  description: ReactNode;
  actions: ReactNode;
  className?: string;
  containerClassName?: string;
  actionsLabel?: string;
}) {
  return (
    <div className={classNames(styles.container, containerClassName)}>
      <header className={classNames("entity-header", styles.header, className)}>
        {avatar}
        <div className={classNames("entity-heading", styles.heading)}>
          <div className={classNames("entity-title-row", styles.titleRow)}>{title}</div>
          {description ? <div className={styles.description}>{description}</div> : null}
        </div>
        <div className="entity-toolbar" aria-label={actionsLabel}>
          {actions}
        </div>
      </header>
    </div>
  );
}
