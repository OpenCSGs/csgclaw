import type { ReactNode } from "react";
import styles from "./ResourceFeaturedIcon.module.css";

export function ResourceFeaturedIcon({ children }: { children: ReactNode }) {
  return (
    <span className={styles.icon} aria-hidden="true">
      <span className={styles.shadow} />
      <span className={styles.glass}>{children}</span>
    </span>
  );
}
