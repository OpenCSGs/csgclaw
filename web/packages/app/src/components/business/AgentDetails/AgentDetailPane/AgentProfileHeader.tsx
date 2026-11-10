import { EntityHeader } from "@/components/business/EntityDetail";
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
      <EntityHeader
        avatar={avatar}
        title={title}
        actions={
          <div className="entity-toolbar" aria-label={actionsLabel}>
            {actions}
          </div>
        }
        className={classNames(styles.header, className)}
        headingClassName={styles.heading}
        titleClassName={styles.titleRow}
      >
        {description ? <div className={styles.description}>{description}</div> : null}
      </EntityHeader>
    </div>
  );
}
