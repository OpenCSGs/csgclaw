import { useEffect, useState, type ReactNode } from "react";
import styles from "./UiLab.module.css";

/** Mount on first visit, then retain local demo state across section changes. */
export function UiLabPanel({
  id,
  active,
  visible = active === id,
  children,
}: {
  id: string;
  active: string;
  visible?: boolean;
  children: ReactNode;
}) {
  const [visited, setVisited] = useState(visible);
  useEffect(() => {
    if (visible) setVisited(true);
  }, [visible]);
  return (
    <div className={styles.panel} hidden={!visible}>
      {(visited || visible) && children}
    </div>
  );
}
