import type { ReactNode } from "react";
import styles from "./ResourceStatus.module.css";

export function ResourceStatus({
  status,
  children,
}: {
  status: "enabled" | "disabled" | "error" | "pending";
  children: ReactNode;
}) {
  return (
    <span className={styles.status} data-status={status}>
      {children}
    </span>
  );
}
