import { useEffect, useState, type ReactNode } from "react";
import styles from "./UiLab.module.css";

/** Mount on first visit, then retain local demo state across section changes. */
export function UiLabPanel({ id, active, children }: { id: string; active: string; children: ReactNode }) {
  const [visited, setVisited] = useState(active === id);
  useEffect(() => {
    if (active === id) setVisited(true);
  }, [active, id]);
  return (
    <div className={styles.panel} hidden={active !== id}>
      {(visited || active === id) && children}
    </div>
  );
}
