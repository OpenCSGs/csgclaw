import type { ReactNode } from "react";
import styles from "./ResourceListCard.module.css";

export function ResourceList({ children }: { children: ReactNode }) {
  return <div className={styles.grid}>{children}</div>;
}

export function ResourceListCard({
  title,
  description,
  icon,
  badge,
  actions,
  active,
  onOpen,
}: {
  title: string;
  description?: string;
  icon: ReactNode;
  badge?: ReactNode;
  actions?: ReactNode;
  active?: boolean;
  onOpen: () => void;
}) {
  return (
    <article className={`${styles.card} ${active ? styles.active : ""}`}>
      <button type="button" className={styles.content} onClick={onOpen}>
        <span className={styles.icon} aria-hidden="true">
          {icon}
        </span>
        <span className={styles.copy}>
          <span className={styles.title}>{title}</span>
          <span className={styles.description}>{description || title}</span>
          {badge}
        </span>
      </button>
      {actions ? <div className={styles.actions}>{actions}</div> : null}
    </article>
  );
}
