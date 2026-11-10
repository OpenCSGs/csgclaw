import type { ReactNode } from "react";
import styles from "./OperationStatus.module.css";

export function OperationStatus({ phase, children }: { phase: string; children: ReactNode }) {
  return (
    <div className={styles.status} data-phase={phase} role="status">
      <span className={styles.dot} aria-hidden="true" />
      <p>{children}</p>
    </div>
  );
}
