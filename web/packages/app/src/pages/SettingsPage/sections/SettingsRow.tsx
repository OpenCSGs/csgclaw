import { classNames } from "@/shared/lib/classNames";
import type { ReactNode, Ref } from "react";
import styles from "../SettingsPage.module.css";

export type SettingsRowProps = {
  title: ReactNode;
  description: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  contentClassName?: string;
  ref?: Ref<HTMLElement>;
};

export function SettingsRow({
  title,
  description,
  actions,
  children,
  className,
  contentClassName,
  ref,
}: SettingsRowProps) {
  return (
    <section ref={ref} className={classNames(styles.row, className)}>
      <div className={styles.rowHead}>
        <div className={styles.rowIntro}>
          <h2>{title}</h2>
          <p>{description}</p>
        </div>
        {actions ? <div className={styles.rowActions}>{actions}</div> : null}
      </div>
      <div className={classNames(styles.rowContent, contentClassName)}>{children}</div>
    </section>
  );
}
