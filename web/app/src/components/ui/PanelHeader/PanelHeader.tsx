import styles from "./PanelHeader.module.css";
import type { HTMLAttributes, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";

export type PanelHeaderProps = HTMLAttributes<HTMLElement> & { actions?: ReactNode };

export function PanelHeader({ children, actions, className, ...props }: PanelHeaderProps) {
  return (
    <header className={classNames(styles["csg-panel-header"], className)} {...props}>
      {children}
      {actions ? <div className={styles["csg-panel-header-actions"]}>{actions}</div> : null}
    </header>
  );
}
