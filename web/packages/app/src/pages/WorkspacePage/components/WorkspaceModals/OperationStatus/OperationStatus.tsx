import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";
import styles from "./OperationStatus.module.css";

export function OperationStatus({
  phase,
  role = "status",
  children,
}: {
  phase: string;
  role?: "status" | "alert";
  children: ReactNode;
}) {
  return (
    <div className={styles.status} data-phase={phase} role={role}>
      {role === "alert" ? (
        <CircleAlert className={styles.alertIcon} size={16} aria-hidden="true" />
      ) : (
        <span className={styles.dot} aria-hidden="true" />
      )}
      <p>{children}</p>
    </div>
  );
}
